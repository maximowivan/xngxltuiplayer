package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Meta is the metadata used for tagging and display.
type Meta struct {
	Title       string
	Artist      string
	Album       string
	Year        string
	Duration    time.Duration
	CoverURL    string
	SourceURL   string
	DownloadURL string
}

type ytInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Track       string   `json:"track"`
	Artist      string   `json:"artist"`
	Creator     string   `json:"creator"`
	Album       string   `json:"album"`
	Uploader    string   `json:"uploader"`
	Channel     string   `json:"channel"`
	ReleaseYear int      `json:"release_year"`
	ReleaseDate string   `json:"release_date"`
	Duration    float64  `json:"duration"`
	Thumbnail   string   `json:"thumbnail"`
	WebpageURL  string   `json:"webpage_url"`
	URL         string   `json:"url"`
	Ext         string   `json:"ext"`
	IsLive      bool     `json:"is_live"`
	Entries     []ytInfo `json:"entries"`
}

type oembedResponse struct {
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
	AuthorName   string `json:"author_name"`
}

type itunesResponse struct {
	Results []itunesTrack `json:"results"`
}

type itunesTrack struct {
	TrackName       string `json:"trackName"`
	ArtistName      string `json:"artistName"`
	CollectionName  string `json:"collectionName"`
	ReleaseDate     string `json:"releaseDate"`
	ArtworkURL100   string `json:"artworkUrl100"`
	TrackTimeMillis int64  `json:"trackTimeMillis"`
}

func (m *Manager) resolve(ctx context.Context, link Link, id int) (Meta, error) {
	m.setDetail(id, "resolving link")
	if link.Kind == KindSpotifyShort {
		resolved, err := m.expandSpotifyShort(ctx, link.URL)
		if err != nil {
			return Meta{}, err
		}
		link = resolved
	}

	switch link.Kind {
	case KindSpotifyTrack:
		return m.resolveSpotify(ctx, link, id)
	case KindDirect:
		title := titleFromURL(link.URL)
		return Meta{
			Title:       title,
			SourceURL:   link.URL,
			DownloadURL: link.URL,
		}, nil
	default:
		info, err := m.fetchInfo(ctx, link.URL, false)
		if err != nil {
			return Meta{}, withNetworkHint(link, err)
		}
		meta, err := metaFromInfo(info)
		if err != nil {
			return Meta{}, err
		}
		meta.SourceURL = link.URL
		return meta, nil
	}
}

func (m *Manager) expandSpotifyShort(ctx context.Context, shortURL string) (Link, error) {
	client := *m.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	current := shortURL
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, current, nil)
		if err != nil {
			return Link{}, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			return Link{}, fmt.Errorf("resolve Spotify short link: %w", err)
		}
		_ = resp.Body.Close()
		loc := resp.Header.Get("Location")
		if loc == "" {
			break
		}
		next, err := url.Parse(loc)
		if err != nil {
			break
		}
		if !next.IsAbs() {
			base, _ := url.Parse(current)
			next = base.ResolveReference(next)
		}
		current = next.String()
		if strings.Contains(strings.ToLower(current), "open.spotify.com/track/") {
			return Classify(current)
		}
	}
	return Link{}, errors.New("could not resolve the Spotify short link — paste the full track link instead")
}

