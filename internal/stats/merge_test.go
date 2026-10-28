package stats

import (
	"testing"
)

func TestMergeStats(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	oldPath := "old.mp3"
	newPath := "new.mp3"
	if err := s.SetFavorite(oldPath, true); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPlayed(oldPath); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPlayed(oldPath); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPlayed(newPath); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if !s.IsFavorite(newPath) {
		t.Fatal("favourite was not merged")
	}
	if got := s.Plays(newPath); got != 2 {
		t.Fatalf("plays = %d, want 2 (max)", got)
	}
	if got := s.Plays(oldPath); got != 0 {
		t.Fatalf("old path still has %d plays", got)
	}
	if s.Recent(10) == nil {
		t.Fatal("recent list is nil")
	}
}
