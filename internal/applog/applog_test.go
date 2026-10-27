package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndWrite(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Printf("hello %s", "world")
	path := Path()
	if path == "" {
		t.Fatal("log path is empty")
	}
	Close()

	data, err := os.ReadFile(filepath.Join(dir, "logs", "xngxltuiplayer.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("log does not contain the message: %q", data)
	}
}

func TestPrintfBeforeInitIsSafe(t *testing.T) {
	Printf("ignored")
}