func (m *Manager) resolveSpotify(ctx context.Context, link Link, id int) (Meta, error) {
	m.setDetail(id, "reading Spotify metadata")
	oembed, err := m.fetchOEmbed(ctx, link.URL)
	if err != nil {
		return Meta{}, fmt.Errorf("Spotify metadata unavailable: %w", err)
	}

	title := cleanText(oembed.Title)
	artist := cleanText(oembed.AuthorName)
	if title == "" {
		return Meta{}, errors.New("Spotify returned no track title")
	}

	m.setDetail(id, "matching track on iTunes")
	it, itErr := m.searchITunes(ctx, title, artist)
	if itErr != nil && artist != "" {
		// Some artist spellings return nothing; retry with the title alone.
		it, _ = m.searchITunes(ctx, title, "")
	}
	album, year := "", ""
	var duration time.Duration
	coverURL := oembed.ThumbnailURL
	if it != nil {
		if artist == "" {
			artist = cleanText(it.ArtistName)
		}
		album = cleanText(it.CollectionName)
		if len(it.ReleaseDate) >= 4 {
			year = it.ReleaseDate[:4]
		}
		if it.TrackTimeMillis > 0 {
			duration = time.Duration(it.TrackTimeMillis) * time.Millisecond
		}
		if coverURL == "" {
			coverURL = upgradeArtwork(it.ArtworkURL100)
		}
	}

	query := title + " audio"
	if artist != "" {
		query = artist + " " + title + " audio"
	}
	m.setDetail(id, "finding audio")
	info, err := m.fetchSearch(ctx, query, 8)
	if err != nil || info == nil {
		info, err = m.fetchSearch(ctx, title+" audio", 5)
	}
	if err != nil || info == nil {
		if err == nil {
			err = errors.New("no matching audio found")
		}
		return Meta{}, fmt.Errorf("could not find audio for %q: %w", title, err)
	}

	if artist == "" {
		artist = cleanArtist(firstNonEmpty(info.Artist, info.Creator, info.Uploader, info.Channel))
	}
	if duration <= 0 {
		duration = secondsToDuration(info.Duration)
	}
	if coverURL == "" {
		coverURL = info.Thumbnail
	}
	downloadURL := infoURL(info)
	if downloadURL == "" {
		return Meta{}, errors.New("could not build a download URL for the matched audio")
	}

	meta := Meta{
		Title:       title,
		Artist:      artist,
		Album:       album,
		Year:        year,
		Duration:    duration,
		CoverURL:    coverURL,
		SourceURL:   link.URL,
		DownloadURL: downloadURL,
	}
	if meta.Artist == "" {
		meta.Artist = cleanArtist(info.Uploader)
	}
	return meta, nil
}

func (m *Manager) fetchOEmbed(ctx context.Context, spotifyURL string) (oembedResponse, error) {
	var out oembedResponse
	endpoint := "https://open.spotify.com/oembed?url=" + url.QueryEscape(spotifyURL)
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnavailableForLegalReasons {
		return out, errors.New("Spotify blocks metadata requests in this region (HTTP 451); a VPN may help")
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("Spotify oEmbed returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, fmt.Errorf("parse Spotify oEmbed: %w", err)
	}
	return out, nil
}

func (m *Manager) searchITunes(ctx context.Context, title, artist string) (*itunesTrack, error) {
	term := strings.TrimSpace(title)
	if a := strings.TrimSpace(artist); a != "" {
		term = a + " " + term
	}
	endpoint := "https://itunes.apple.com/search?term=" + url.QueryEscape(term) +
		"&entity=song&limit=15&media=music"
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iTunes returned HTTP %d", resp.StatusCode)
	}
	var parsed itunesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&parsed); err != nil {
		return nil, err
	}

	want := normalizeTitle(title)
	wantArtist := normalizeTitle(artist)
	bestScore := -1000
	var best *itunesTrack
	for i := range parsed.Results {
		it := &parsed.Results[i]
		got := normalizeTitle(it.TrackName)
		score := 0
		switch {
		case got == want && want != "":
			score += 100
		case got != "" && strings.Contains(got, want):
			score += 55
		case want != "" && strings.Contains(want, got):
			score += 35
		default:
			score -= 40
		}
		if wantArtist != "" {
			gotArtist := normalizeTitle(it.ArtistName)
			switch {
			case gotArtist == wantArtist && gotArtist != "":
				score += 60
			case gotArtist != "" && strings.Contains(gotArtist, wantArtist):
				score += 30
			case wantArtist != "" && strings.Contains(wantArtist, gotArtist):
				score += 20
			default:
				score -= 25
			}
		}
		if it.ArtistName != "" && it.CollectionName != "" {
			score += 5
		}
		lowered := strings.ToLower(it.TrackName)
		for _, bad := range []string{"karaoke", "tribute", "made famous", "cover version", "instrumental"} {
			if strings.Contains(lowered, bad) && !strings.Contains(strings.ToLower(title), bad) {
				score -= 70
			}
		}
		if score > bestScore {
			bestScore = score
			best = it
		}
	}
	if best == nil || bestScore < 0 {
		return nil, errors.New("no confident iTunes match")
	}
	return best, nil
}

