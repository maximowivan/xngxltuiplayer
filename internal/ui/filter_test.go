package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/model"
)

// addFilterTrack appends one more track to the test library and rebuilds the
// visible list. The test model starts with three tracks from three artists.
func addFilterTrack(t *testing.T, m *Model, title, artist, album string) model.Track {
	t.Helper()
	tr := model.Track{
		Path:     filepath.Join(m.cfg.MusicDir, title+".mp3"),
		Title:    title,
		Artist:   artist,
		Album:    album,
		Duration: 2 * time.Minute,
	}
	m.tracks = append(m.tracks, tr)
	m.rebuildVisible("")
	return tr
}

// cursorToPath moves the list cursor onto the track with the given path.
func cursorToPath(t *testing.T, m *Model, path string) {
	t.Helper()
	for i, tr := range m.visible {
		if tr.Path == path {
			m.cursor = i
			m.clampListOffset()
			return
		}
	}
	t.Fatalf("track %q is not visible", path)
}

func TestTrackFilterCyclesArtistAlbumOff(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32

	addFilterTrack(t, m, "Binary Dawn", "Aurora", "Binary Dawn")
	first := m.tracks[0]
	cursorToPath(t, m, first.Path)

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if m.filterKind != filterKindArtist || m.filterValue != "Aurora" {
		t.Fatalf("filter = %q:%q, want artist:Aurora", m.filterKind, m.filterValue)
	}
	if len(m.visible) != 2 {
		t.Fatalf("artist filter left %d tracks, want 2", len(m.visible))
	}
	if m.visible[m.cursor].Path != first.Path {
		t.Fatalf("cursor moved to %q, want %q", m.visible[m.cursor].Path, first.Path)
	}
	assertFrame(t, m, "artist filter")

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if m.filterKind != filterKindAlbum || m.filterValue != "Neon Skyline" {
		t.Fatalf("filter = %q:%q, want album:Neon Skyline", m.filterKind, m.filterValue)
	}
	if len(m.visible) != 1 || m.visible[0].Path != first.Path {
		t.Fatalf("album filter left %d tracks", len(m.visible))
	}
	assertFrame(t, m, "album filter")

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if m.filterKind != "" || m.filterValue != "" {
		t.Fatalf("filter = %q:%q, want cleared", m.filterKind, m.filterValue)
	}
	if len(m.visible) != len(m.tracks) {
		t.Fatalf("cleared filter left %d tracks, want %d", len(m.visible), len(m.tracks))
	}
}

func TestTrackFilterShowsCountsAndLabel(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	addFilterTrack(t, m, "Binary Dawn", "Aurora", "Binary Dawn")
	cursorToPath(t, m, m.tracks[0].Path)
	m.cycleTrackFilter()

	view := m.View()
	if !strings.Contains(view, "artist: Aurora") {
		t.Fatalf("title does not show the filter:\n%s", view)
	}
	if !strings.Contains(view, "2 of 4 tracks") {
		t.Fatalf("footer does not show the filtered count:\n%s", view)
	}
}

func TestEscClearsTrackFilter(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	cursorToPath(t, m, m.tracks[0].Path)
	m.cycleTrackFilter()
	if m.filterKind == "" {
		t.Fatal("filter was not applied")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filterKind != "" {
		t.Fatalf("esc left filter %q active", m.filterKind)
	}
	if m.search != "" {
		t.Fatalf("esc should not touch the search query, got %q", m.search)
	}
	if len(m.visible) != len(m.tracks) {
		t.Fatalf("visible = %d tracks after clearing", len(m.visible))
	}
}

func TestTrackFilterCombinesWithSearch(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	addFilterTrack(t, m, "Binary Dawn", "Aurora", "Binary Dawn")
	cursorToPath(t, m, m.tracks[0].Path)
	m.cycleTrackFilter()
	if len(m.visible) != 2 {
		t.Fatalf("artist filter left %d tracks", len(m.visible))
	}
	m.search = "binary"
	m.rebuildVisible("")
	if len(m.visible) != 1 || m.visible[0].Title != "Binary Dawn" {
		t.Fatalf("search over filter = %d tracks", len(m.visible))
	}
}

func TestTrackFilterEmptyResultMessage(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	// Force a filter value that matches nothing to exercise the empty state.
	m.filterKind = filterKindArtist
	m.filterValue = "Nobody"
	m.rebuildVisible("")
	if len(m.visible) != 0 {
		t.Fatalf("visible = %d tracks, want 0", len(m.visible))
	}
	view := m.View()
	if !strings.Contains(view, "No tracks with this artist.") {
		t.Fatalf("empty state does not mention the filter:\n%s", view)
	}
}
