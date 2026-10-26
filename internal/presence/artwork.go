package presence

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type itunesResponse struct {
	Results []itunesSong `json:"results"`
}

type itunesSong struct {
	TrackName       string `json:"trackName"`
	ArtistName      string `json:"artistName"`
	ArtworkURL100   string `json:"artworkUrl100"`
	TrackTimeMillis int64  `json:"trackTimeMillis"`
}

// lookupSourceArtwork recovers the public artwork URL stored with a download.
func lookupSourceArtwork(ctx context.Context, client *http.Client, sourceURL string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return ""
	}
	u, err := url.Parse(sourceURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "open.spotify.com" || host == "play.spotify.com":
		return spotifyThumbnail(ctx, client, sourceURL)
	case host == "youtu.be" || strings.HasSuffix(host, "youtube.com"):
		if id := youtubeID(u); id != "" {
			return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
		}
	case strings.HasSuffix(host, "soundcloud.com"):
		return openGraphImage(ctx, client, sourceURL)
	default:
		return openGraphImage(ctx, client, sourceURL)
	}
	return ""
}

func spotifyThumbnail(ctx context.Context, client *http.Client, sourceURL string) string {
	endpoint := "https://open.spotify.com/oembed?url=" + url.QueryEscape(sourceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "xngxltuiplayer/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.ThumbnailURL)
}

var ogImagePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`),
	regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`),
	regexp.MustCompile(`(?is)<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`),
}

func openGraphImage(ctx context.Context, client *http.Client, pageURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 xngxltuiplayer/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ""
	}
	for _, pattern := range ogImagePatterns {
		if match := pattern.FindSubmatch(body); len(match) == 2 {
			image := html.UnescapeString(string(match[1]))
			if strings.HasPrefix(image, "//") {
				image = "https:" + image
			}
			return strings.TrimSpace(image)
		}
	}
	return ""
}

func youtubeID(u *url.URL) string {
	if u == nil {
		return ""
	}
	if u.Hostname() == "youtu.be" {
		path := strings.Trim(u.Path, "/")
		if path != "" {
			return path
		}
	}
	if id := strings.TrimSpace(u.Query().Get("v")); id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if (part == "shorts" || part == "embed" || part == "live") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// lookupArtwork finds a public square cover URL for a local track. Discord can
// only render remote images, so embedded artwork alone cannot be sent.
func lookupArtwork(ctx context.Context, client *http.Client, title, artist string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	term := title
	if strings.TrimSpace(artist) != "" {
		term = strings.TrimSpace(artist) + " " + title
	}
	endpoint := "https://itunes.apple.com/search?term=" + url.QueryEscape(term) +
		"&entity=song&limit=8&media=music"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "xngxltuiplayer/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var parsed itunesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&parsed); err != nil {
		return ""
	}
	return chooseArtwork(parsed.Results, title, artist)
}

func chooseArtwork(songs []itunesSong, title, artist string) string {
	wantTitle := normalise(title)
	wantArtist := normalise(artist)
	bestScore := -1000
	best := ""
	for _, song := range songs {
		gotTitle := normalise(song.TrackName)
		gotArtist := normalise(song.ArtistName)
		score := 0
		switch {
		case wantTitle != "" && gotTitle == wantTitle:
			score += 100
		case wantTitle != "" && strings.Contains(gotTitle, wantTitle):
			score += 55
		case wantTitle != "" && strings.Contains(wantTitle, gotTitle):
			score += 25
		default:
			score -= 40
		}
		if wantArtist != "" {
			switch {
			case gotArtist == wantArtist:
				score += 50
			case strings.Contains(gotArtist, wantArtist):
				score += 25
			default:
				score -= 15
			}
		}
		lowered := strings.ToLower(song.TrackName)
		for _, bad := range []string{"karaoke", "tribute", "cover version", "instrumental"} {
			if strings.Contains(lowered, bad) && !strings.Contains(strings.ToLower(title), bad) {
				score -= 80
			}
		}
		if song.ArtworkURL100 == "" {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = song.ArtworkURL100
		}
	}
	if bestScore < 0 {
		return ""
	}
	return upgradeArtwork(best)
}

func normalise(s string) string {
	s = strings.ToLower(s)
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
	var b strings.Builder
	space := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func upgradeArtwork(raw string) string {
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"100x100bb", "600x600bb",
		"200x200bb", "600x600bb",
		"300x300bb", "600x600bb",
	)
	return replacer.Replace(raw)
}
