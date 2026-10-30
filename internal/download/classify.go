package download

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Kind identifies the source of a link.
type Kind int

const (
	KindUnknown Kind = iota
	KindSpotifyTrack
	KindSpotifyShort
	KindSoundCloud
	KindSoundCloudSet
	KindYouTube
	KindYouTubePlaylist
	KindDirect
	KindGeneric
)

func (k Kind) String() string {
	switch k {
	case KindSpotifyTrack, KindSpotifyShort:
		return "Spotify"
	case KindSoundCloud:
		return "SoundCloud"
	case KindSoundCloudSet:
		return "SoundCloud set"
	case KindYouTube:
		return "YouTube"
	case KindYouTubePlaylist:
		return "YouTube playlist"
	case KindDirect:
		return "Direct audio"
	case KindGeneric:
		return "Web"
	default:
		return "Unknown"
	}
}

// Batch reports whether the link resolves to several tracks.
func (k Kind) Batch() bool {
	return k == KindYouTubePlaylist || k == KindSoundCloudSet
}

// Link is a classified, normalized download URL. Seed carries known metadata
// for tracks that came from a playlist or set, so the resolver does not have to
// look the title up again.
type Link struct {
	Raw  string
	URL  string
	Kind Kind
	ID   string
	Seed *Meta
}

