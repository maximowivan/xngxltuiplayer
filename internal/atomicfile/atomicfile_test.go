package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type testState struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

func TestWriteJSONAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	want := testState{Name: "first", Number: 1}
	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got testState
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not exist after the first write: %v", err)
	}
}

func TestWriteJSONCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := WriteJSON(path, testState{Name: "first", Number: 1}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, testState{Name: "second", Number: 2}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	var backup testState
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("parse backup: %v", err)
	}
	if backup.Name != "first" {
		t.Fatalf("backup = %+v, want first", backup)
	}
}

func TestReadJSONRecoversFromCorruptPrimary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := WriteJSON(path, testState{Name: "good", Number: 7}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, testState{Name: "new", Number: 8}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got testState
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON should recover: %v", err)
	}
	if got.Name != "good" || got.Number != 7 {
		t.Fatalf("recovered %+v, want good/7", got)
	}

	// The primary file should be restored from the backup.
	var restored testState
	if err := ReadJSON(path, &restored); err != nil {
		t.Fatalf("ReadJSON after restore: %v", err)
	}
	if restored != got {
		t.Fatalf("restored %+v, want %+v", restored, got)
	}
}

func TestReadJSONMissingReturnsError(t *testing.T) {
	var got testState
	if err := ReadJSON(filepath.Join(t.TempDir(), "missing.json"), &got); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestWriteReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	if err := Write(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}
