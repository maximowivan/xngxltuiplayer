package stats

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFavoriteAndPlayCountersPersist(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")

	if err := s.SetFavorite(a, true); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPlayed(a); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPlayed(b); err != nil {
		t.Fatal(err)
	}

	reloaded := Load(dir)
	if !reloaded.IsFavorite(a) {
		t.Fatal("favorite was not persisted")
	}
	if reloaded.Plays(a) != 1 {
		t.Fatalf("plays(a) = %d, want 1", reloaded.Plays(a))
	}
	if reloaded.FavoriteCount() != 1 {
		t.Fatalf("favorite count = %d, want 1", reloaded.FavoriteCount())
	}
	if got := reloaded.Recent(10); len(got) != 2 {
		t.Fatalf("recent = %v, want 2 entries", got)
	}
}

func TestToggleFavorite(t *testing.T) {
	s := Load(t.TempDir())
	path := filepath.Join(t.TempDir(), "x.mp3")

	on, err := s.ToggleFavorite(path)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("first toggle should enable favourite")
	}
	off, err := s.ToggleFavorite(path)
	if err != nil {
		t.Fatal(err)
	}
	if off {
		t.Fatal("second toggle should disable favourite")
	}
	if s.FavoriteCount() != 0 {
		t.Fatalf("favorite count = %d, want 0", s.FavoriteCount())
	}
}

func TestRecentOrder(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")

	if err := s.MarkPlayed(a); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := s.MarkPlayed(b); err != nil {
		t.Fatal(err)
	}
	got := s.Recent(1)
	if len(got) != 1 || got[0] != pathKey(b) {
		t.Fatalf("recent(1) = %v, want %q", got, pathKey(b))
	}
}
