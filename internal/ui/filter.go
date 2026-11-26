package ui

import (
	"fmt"
	"strings"
	"time"

	"tuiplay/internal/model"
)

// Track filters narrow the list to one artist or one album taken from the
// selected track. A filter stacks on top of the current view and the search
// query and lives until it is cleared; it is not written to the session.
const (
	filterKindArtist = "artist"
	filterKindAlbum  = "album"
)

// filterLabel returns a human label such as "artist: Aurora", or "" when no
// filter is active.
func (m *Model) filterLabel() string {
	if m.filterKind == "" || m.filterValue == "" {
		return ""
	}
	return m.filterKind + ": " + m.filterValue
}

// filterValueFor returns the value the given filter kind compares against.
// The display helpers fall back to "Unknown artist" / "Unknown album", so
// untagged tracks group together instead of splitting into empty values.
func filterValueFor(kind string, t model.Track) string {
	if kind == filterKindArtist {
		return cleanText(t.DisplayArtist())
	}
	return cleanText(t.DisplayAlbum())
}

// cycleTrackFilter walks none -> artist -> album -> none using the selected
// track as the source. Pressing "A" repeatedly on the same track narrows the
// list to its artist and then to its album.
func (m *Model) cycleTrackFilter() {
	t, ok := m.selectedTrack()
	if !ok {
		m.setToast("Select a track first", toastInfo, 1500*time.Millisecond)
		return
	}
	artist := filterValueFor(filterKindArtist, t)
	album := filterValueFor(filterKindAlbum, t)
	switch m.filterKind {
	case filterKindArtist:
		// A filter for another artist means the user picked a different track;
		// start over with that track's artist instead of jumping to an album.
		if !strings.EqualFold(m.filterValue, artist) {
			m.setTrackFilter(filterKindArtist, artist)
			return
		}
		if strings.TrimSpace(t.Album) == "" {
			m.clearTrackFilter()
			m.setToast("No album tag"+currentGlyphs.Separator+"filter cleared", toastInfo, 1800*time.Millisecond)
			return
		}
		m.setTrackFilter(filterKindAlbum, album)
	case filterKindAlbum:
		m.clearTrackFilter()
		m.setToast("Filter cleared", toastInfo, 1500*time.Millisecond)
	default:
		m.setTrackFilter(filterKindArtist, artist)
	}
}

// setTrackFilter applies a filter and keeps the cursor on the track it was
// taken from.
func (m *Model) setTrackFilter(kind, value string) {
	anchor := ""
	if t, ok := m.selectedTrack(); ok {
		anchor = t.Path
	}
	m.filterKind = kind
	m.filterValue = value
	m.rebuildVisible(anchor)
	m.setToast(fmt.Sprintf("Filter %s: %s%s%d tracks", kind, value, currentGlyphs.Separator, len(m.visible)), toastInfo, 2*time.Second)
}

// clearTrackFilter removes the filter, if any, and reports whether anything
// changed.
func (m *Model) clearTrackFilter() bool {
	if m.filterKind == "" {
		return false
	}
	m.filterKind = ""
	m.filterValue = ""
	m.rebuildVisible("")
	return true
}

// applyTrackFilter keeps the tracks matching the active filter. The returned
// slice is a copy whenever a filter is active, so sorting it cannot reorder
// the library itself.
func (m *Model) applyTrackFilter(tracks []model.Track) []model.Track {
	if m.filterKind == "" || m.filterValue == "" {
		return tracks
	}
	out := make([]model.Track, 0, len(tracks))
	for _, t := range tracks {
		if strings.EqualFold(filterValueFor(m.filterKind, t), m.filterValue) {
			out = append(out, t)
		}
	}
	return out
}

// browseBase returns the current view with the active filter applied.
func (m *Model) browseBase() []model.Track {
	return m.applyTrackFilter(m.baseTracks())
}
