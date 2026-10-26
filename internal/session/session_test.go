package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	state := State{
		Volume:     73,
		Repeat:     "all",
		Shuffle:    true,
		View:       "Road Trip",
		Theme:      "nord",
		Track:      filepath.Join(dir, "a.mp3"),
		PositionMS: 42000,
	}
	if err := Save(dir, state); err != nil {
		t.Fatal(err)
	}
	got := Load(dir)
	if got != state {
		t.Fatalf("loaded %+v, want %+v", got, state)
	}
}

func TestDefaultsAndNormalization(t *testing.T) {
	dir := t.TempDir()
	got := Load(dir)
	def := Default()
	if got != def {
		t.Fatalf("missing file: got %+v, want %+v", got, def)
	}

	state := State{Volume: 200, Repeat: "sometimes", View: "  ", PositionMS: -10}
	if err := Save(dir, state); err != nil {
		t.Fatal(err)
	}
	got = Load(dir)
	if got.Volume != 100 || got.Repeat != "off" || got.View != "all" || got.PositionMS != 0 {
		t.Fatalf("state was not normalized: %+v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir); got != Default() {
		t.Fatalf("invalid JSON should return defaults, got %+v", got)
	}
}
