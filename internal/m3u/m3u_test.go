package m3u

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tuiplay/internal/model"
)

func TestParseExtendedM3U(t *testing.T) {
	base := t.TempDir()
	data := "\uFEFF#EXTM3U\n" +
		"#EXTINF:123,Aurora - Night Drive\n" +
		"a.mp3\n" +
		"#EXTINF:42.5,No Artist\n" +
		filepath.Join(base, "sub", "b.flac") + "\n" +
		"# comment\n"

	entries, err := Parse([]byte(data), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Path != filepath.Join(base, "a.mp3") {
		t.Fatalf("first path = %q", entries[0].Path)
	}
	if entries[0].Artist != "Aurora" || entries[0].Title != "Night Drive" {
		t.Fatalf("metadata = %+v", entries[0])
	}
	if entries[0].Duration != 123*time.Second {
		t.Fatalf("duration = %v", entries[0].Duration)
	}
	if entries[1].Title != "No Artist" || entries[1].Duration != 42*time.Second+500*time.Millisecond {
		t.Fatalf("second entry = %+v", entries[1])
	}
}

func TestParseRejectsRemoteURLs(t *testing.T) {
	_, err := Parse([]byte("https://example.com/a.mp3\n"), t.TempDir())
	if err == nil {
		t.Fatal("expected an error for a remote URL")
	}
}

func TestWriteAndParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	music := filepath.Join(dir, "music")
	if err := os.MkdirAll(music, 0o755); err != nil {
		t.Fatal(err)
	}
	trackPath := filepath.Join(music, "Aurora - Night Drive.mp3")
	if err := os.WriteFile(trackPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	playlist := filepath.Join(music, "road.m3u8")
	tracks := []model.Track{{
		Path:     trackPath,
		Title:    "Night Drive",
		Artist:   "Aurora",
		Duration: 3*time.Minute + 21*time.Second,
	}}
	if err := WriteFile(playlist, tracks); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(playlist)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Aurora - Night Drive") {
		t.Fatalf("missing metadata: %s", data)
	}
	if !strings.Contains(string(data), "Aurora - Night Drive.mp3") {
		t.Fatalf("path should be relative: %s", data)
	}

	entries, err := ParseFile(playlist)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != trackPath {
		t.Fatalf("round trip = %+v", entries)
	}
}

func TestWriteEmptyPlaylist(t *testing.T) {
	err := WriteFile(filepath.Join(t.TempDir(), "empty.m3u"), nil)
	if err == nil {
		t.Fatal("expected an error for an empty playlist")
	}
}
