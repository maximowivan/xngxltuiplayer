package playlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplacePathEverywhere(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dir, "old.mp3")
	newPath := filepath.Join(dir, "new.mp3")
	if _, _, err := s.AddMany("Mix", []string{oldPath, newPath, oldPath}); err != nil {
		t.Fatal(err)
	}
	changed, err := s.ReplacePathEverywhere(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed == 0 {
		t.Fatal("expected a change")
	}
	pl, ok := s.Get("Mix")
	if !ok {
		t.Fatal("playlist missing")
	}
	if len(pl.Tracks) != 1 || pl.Tracks[0] != newPath {
		t.Fatalf("tracks = %v, want [%s]", pl.Tracks, newPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "playlists.json")); err != nil {
		t.Fatalf("playlist file was not saved: %v", err)
	}
}
