package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tuiplay/internal/model"
)

func TestFindDuplicatesByTagsAndDuration(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(musicDir, "a.mp3")
	b := filepath.Join(musicDir, "b.mp3")
	c := filepath.Join(musicDir, "c.mp3")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("same audio bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(c, []byte("different audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := New(musicDir, filepath.Join(base, "data"))
	lib.tracks = []model.Track{
		{Path: a, Title: "Night Drive", Artist: "Aurora", Duration: 3 * time.Minute},
		{Path: b, Title: "night drive!", Artist: "AURORA", Duration: 3*time.Minute + time.Second},
		{Path: c, Title: "Night Drive", Artist: "Aurora", Duration: 4 * time.Minute},
	}
	groups, err := lib.FindDuplicates()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(groups), groups)
	}
	if len(groups[0].Tracks) != 2 {
		t.Fatalf("group tracks = %d, want 2", len(groups[0].Tracks))
	}
	if !groups[0].Exact {
		t.Fatal("identical files should be marked exact")
	}
}

func TestNormalizeDuplicateText(t *testing.T) {
	if got, want := normalizeDuplicateText("  Night-Drive (Remastered)  "), "night drive remastered"; got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}

func TestMoveToTrash(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "duplicate.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := New(musicDir, dataDir)
	lib.tracks = []model.Track{{Path: path, Title: "Night Drive"}}
	dest, err := lib.MoveToTrash(lib.tracks[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original still exists: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "audio" {
		t.Fatalf("trash content = %q, %v", data, err)
	}
	if len(lib.Tracks()) != 0 {
		t.Fatalf("track was not removed from the library: %+v", lib.Tracks())
	}
}
