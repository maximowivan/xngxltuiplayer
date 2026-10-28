package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
	"tuiplay/internal/updater"
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

func TestTagEditorCoverControls(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32
	m.cursor = 0
	m.openTagEditor()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !m.tags.removeCover {
		t.Fatal("x did not mark the cover for removal")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if m.tags.removeCover {
		t.Fatal("focusing the cover field should keep the existing cover")
	}
	if m.tags.focus != len(m.tags.fields) {
		t.Fatalf("focus = %d, want cover field %d", m.tags.focus, len(m.tags.fields))
	}
	m.tags.cover.SetValue(filepath.Join(t.TempDir(), "missing.jpg"))
	if cmd := m.submitTagEdit(); cmd != nil {
		t.Fatal("missing cover should fail before starting playback save")
	}
	if m.tags.err == "" || !strings.Contains(m.tags.err, "Cover:") {
		t.Fatalf("cover error = %q", m.tags.err)
	}
	assertFrame(t, m, "tag editor")
}

func TestUpdateCheckOpensPromptOnlyForNewerVersion(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.updateVersion = "0.4.0"
	m.handleUpdateChecked(updateCheckedMsg{release: updater.Release{TagName: "v0.4.0"}})
	if m.modal == modalUpdate {
		t.Fatal("same version must not open the update prompt")
	}
	m.handleUpdateChecked(updateCheckedMsg{release: updater.Release{TagName: "v0.4.1", HTMLURL: "https://example.test"}})
	if m.modal != modalUpdate || !m.updateAvailable {
		t.Fatalf("modal=%v available=%v", m.modal, m.updateAvailable)
	}
	assertFrame(t, m, "update prompt")
}

func TestUpdateProgressAndErrorStates(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.updateVersion = "0.4.0"
	m.updateRelease = updater.Release{TagName: "v0.4.1"}
	m.openUpdatePrompt()
	m.updateStaging = true
	m.handleUpdateProgress(updateProgressMsg{progress: updater.Progress{
		Phase: "download", Done: 2 << 20, Total: 4 << 20, Percent: 0.5,
	}})
	if m.updatePhase != "download" || m.updatePercent != 0.5 {
		t.Fatalf("download state = %+v", m)
	}
	assertFrame(t, m, "update progress")
	m.handleUpdateProgress(updateProgressMsg{progress: updater.Progress{Phase: "verify"}})
	if m.updatePhase != "verify" {
		t.Fatalf("phase = %q, want verify", m.updatePhase)
	}
	cmd := m.handleUpdateStageDone(updateStageDoneMsg{err: os.ErrPermission})
	if cmd != nil {
		t.Fatal("failed update should not schedule apply")
	}
	if m.updatePhase != "error" || m.updateErr == "" {
		t.Fatalf("error state = %+v", m)
	}
	assertFrame(t, m, "update error")
}

func TestUpdateStageDoneReady(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.updateRelease = updater.Release{TagName: "v0.4.1"}
	m.openUpdatePrompt()
	m.updateStaging = true
	cmd := m.handleUpdateStageDone(updateStageDoneMsg{stage: t.TempDir()})
	if cmd == nil {
		t.Fatal("successful stage should schedule apply")
	}
	if m.updatePhase != "ready" || m.updateStaging {
		t.Fatalf("ready state = %+v", m)
	}
	assertFrame(t, m, "update ready")
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
