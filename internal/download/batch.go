package download

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// maxBatchTracks limits one playlist/set so a typo cannot queue thousands of
// downloads. The cap is deliberately generous; normal albums and mixes fit.
const maxBatchTracks = 500

// PreviewTrack is one track discovered inside a playlist or set.
type PreviewTrack struct {
	Title       string
	Artist      string
	Album       string
	Year        string
	Duration    time.Duration
	TrackNumber int
	DiscNumber  int
	URL         string
	CoverURL    string
}

// Preview describes a playlist or set before it is queued.
type Preview struct {
	Raw             string
	URL             string
	Kind            Kind
	Title           string
	Subtitle        string
	CoverURL        string
	Tracks          []PreviewTrack
	Skipped         int
	Truncated       bool
	PartialDuration bool
	TotalDuration   time.Duration
}

// Batch reports whether the preview contains several tracks.
func (p Preview) Batch() bool {
	return p.Kind.Batch()
}

// BatchResult summarizes an accepted preview.
type BatchResult struct {
	GroupID    int
	Queued     int
	Skipped    int
	Duplicates int
}

// Preview classifies a link and, for playlists and sets, reads the track list
// without downloading audio. Single-track links are returned as a non-batch
// preview so the caller can use the normal Enqueue path.
func (m *Manager) Preview(ctx context.Context, raw string) (Preview, error) {
	link, err := Classify(raw)
	if err != nil {
		return Preview{}, err
	}
	if link.Kind == KindSpotifyShort {
		resolved, expandErr := m.expandSpotifyShort(ctx, link.URL)
		if expandErr != nil {
			return Preview{}, expandErr
		}
		link = resolved
	}
	if link.Kind == KindSoundCloud && strings.Contains(strings.ToLower(link.URL), "on.soundcloud.com") {
		if resolved, expandErr := m.expandSoundCloudShort(ctx, link.URL); expandErr == nil {
			link = resolved
		}
	}

	preview := Preview{Raw: raw, URL: link.URL, Kind: link.Kind}
	if !link.Kind.Batch() {
		return preview, nil
	}
	if link.Kind == KindSoundCloudSet {
		return m.previewSoundCloud(ctx, link)
	}

	info, err := m.fetchInfo(ctx, link.URL, true)
	if err != nil {
		return Preview{}, withNetworkHint(link, err)
	}
	if info == nil {
		return Preview{}, errors.New("yt-dlp returned no playlist metadata")
	}
	if len(info.Entries) == 0 {
		return Preview{}, errors.New("this playlist has no downloadable tracks")
	}

	preview.Title = cleanText(info.Title)
	if preview.Title == "" {
		preview.Title = link.URL
	}
	preview.Subtitle = cleanArtist(firstNonEmpty(info.Uploader, info.Channel, info.Artist))
	preview.CoverURL = infoThumbnail(info)

	total := 0
	for _, entry := range info.Entries {
		track, ok := previewTrackFromEntry(link.Kind, entry, preview.Title)
		if !ok {
			preview.Skipped++
			continue
		}
		total++
		if total > maxBatchTracks {
			preview.Truncated = true
			break
		}
		preview.Tracks = append(preview.Tracks, track)
		preview.TotalDuration += track.Duration
		if preview.CoverURL == "" && track.CoverURL != "" {
			preview.CoverURL = track.CoverURL
		}
	}
	if len(preview.Tracks) == 0 {
		return Preview{}, errors.New("this playlist has no playable tracks")
	}
	return preview, nil
}

func previewTrackFromEntry(kind Kind, entry ytInfo, album string) (PreviewTrack, bool) {
	title := cleanText(firstNonEmpty(entry.Track, entry.Title))
	if title == "" || isUnavailableEntry(title) || entry.IsLive {
		return PreviewTrack{}, false
	}
	url := batchEntryURL(kind, entry)
	if url == "" {
		return PreviewTrack{}, false
	}

	artist := cleanArtist(firstNonEmpty(entry.Artist, entry.Creator, entry.Uploader, entry.Channel))
	switch {
	case artist != "":
		title = stripArtistPrefix(title, artist)
	case strings.Contains(title, " - "):
		if left, right, ok := splitArtistTitle(title); ok {
			artist, title = cleanText(left), cleanText(right)
		}
	}
	title = cleanVideoSuffix(title)
	if title == "" {
		return PreviewTrack{}, false
	}

	year := ""
	switch {
	case entry.ReleaseYear > 0:
		year = fmt.Sprintf("%d", entry.ReleaseYear)
	case len(entry.ReleaseDate) >= 4:
		year = entry.ReleaseDate[:4]
	}

	trackAlbum := cleanText(entry.Album)
	if trackAlbum == "" {
		trackAlbum = album
	}
	cover := infoThumbnail(&entry)
	return PreviewTrack{
		Title:    title,
		Artist:   artist,
		Album:    trackAlbum,
		Year:     year,
		Duration: secondsToDuration(entry.Duration),
		URL:      url,
		CoverURL: cover,
	}, true
}

