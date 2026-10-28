package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherNotifiesOnLibraryChange(t *testing.T) {
	root := t.TempDir()
	w, err := NewWatcher(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.fsw == nil {
		t.Skip("native filesystem watcher is unavailable; polling fallback is active")
	}

	if err := os.WriteFile(filepath.Join(root, "new.mp3"), []byte("not really audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.Changes:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not report a new file")
	}
}

func TestDirectorySignatureChanges(t *testing.T) {
	root := t.TempDir()
	before := directorySignature(root)
	if err := os.WriteFile(filepath.Join(root, "one.flac"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := directorySignature(root)
	if before == after {
		t.Fatal("signature did not change after adding a supported file")
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	unchanged := directorySignature(root)
	if after != unchanged {
		t.Fatal("unsupported files should not change the poll signature")
	}
}
