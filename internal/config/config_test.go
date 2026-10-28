package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExplicitDirs(t *testing.T) {
	base := t.TempDir()
	music := filepath.Join(base, "my music")
	data := filepath.Join(base, "my data")
	cfg, err := Load(music, data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MusicDir != filepath.Clean(music) {
		t.Fatalf("MusicDir = %q, want %q", cfg.MusicDir, music)
	}
	if cfg.DataDir != filepath.Clean(data) {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, data)
	}
	if cfg.CoversDir != filepath.Join(filepath.Clean(data), "covers") {
		t.Fatalf("CoversDir = %q", cfg.CoversDir)
	}
	for _, dir := range []string{cfg.MusicDir, cfg.DataDir, cfg.CoversDir, cfg.TempDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("%s was not created: %v", dir, err)
		}
	}
	if err := Writable(cfg.MusicDir); err != nil {
		t.Fatalf("music dir is not writable: %v", err)
	}
}

func TestTemporaryBuildDetection(t *testing.T) {
	if isTemporaryBuild(t.TempDir()) {
		t.Fatal("a portable copy in the temp folder must not be treated as go run")
	}
	buildDir := filepath.Join(os.TempDir(), "go-build123", "b001", "exe")
	if !isTemporaryBuild(buildDir) {
		t.Fatal("go-build directory was not detected")
	}
	if isTemporaryBuild(filepath.Join(os.TempDir(), "go-build")) {
		t.Fatal("a folder named go-build must not be treated as go run")
	}
	if isTemporaryBuild(filepath.Join(os.TempDir(), "go-build", "b001", "app")) {
		t.Fatal("a non-exe go-build folder must not be treated as go run")
	}
}