var (
	spotifyIDPattern   = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
	youtubeIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youtubeListPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,64}$`)
	directExtensions   = map[string]bool{
		".mp3": true, ".flac": true, ".wav": true,
		".m4a": true, ".aac": true, ".ogg": true,
		".opus": true, ".webm": true, ".wma": true,
	}
)

// Classify validates and normalizes a user-supplied link.
func Classify(raw string) (Link, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "<>")
	if raw == "" {
		return Link{}, errors.New("link is empty")
	}
	if len(raw) > 4096 {
		return Link{}, errors.New("link is too long")
	}

	if strings.HasPrefix(strings.ToLower(raw), "spotify:") {
		parts := strings.Split(raw, ":")
		if len(parts) >= 3 {
			kind := strings.ToLower(strings.TrimSpace(parts[1]))
			id := strings.TrimSpace(parts[2])
			switch kind {
			case "track":
				if !spotifyIDPattern.MatchString(id) {
					return Link{}, errors.New("invalid Spotify track id")
				}
				u := "https://open.spotify.com/track/" + id
				return Link{Raw: raw, URL: u, Kind: KindSpotifyTrack, ID: id}, nil
			case "playlist", "album":
				return Link{}, fmt.Errorf("Spotify %s links are not supported yet — paste a single track, or use a YouTube or SoundCloud playlist", kind)
			}
		}
		return Link{}, errors.New("only individual Spotify tracks are supported (spotify:track:...)")
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Link{}, fmt.Errorf("invalid link: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Link{}, errors.New("only http(s) links are supported")
	}
	u.Scheme = scheme
	if u.Host == "" {
		return Link{}, errors.New("link has no host")
	}

	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	switch {
	case host == "open.spotify.com" || host == "play.spotify.com":
		return classifySpotify(u, raw)
	case host == "spotify.link" || host == "spoti.fi":
		return Link{Raw: raw, URL: u.String(), Kind: KindSpotifyShort}, nil
	case host == "soundcloud.com" || host == "on.soundcloud.com":
		return classifySoundCloud(u, raw)
	case host == "w.soundcloud.com":
		if inner := u.Query().Get("url"); inner != "" {
			return Classify(inner)
		}
		return Link{}, errors.New("SoundCloud widget link has no track URL")
	case host == "youtube.com" || host == "music.youtube.com" || host == "youtu.be" || host == "youtube-nocookie.com":
		return classifyYouTube(u, host, raw)
	}

	ext := strings.ToLower(path.Ext(u.Path))
	if directExtensions[ext] {
		return Link{Raw: raw, URL: u.String(), Kind: KindDirect}, nil
	}
	return Link{Raw: raw, URL: u.String(), Kind: KindGeneric}, nil
}

func classifySpotify(u *url.URL, raw string) (Link, error) {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && strings.HasPrefix(strings.ToLower(parts[0]), "intl-") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return Link{}, errors.New("invalid Spotify link")
	}
	kind := strings.ToLower(parts[0])
	switch kind {
	case "track":
		if len(parts) < 2 || !spotifyIDPattern.MatchString(parts[1]) {
			return Link{}, errors.New("invalid Spotify track link")
		}
		id := parts[1]
		return Link{Raw: raw, URL: "https://open.spotify.com/track/" + id, Kind: KindSpotifyTrack, ID: id}, nil
	case "playlist", "album":
		return Link{}, fmt.Errorf("Spotify %s links are not supported yet — paste a single track, or use a YouTube or SoundCloud playlist", kind)
	case "artist", "show", "episode", "user":
		return Link{}, fmt.Errorf("Spotify %s links are not supported — paste a single track link", kind)
	default:
		return Link{}, errors.New("unsupported Spotify link")
	}
}

func classifySoundCloud(u *url.URL, raw string) (Link, error) {
	host := strings.ToLower(u.Hostname())
	if host == "on.soundcloud.com" {
		return Link{Raw: raw, URL: u.String(), Kind: KindSoundCloud}, nil
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			return Link{}, errors.New("paste a link to a specific SoundCloud track or set")
		}
	}
	if len(segments) == 3 && strings.EqualFold(segments[1], "sets") {
		u.Path = "/" + segments[0] + "/sets/" + segments[2]
		u.RawQuery = ""
		u.Fragment = ""
		return Link{Raw: raw, URL: u.String(), Kind: KindSoundCloudSet, ID: segments[2]}, nil
	}
	if len(segments) >= 2 {
		switch strings.ToLower(segments[1]) {
		case "sets", "albums", "playlists", "tracks", "likes", "reposts", "followers", "following":
			return Link{}, errors.New("paste a link to a specific SoundCloud track or set")
		}
	}
	if len(segments) != 2 {
		return Link{}, errors.New("paste a link to a specific SoundCloud track or set")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return Link{Raw: raw, URL: u.String(), Kind: KindSoundCloud}, nil
}

func classifyYouTube(u *url.URL, host, raw string) (Link, error) {
	p := strings.Trim(u.Path, "/")
	segments := strings.Split(p, "/")
	list := strings.TrimSpace(u.Query().Get("list"))

	if p == "playlist" || p == "embed/videoseries" {
		if list == "" {
			return Link{}, errors.New("YouTube playlist link has no list id")
		}
		if !youtubeListPattern.MatchString(list) {
			return Link{}, errors.New("invalid YouTube playlist id")
		}
		normalized := "https://www.youtube.com/playlist?list=" + list
		return Link{Raw: raw, URL: normalized, Kind: KindYouTubePlaylist, ID: list}, nil
	}
	if len(segments) > 0 && segments[0] == "results" {
		return Link{}, errors.New("YouTube searches are not supported — paste a playlist or video link")
	}
	if strings.HasPrefix(p, "channel/") || strings.HasPrefix(p, "c/") || strings.HasPrefix(p, "@") {
		return Link{}, errors.New("paste a link to a specific YouTube video, not a channel")
	}

	var id string
	switch {
	case host == "youtu.be":
		if len(segments) > 0 {
			id = segments[0]
		}
	case len(segments) >= 2 && (segments[0] == "shorts" || segments[0] == "embed" || segments[0] == "live"):
		id = segments[1]
	default:
		id = u.Query().Get("v")
	}
	id = strings.TrimSpace(id)
	if id == "" && list != "" {
		if !youtubeListPattern.MatchString(list) {
			return Link{}, errors.New("invalid YouTube playlist id")
		}
		normalized := "https://www.youtube.com/playlist?list=" + list
		return Link{Raw: raw, URL: normalized, Kind: KindYouTubePlaylist, ID: list}, nil
	}
	if id == "" {
		return Link{}, errors.New("could not find a video id in the YouTube link")
	}
	if !youtubeIDPattern.MatchString(id) {
		return Link{}, errors.New("invalid YouTube video id")
	}
	normalized := "https://www.youtube.com/watch?v=" + id
	return Link{Raw: raw, URL: normalized, Kind: KindYouTube, ID: id}, nil
}
