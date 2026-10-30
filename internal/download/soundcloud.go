package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type scHydration struct {
	Hydratable string          `json:"hydratable"`
	Data       json.RawMessage `json:"data"`
}

type scPlaylist struct {
	Title      string    `json:"title"`
	ArtworkURL string    `json:"artwork_url"`
	User       scUser    `json:"user"`
	Tracks     []scTrack `json:"tracks"`
}

type scUser struct {
	Username string `json:"username"`
}

type scTrack struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Duration     int64  `json:"duration"`
	PermalinkURL string `json:"permalink_url"`
	ArtworkURL   string `json:"artwork_url"`
	ReleaseDate  string `json:"release_date"`
	User         scUser `json:"user"`
}

type scOEmbed struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// previewSoundCloud reads a set page and parses the hydration JSON. It is much
// faster than asking yt-dlp to resolve every track: one request returns the
// whole track list. SoundCloud only embeds full metadata for the first few
// tracks when signed out, so the remaining ids are resolved through oEmbed.
func (m *Manager) previewSoundCloud(ctx context.Context, link Link) (Preview, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	playlist, err := m.fetchSoundCloudPlaylist(ctx, link.URL)
	if err != nil {
		return Preview{}, withNetworkHint(link, err)
	}
	if len(playlist.Tracks) == 0 {
		return Preview{}, errors.New("this SoundCloud set has no tracks")
	}

	preview := Preview{
		Raw:      link.Raw,
		URL:      link.URL,
		Kind:     link.Kind,
		Title:    cleanText(playlist.Title),
		Subtitle: cleanArtist(playlist.User.Username),
		CoverURL: upgradeSoundCloudArtwork(playlist.ArtworkURL),
	}
	if preview.Title == "" {
		preview.Title = link.URL
	}

	tracks := playlist.Tracks
	if len(tracks) > maxBatchTracks {
		tracks = tracks[:maxBatchTracks]
		preview.Truncated = true
	}
	resolved := m.resolveSoundCloudTracks(ctx, tracks)
	for _, track := range resolved {
		if track.URL == "" || track.Title == "" {
			preview.Skipped++
			continue
		}
		if track.Album == "" {
			track.Album = preview.Title
		}
		if track.CoverURL == "" {
			track.CoverURL = preview.CoverURL
		}
		if track.Duration <= 0 {
			preview.PartialDuration = true
		}
		preview.Tracks = append(preview.Tracks, track)
		preview.TotalDuration += track.Duration
	}
	if len(preview.Tracks) == 0 {
		return Preview{}, errors.New("this SoundCloud set has no playable tracks")
	}
	return preview, nil
}

func (m *Manager) fetchSoundCloudPlaylist(ctx context.Context, pageURL string) (scPlaylist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return scPlaylist{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := m.client.Do(req)
	if err != nil {
		return scPlaylist{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return scPlaylist{}, fmt.Errorf("SoundCloud returned HTTP %d for this set", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return scPlaylist{}, err
	}
	playlist, err := parseSoundCloudHydration(string(body))
	if err == nil {
		return playlist, nil
	}

	// If SoundCloud changes the page format, fall back to yt-dlp's flat list.
	// Entries may only contain ids; soundCloudTrack resolves those via oEmbed.
	info, ytdlpErr := m.fetchInfo(ctx, pageURL, true)
	if ytdlpErr != nil {
		return scPlaylist{}, fmt.Errorf("could not read the SoundCloud set: %w", err)
	}
	fallback := scPlaylist{
		Title:      info.Title,
		ArtworkURL: info.Thumbnail,
		User:       scUser{Username: firstNonEmpty(info.Uploader, info.Channel)},
	}
	for _, entry := range info.Entries {
		id, parseErr := strconv.ParseInt(strings.TrimSpace(entry.ID), 10, 64)
		if parseErr != nil {
			continue
		}
		url := ""
		if u := strings.TrimSpace(entry.WebpageURL); strings.HasPrefix(u, "http") {
			url = u
		} else if u := strings.TrimSpace(entry.URL); strings.HasPrefix(u, "http") {
			url = u
		}
		fallback.Tracks = append(fallback.Tracks, scTrack{
			ID:           id,
			Title:        entry.Title,
			Duration:     int64(entry.Duration * 1000),
			PermalinkURL: url,
			ArtworkURL:   entry.Thumbnail,
			ReleaseDate:  entry.ReleaseDate,
			User:         scUser{Username: firstNonEmpty(entry.Uploader, entry.Channel)},
		})
	}
	if len(fallback.Tracks) == 0 {
		return scPlaylist{}, fmt.Errorf("could not read the SoundCloud set: %w", err)
	}
	return fallback, nil
}

func (m *Manager) resolveSoundCloudTracks(ctx context.Context, tracks []scTrack) []PreviewTrack {
	out := make([]PreviewTrack, len(tracks))
	jobs := make(chan int)
	var wg sync.WaitGroup

	workers := 6
	if len(tracks) < workers {
		workers = len(tracks)
	}
	if workers < 1 {
		return out
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if track, ok := m.soundCloudTrack(ctx, tracks[index]); ok {
					out[index] = track
				}
			}
		}()
	}
	for i := range tracks {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return out
		}
	}
	close(jobs)
	wg.Wait()
	return out
}

