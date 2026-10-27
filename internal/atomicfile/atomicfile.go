// Package atomicfile writes files through a temporary file and keeps a .bak
// copy of the previous valid content. It is used for small JSON state files
// that must survive crashes and power loss.
package atomicfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

// Write replaces path with data through a temporary file in the same folder.
// The previous file is copied to path+".bak" when it is valid JSON and Write
// is called through WriteJSON; plain Write does not create a backup.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replace(tmpName, path)
}

// WriteJSON marshals v with two-space indentation and writes it atomically.
// If the existing file parses as JSON, it is saved as path+".bak" first.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if old, err := os.ReadFile(path); err == nil && len(old) > 0 {
		var probe any
		if json.Unmarshal(old, &probe) == nil {
			_ = writeBackup(path, old)
		}
	}
	return Write(path, data, 0o644)
}

// ReadJSON reads path. If the primary file is missing or invalid, it tries
// path+".bak"; a successful recovery is written back to the primary path.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return readBackup(path, v)
	}

	if err := json.Unmarshal(data, v); err != nil {
		if backupErr := readBackup(path, v); backupErr == nil {
			return nil
		}
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func readBackup(path string, v any) error {
	bak := path + ".bak"
	data, err := os.ReadFile(bak)
	if err != nil {
		return err
	}
	reset(v)
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(bak), err)
	}
	// Restore the primary file so the next launch starts from a good state.
	_ = Write(path, data, 0o644)
	return nil
}

func reset(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	elem := rv.Elem()
	if elem.IsValid() && elem.CanSet() {
		elem.Set(reflect.Zero(elem.Type()))
	}
}

func writeBackup(path string, data []byte) error {
	tmp := path + ".bak.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := replace(tmp, path+".bak"); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CopyFile copies src to dst atomically. It is used for backups outside the
// JSON helpers and keeps the destination unchanged if reading or writing fails.
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replace(tmpName, dst)
}