func (m *Manager) fetchSearch(ctx context.Context, query string, limit int) (*ytInfo, error) {
	if limit < 1 {
		limit = 1
	}
	target := fmt.Sprintf("ytsearch%d:%s", limit, query)
	info, err := m.fetchInfo(ctx, target, true)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("empty search result")
	}
	if len(info.Entries) == 0 {
		// Some yt-dlp versions return the first video directly.
		if info.ID != "" && info.Title != "" {
			return info, nil
		}
		return nil, errors.New("no search results")
	}
	entries := make([]ytInfo, 0, len(info.Entries))
	for _, e := range info.Entries {
		if e.ID != "" && !e.IsLive {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("no playable search results")
	}
	best := chooseSearchEntry(entries, query)
	return &best, nil
}

func chooseSearchEntry(entries []ytInfo, query string) ytInfo {
	q := normalizeTitle(query)
	// Strip the trailing word "audio" from the query for scoring.
	q = strings.TrimSuffix(q, " audio")
	bestScore := math.MinInt
	best := entries[0]
	for _, e := range entries {
		title := normalizeTitle(e.Title)
		uploader := normalizeTitle(firstNonEmpty(e.Uploader, e.Channel, e.Artist))
		score := 0
		if title == q {
			score += 80
		} else if strings.Contains(title, q) {
			score += 45
		} else if q != "" && strings.Contains(q, title) {
			score += 25
		}
		if uploader != "" && strings.Contains(q, uploader) {
			score += 25
		}
		if e.Duration >= 45 && e.Duration <= 15*60 {
			score += 10
		}
		lowered := strings.ToLower(e.Title)
		for _, bad := range []string{"karaoke", "nightcore", "8d audio", "instrumental", "cover", "reaction"} {
			if strings.Contains(lowered, bad) && !strings.Contains(strings.ToLower(query), bad) {
				score -= 60
			}
		}
		if strings.Contains(lowered, "live") && !strings.Contains(strings.ToLower(query), "live") {
			score -= 25
		}
		if score > bestScore {
			bestScore = score
			best = e
		}
	}
	return best
}

func metaFromInfo(info *ytInfo) (Meta, error) {
	if info == nil {
		return Meta{}, errors.New("empty metadata")
	}
	if info.IsLive {
		return Meta{}, errors.New("live streams cannot be downloaded")
	}
	title := cleanText(firstNonEmpty(info.Track, info.Title))
	artist := cleanArtist(firstNonEmpty(info.Artist, info.Creator, info.Uploader, info.Channel))
	if artist == "" {
		if left, right, ok := splitArtistTitle(title); ok {
			artist, title = cleanText(left), cleanText(right)
		}
	} else {
		title = stripArtistPrefix(title, artist)
	}
	title = cleanVideoSuffix(title)
	if title == "" {
		return Meta{}, errors.New("could not determine the track title")
	}
	year := ""
	if info.ReleaseYear > 0 {
		year = strconv.Itoa(info.ReleaseYear)
	} else if len(info.ReleaseDate) >= 4 {
		year = info.ReleaseDate[:4]
	}
	downloadURL := infoURL(info)
	if downloadURL == "" {
		return Meta{}, errors.New("could not determine a download URL")
	}
	return Meta{
		Title:       title,
		Artist:      artist,
		Album:       cleanText(info.Album),
		Year:        year,
		Duration:    secondsToDuration(info.Duration),
		CoverURL:    info.Thumbnail,
		DownloadURL: downloadURL,
	}, nil
}

