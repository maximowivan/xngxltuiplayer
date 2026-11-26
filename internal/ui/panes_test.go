package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestActivePaneColorsFollowFocus(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.focus = focusTracks
	if border, title := m.tracksPaneColors(); border != colAccent || title != colAccent {
		t.Fatalf("tracks pane colors = %v/%v, want accent", border, title)
	}
	if border, _ := m.sidebarPaneColors(); border != colFaint {
		t.Fatalf("idle sidebar border = %v, want faint", border)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusSidebar {
		t.Fatalf("focus = %v after tab, want sidebar", m.focus)
	}
	if border, title := m.sidebarPaneColors(); border != colAccent || title != colAccent {
		t.Fatalf("sidebar pane colors = %v/%v, want accent", border, title)
	}
	if border, _ := m.tracksPaneColors(); border != colFaint {
		t.Fatalf("idle tracks border = %v, want faint", border)
	}
	assertFrame(t, m, "sidebar focus")
}

func TestFavoriteMarkerInTrackList(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32

	rows := m.renderTrackRows(60, 10)
	if strings.Contains(strings.Join(rows, "\n"), iconFavorite) {
		t.Fatal("no track is a favorite yet, but the marker is visible")
	}

	m.cursor = 0
	want := m.visible[0].Title
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if !m.stats.IsFavorite(m.visible[0].Path) {
		t.Fatal("f did not mark the track as a favorite")
	}

	rows = m.renderTrackRows(60, 10)
	joined := strings.Join(rows, "\n")
	if strings.Count(joined, iconFavorite) != 1 {
		t.Fatalf("favorite markers = %d, want 1:\n%s", strings.Count(joined, iconFavorite), joined)
	}
	found := false
	for _, row := range rows {
		if strings.Contains(row, want) && strings.Contains(row, iconFavorite) {
			found = true
		}
	}
	if !found {
		t.Fatalf("favorite marker is not on the %q row:\n%s", want, joined)
	}
	for i, row := range rows[:3] {
		if got := ansi.StringWidth(row); got != 60 {
			t.Fatalf("row %d width = %d, want 60", i, got)
		}
	}
}
