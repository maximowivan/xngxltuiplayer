package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
)

func TestTagEditorOpensForSelectedTrack(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	m.cursor = 0
	if cmd := m.openTagEditor(); cmd == nil {
		t.Fatal("expected focus command")
	}
	if m.modal != modalEditTags || m.tags == nil {
		t.Fatalf("tag editor did not open: modal=%v tags=%v", m.modal, m.tags)
	}
	if got := m.tags.fields[0].input.Value(); got != "Night Drive" {
		t.Fatalf("title field = %q", got)
	}
	assertFrame(t, m, "tag editor")
}

func TestFeatureHotkeys(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	m.cursor = 0
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.modal != modalEditTags {
		t.Fatalf("e opened modal %v, want tags", m.modal)
	}
	m.closeModal()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if m.modal != modalDuplicates {
		t.Fatalf("M opened modal %v, want duplicates", m.modal)
	}
}

func TestDuplicatesModalRendersGroups(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	m.modal = modalDuplicates
	m.dupGroups = []library.DuplicateGroup{{
		Title:  "Night Drive",
		Artist: "Aurora",
		Tracks: []model.Track{
			{Path: `C:\music\a.mp3`, Title: "Night Drive", Artist: "Aurora", Size: 8_000_000, Duration: 3 * time.Minute},
			{Path: `C:\music\b.mp3`, Title: "Night Drive", Artist: "Aurora", Size: 4_000_000, Duration: 3 * time.Minute},
		},
	}}
	m.dupKeeper = 0
	assertFrame(t, m, "duplicates")
}

func TestMergeDuplicateGroupMovesToTrash(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	keepPath := filepath.Join(m.cfg.MusicDir, "keep.mp3")
	dupPath := filepath.Join(m.cfg.MusicDir, "duplicate.mp3")
	for _, path := range []string{keepPath, dupPath} {
		if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keep := model.Track{Path: keepPath, Title: "Night Drive", Artist: "Aurora", Size: 5}
	dup := model.Track{Path: dupPath, Title: "Night Drive", Artist: "Aurora", Size: 5}
	m.tracks = []model.Track{keep, dup}
	m.visible = []model.Track{keep, dup}
	m.dupGroups = []library.DuplicateGroup{{Tracks: []model.Track{keep, dup}}}
	m.dupGroup, m.dupKeeper = 0, 0
	m.modal = modalDuplicates
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.playlists.AddMany("Mix", []string{keepPath, dupPath}); err != nil {
		t.Fatal(err)
	}
	if err := m.stats.SetFavorite(dupPath, true); err != nil {
		t.Fatal(err)
	}

	cmd := m.mergeDuplicateGroup()
	if cmd == nil {
		t.Fatal("merge command is nil")
	}
	msg, ok := cmd().(duplicatesMergedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("merge result = %#v", msg)
	}
	if _, err := os.Stat(dupPath); !os.IsNotExist(err) {
		t.Fatalf("duplicate still exists: %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("keeper missing: %v", err)
	}
	m.handleDuplicatesMerged(msg)
	pl, _ := m.playlists.Get("Mix")
	if len(pl.Tracks) != 1 || pl.Tracks[0] != keepPath {
		t.Fatalf("playlist after merge = %v", pl.Tracks)
	}
	if !m.stats.IsFavorite(keepPath) {
		t.Fatal("favourite was not moved to keeper")
	}
	trash, err := os.ReadDir(filepath.Join(m.cfg.DataDir, "trash"))
	if err != nil || len(trash) != 1 {
		t.Fatalf("trash entries = %v, %v", trash, err)
	}
}