func infoURL(info *ytInfo) string {
	if info == nil {
		return ""
	}
	if u := strings.TrimSpace(info.WebpageURL); strings.HasPrefix(u, "http") && !strings.Contains(u, "ytsearch") {
		return u
	}
	if u := strings.TrimSpace(info.URL); strings.HasPrefix(u, "http") && !strings.Contains(u, "ytsearch") {
		return u
	}
	if youtubeIDPattern.MatchString(info.ID) {
		return "https://www.youtube.com/watch?v=" + info.ID
	}
	return ""
}

var suffixPattern = regexp.MustCompile(`(?i)\s*[\[(](official\s*(music\s*)?(video|audio)|official\s*lyric\s*video|lyric\s*video|audio|visualizer|hd|hq|4k|explicit)[\])]\s*$`)

func cleanVideoSuffix(title string) string {
	for {
		cleaned := suffixPattern.ReplaceAllString(title, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == title || cleaned == "" {
			break
		}
		title = cleaned
	}
	return strings.TrimSpace(title)
}

func stripArtistPrefix(title, artist string) string {
	lowerTitle := strings.ToLower(title)
	lowerArtist := strings.ToLower(strings.TrimSpace(artist))
	if lowerArtist == "" {
		return title
	}
	prefixes := []string{lowerArtist + " - ", lowerArtist + " – ", lowerArtist + " — ", lowerArtist + ": "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerTitle, prefix) {
			rest := title[len(prefix):]
			if strings.TrimSpace(rest) != "" {
				return strings.TrimSpace(rest)
			}
		}
	}
	return title
}

func splitArtistTitle(title string) (artist, track string, ok bool) {
	for _, sep := range []string{" - ", " – ", " — "} {
		if idx := strings.Index(title, sep); idx > 0 && idx < len(title)-len(sep) {
			return title[:idx], title[idx+len(sep):], true
		}
	}
	return "", "", false
}

func cleanText(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// cleanArtist removes suffixes that YouTube adds to auto-generated artist
// channels, e.g. "Aurora - Topic".
func cleanArtist(s string) string {
	s = cleanText(s)
	lower := strings.ToLower(s)
	const topic = " - topic"
	if strings.HasSuffix(lower, topic) {
		s = strings.TrimSpace(s[:len(s)-len(topic)])
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func secondsToDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func upgradeArtwork(u string) string {
	if u == "" {
		return ""
	}
	replacer := strings.NewReplacer("100x100bb", "900x900bb", "200x200bb", "900x900bb", "300x300bb", "900x900bb")
	return replacer.Replace(u)
}

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Remove bracketed suffixes and featured-artist credits for comparison.
	for {
		start := strings.IndexAny(s, "([")
		if start < 0 {
			break
		}
		end := strings.IndexAny(s[start:], ")]")
		if end < 0 {
			break
		}
		s = s[:start] + " " + s[start+end+1:]
	}
	if idx := strings.Index(s, " feat. "); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, " ft. "); idx >= 0 {
		s = s[:idx]
	}
	s = nonAlnum.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func withNetworkHint(link Link, err error) error {
	msg := strings.ToLower(err.Error())
	if link.Kind == KindSoundCloud && (strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout") || strings.Contains(msg, "connection")) {
		return fmt.Errorf("%w — SoundCloud could not be reached; check your connection or VPN", err)
	}
	if strings.Contains(msg, "sign in to confirm") || strings.Contains(msg, "not a bot") {
		return fmt.Errorf("%w — YouTube wants a signed-in session; export cookies to data/cookies.txt", err)
	}
	return err
}
