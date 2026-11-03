package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPreviewWindow(t *testing.T) {
	cases := []struct {
		duration time.Duration
		start    time.Duration
		length   time.Duration
	}{
		{0, 30 * time.Second, 20 * time.Second},
		{10 * time.Second, 0, 10 * time.Second},
		{20 * time.Second, 0, 20 * time.Second},
		{30 * time.Second, 8 * time.Second, 20 * time.Second},
		{3 * time.Minute, 45 * time.Second, 20 * time.Second},
		{60 * time.Minute, 45 * time.Second, 20 * time.Second},
	}
	for _, tc := range cases {
		start, length := previewWindow(tc.duration)
		if start != tc.start || length != tc.length {
			t.Errorf("previewWindow(%v) = %v, %v; want %v, %v", tc.duration, start, length, tc.start, tc.length)
		}
	}
}

func TestPreviewSection(t *testing.T) {
	got := previewSection(45*time.Second, 20*time.Second)
	if got != "*00:00:45-00:01:05" {
		t.Fatalf("previewSection = %q", got)
	}
	if previewLength(10*time.Second) != 10*time.Second || previewLength(time.Minute) != 20*time.Second {
		t.Fatalf("unexpected preview lengths")
	}
}

func TestCloseRemovesPreviewCache(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(Options{
		MusicDir: filepath.Join(base, "music"),
		DataDir:  filepath.Join(base, "data"),
		TempDir:  filepath.Join(base, "tmp"),
	})
	dir := mgr.previewDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clip.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("preview cache still exists after Close: %v", err)
	}
}

func TestPrunePreviewCache(t *testing.T) {
	mgr := newBatchTestManager(t)
	dir := mgr.previewDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxPreviewFiles+3; i++ {
		path := filepath.Join(dir, "clip-"+string(rune('a'+i))+".mp3")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		now := time.Now().Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	mgr.prunePreviewCache()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	if count > maxPreviewFiles {
		t.Fatalf("preview files = %d, want <= %d", count, maxPreviewFiles)
	}
}
