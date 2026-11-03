package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenamePlaylistFlow(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Focus"); err != nil {
		t.Fatal(err)
	}
	m.focus = focusSidebar
	m.sidebarCursor = 3
	m.view = "Mix"

	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2}); m.modal != modalRenamePlaylist {
		t.Fatalf("modal = %v, want rename playlist", m.modal)
	}
	if got := m.nameInput.Value(); got != "Mix" {
		t.Fatalf("prefilled name = %q, want Mix", got)
	}
	if frame := m.renderModal(110, 32); !strings.Contains(frame, "Rename \"Mix\"") {
		t.Fatalf("rename modal body missing:\n%s", frame)
	}

	m.nameInput.SetValue("Road Trip")
	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); m.modal != modalNone {
		t.Fatalf("modal = %v after save, want none", m.modal)
	}
	if !m.playlists.Has("Road Trip") || m.playlists.Has("Mix") {
		t.Fatalf("playlist did not get renamed: %v", m.playlists.Names())
	}
	if m.view != "Road Trip" {
		t.Fatalf("view = %q, want Road Trip", m.view)
	}

	m.sidebarCursor = 3
	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")}); m.modal != modalRenamePlaylist {
		t.Fatalf("modal = %v, want rename playlist", m.modal)
	}
	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}); m.modal != modalNone {
		t.Fatalf("modal = %v after esc, want none", m.modal)
	}
}

func TestRenamePlaylistValidation(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Focus"); err != nil {
		t.Fatal(err)
	}
	m.focus = focusSidebar
	m.sidebarCursor = 3

	m.openRenamePlaylist()
	m.nameInput.SetValue("focus")
	m.submitPlaylistRename()
	if m.modal != modalRenamePlaylist || m.modalErr == "" {
		t.Fatalf("duplicate rename = modal %v, err %q", m.modal, m.modalErr)
	}

	m.nameInput.SetValue("  ")
	m.submitPlaylistRename()
	if m.modal != modalRenamePlaylist || m.modalErr == "" {
		t.Fatalf("empty rename = modal %v, err %q", m.modal, m.modalErr)
	}
}

func TestRenamePlaylistNeedsSidebar(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	m.focus = focusTracks
	m.sidebarCursor = 3

	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")}); m.modal != modalNone {
		t.Fatalf("modal = %v, want none", m.modal)
	}
	if !strings.Contains(m.toast, "sidebar") {
		t.Fatalf("toast = %q", m.toast)
	}
}