func (m *Manager) soundCloudTrack(ctx context.Context, track scTrack) (PreviewTrack, bool) {
	url := strings.TrimSpace(track.PermalinkURL)
	title := cleanText(track.Title)
	artist := ""
	year := ""
	cover := upgradeSoundCloudArtwork(track.ArtworkURL)

	if title == "" && track.ID != 0 {
		if url == "" {
			url = fmt.Sprintf("https://api.soundcloud.com/tracks/%d", track.ID)
		}
		embed, err := m.fetchSoundCloudOEmbed(ctx, track.ID)
		if err != nil {
			return PreviewTrack{}, false
		}
		title = cleanOEmbedTitle(embed.Title, embed.AuthorName)
		artist = cleanArtist(embed.AuthorName)
		cover = upgradeSoundCloudArtwork(embed.ThumbnailURL)
	} else if title == "" {
		return PreviewTrack{}, false
	}

	if left, right, ok := splitArtistTitle(title); ok {
		artist, title = cleanText(left), cleanText(right)
	}
	if artist == "" {
		artist = cleanArtist(track.User.Username)
	}
	title = cleanVideoSuffix(title)
	if title == "" {
		return PreviewTrack{}, false
	}
	if len(track.ReleaseDate) >= 4 {
		year = track.ReleaseDate[:4]
	}
	return PreviewTrack{
		Title:    title,
		Artist:   artist,
		Year:     year,
		Duration: time.Duration(track.Duration) * time.Millisecond,
		URL:      url,
		CoverURL: cover,
	}, true
}

func (m *Manager) fetchSoundCloudOEmbed(ctx context.Context, id int64) (scOEmbed, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	target := "https://api.soundcloud.com/tracks/" + fmt.Sprintf("%d", id)
	endpoint := "https://soundcloud.com/oembed?format=json&url=" + url.QueryEscape(target)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return scOEmbed{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return scOEmbed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return scOEmbed{}, fmt.Errorf("SoundCloud oEmbed returned HTTP %d", resp.StatusCode)
	}
	var out scOEmbed
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return scOEmbed{}, err
	}
	if cleanText(out.Title) == "" {
		return scOEmbed{}, errors.New("SoundCloud oEmbed returned no title")
	}
	return out, nil
}

func cleanOEmbedTitle(title, author string) string {
	title = cleanText(title)
	author = cleanText(author)
	if author == "" {
		return title
	}
	suffix := " by " + author
	if strings.HasSuffix(strings.ToLower(title), strings.ToLower(suffix)) {
		title = strings.TrimSpace(title[:len(title)-len(suffix)])
	}
	return title
}

func parseSoundCloudHydration(html string) (scPlaylist, error) {
	const marker = "__sc_hydration"
	idx := strings.Index(html, marker)
	if idx < 0 {
		return scPlaylist{}, errors.New("page has no hydration data")
	}
	rest := html[idx+len(marker):]
	eq := strings.IndexByte(rest, '=')
	if eq < 0 {
		return scPlaylist{}, errors.New("page has no hydration assignment")
	}
	dec := json.NewDecoder(strings.NewReader(rest[eq+1:]))
	var items []scHydration
	if err := dec.Decode(&items); err != nil {
		return scPlaylist{}, fmt.Errorf("parse hydration: %w", err)
	}
	for _, item := range items {
		if item.Hydratable != "playlist" || len(item.Data) == 0 {
			continue
		}
		var playlist scPlaylist
		if err := json.Unmarshal(item.Data, &playlist); err != nil {
			return scPlaylist{}, fmt.Errorf("parse playlist: %w", err)
		}
		if len(playlist.Tracks) > 0 || playlist.Title != "" {
			return playlist, nil
		}
	}
	return scPlaylist{}, errors.New("no playlist found in page data")
}

func upgradeSoundCloudArtwork(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.Contains(u, "-large.") {
		return strings.Replace(u, "-large.", "-t500x500.", 1)
	}
	return u
}
