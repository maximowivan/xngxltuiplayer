package model

import (
	"fmt"
	"strings"
	"time"
)

// Track is a single audio file known to the library.
type Track struct {
	Path        string        `json:"path"`
	Title       string        `json:"title"`
	Artist      string        `json:"artist"`
	Album       string        `json:"album"`
	Year        string        `json:"year"`
	TrackNumber string        `json:"track_number,omitempty"`
	Duration    time.Duration `json:"duration"`
	CoverURL    string        `json:"cover_url,omitempty"`
	HasArt      bool          `json:"has_art"`
	Size        int64         `json:"size"`
	ModTime     int64         `json:"mod_time"`
	Added       time.Time     `json:"added"`
	Source      string        `json:"source,omitempty"`
}

// DisplayTitle returns a non-empty title.
func (t Track) DisplayTitle() string {
	if s := strings.TrimSpace(t.Title); s != "" {
		return s
	}
	return "Untitled"
}

// DisplayArtist returns a non-empty artist name.
func (t Track) DisplayArtist() string {
	if s := strings.TrimSpace(t.Artist); s != "" {
		return s
	}
	return "Unknown artist"
}

// DisplayAlbum returns a non-empty album name.
func (t Track) DisplayAlbum() string {
	if s := strings.TrimSpace(t.Album); s != "" {
		return s
	}
	return "Unknown album"
}

// Subtitle returns "Artist · Album" for compact displays.
func (t Track) Subtitle() string {
	parts := []string{t.DisplayArtist()}
	if strings.TrimSpace(t.Album) != "" {
		parts = append(parts, t.Album)
	}
	return strings.Join(parts, " · ")
}

// FormatDuration formats a duration as m:ss or h:mm:ss.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "--:--"
	}
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	m := int(d/time.Minute) % 60
	s := int(d/time.Second) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// SortKey is used for stable library ordering.
func (t Track) SortKey() string {
	return strings.ToLower(t.DisplayArtist() + "\x00" + t.DisplayTitle() + "\x00" + t.Path)
}