func batchEntryURL(kind Kind, entry ytInfo) string {
	if u := strings.TrimSpace(entry.WebpageURL); strings.HasPrefix(u, "http") {
		return u
	}
	raw := strings.TrimSpace(entry.URL)
	if strings.HasPrefix(raw, "http") {
		return raw
	}
	if kind == KindYouTubePlaylist && youtubeIDPattern.MatchString(entry.ID) {
		return "https://www.youtube.com/watch?v=" + entry.ID
	}
	if kind == KindSoundCloudSet && strings.Contains(raw, "/") && entry.ID != "" {
		return "https://soundcloud.com/" + strings.TrimPrefix(raw, "/")
	}
	if kind == KindSoundCloudSet && strings.Contains(entry.ID, "/") {
		return "https://soundcloud.com/" + strings.TrimPrefix(entry.ID, "/")
	}
	return ""
}

func isUnavailableEntry(title string) bool {
	lower := strings.ToLower(strings.TrimSpace(title))
	switch lower {
	case "[deleted video]", "[private video]", "[unavailable video]", "deleted video", "private video":
		return true
	}
	return strings.Contains(lower, "[deleted video]") || strings.Contains(lower, "[private video]")
}

// EnqueuePreview validates a preview and adds every track to the queue as one
// group. Duplicate URLs inside the batch are skipped.
func (m *Manager) EnqueuePreview(preview Preview) (BatchResult, error) {
	if !preview.Kind.Batch() {
		return BatchResult{}, errors.New("this link is not a playlist or set")
	}
	if len(preview.Tracks) == 0 {
		return BatchResult{}, errors.New("nothing to download")
	}
	if err := m.checkTools(true); err != nil {
		return BatchResult{}, err
	}

	type item struct {
		track PreviewTrack
		link  Link
	}
	items := make([]item, 0, len(preview.Tracks))
	seen := make(map[string]bool, len(preview.Tracks))
	result := BatchResult{}
	for _, track := range preview.Tracks {
		url := strings.TrimSpace(track.URL)
		if url == "" {
			result.Skipped++
			continue
		}
		if seen[url] {
			result.Duplicates++
			continue
		}
		seen[url] = true
		kind := KindYouTube
		if preview.Kind == KindSoundCloudSet {
			kind = KindSoundCloud
		}
		items = append(items, item{
			track: track,
			link: Link{
				Raw:  preview.URL,
				URL:  url,
				Kind: kind,
				ID:   trackIDFromURL(url),
			},
		})
	}
	if len(items) == 0 {
		return BatchResult{}, errors.New("all tracks were skipped or duplicated")
	}

	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return BatchResult{}, errors.New("downloader is shutting down")
	}
	m.nextGroupID++
	groupID := m.nextGroupID
	groupTitle := preview.Title
	if groupTitle == "" {
		groupTitle = preview.URL
	}
	ids := make([]int, 0, len(items))
	for i, it := range items {
		m.nextID++
		seed := Meta{
			Title:       it.track.Title,
			Artist:      it.track.Artist,
			Album:       it.track.Album,
			Year:        it.track.Year,
			Duration:    it.track.Duration,
			CoverURL:    it.track.CoverURL,
			SourceURL:   preview.URL,
			DownloadURL: it.track.URL,
			TrackNumber: i + 1,
			DiscNumber:  it.track.DiscNumber,
		}
		if seed.CoverURL == "" {
			seed.CoverURL = preview.CoverURL
		}
		it.link.Seed = &seed
		job := &Job{
			ID:          m.nextID,
			URL:         it.track.URL,
			Kind:        it.link.Kind,
			Title:       it.track.Title,
			Artist:      it.track.Artist,
			Stage:       "queued",
			Detail:      "waiting for a free slot",
			Progress:    -1,
			Status:      StatusQueued,
			Created:     time.Now(),
			GroupID:     groupID,
			GroupTitle:  groupTitle,
			GroupKind:   preview.Kind,
			TrackNumber: i + 1,
			TrackTotal:  len(items),
			link:        it.link,
		}
		m.jobs[job.ID] = job
		m.order = append(m.order, job.ID)
		ids = append(ids, job.ID)
	}
	m.mu.Unlock()

	for _, id := range ids {
		select {
		case m.queue <- id:
		case <-m.ctx.Done():
			return BatchResult{}, errors.New("downloader is shutting down")
		}
	}
	result.GroupID = groupID
	result.Queued = len(ids)
	return result, nil
}

func trackIDFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndex(raw, "v="); idx >= 0 {
		return raw[idx+2:]
	}
	raw = strings.TrimSuffix(raw, "/")
	if idx := strings.LastIndex(raw, "/"); idx >= 0 {
		return raw[idx+1:]
	}
	return raw
}
