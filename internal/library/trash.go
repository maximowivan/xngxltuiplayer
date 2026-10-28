package library

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tuiplay/internal/model"
)

// MoveToTrash moves a duplicate out of the music folder into data/trash so a
// merge is reversible. The original path is removed from the in-memory track
// list and its cover cache is invalidated.
func (l *Library) MoveToTrash(track model.Track) (string, error) {
	l.scanMu.Lock()
	defer l.scanMu.Unlock()

	trashDir := filepath.Join(l.DataDir, "trash")
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return "", err
	}
	dest := uniqueTrashPath(trashDir, track.Path)

	info, err := os.Stat(track.Path)
	if err != nil {
		return "", err
	}
	if err := os.Rename(track.Path, dest); err != nil {
		if copyErr := copyTrashFile(track.Path, dest, info.Mode().Perm()); copyErr != nil {
			return "", err
		}
		if removeErr := os.Remove(track.Path); removeErr != nil {
			return "", removeErr
		}
	}
	_ = os.Remove(CoverPath(l.DataDir, track.Path))

	l.mu.Lock()
	out := l.tracks[:0]
	for _, t := range l.tracks {
		if filepath.Clean(t.Path) != filepath.Clean(track.Path) {
			out = append(out, t)
		}
	}
	l.tracks = out
	l.mu.Unlock()
	return dest, nil
}

func uniqueTrashPath(dir, source string) string {
	base := time.Now().Format("20060102-150405") + "-" + filepath.Base(source)
	path := filepath.Join(dir, base)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 2; ; i++ {
		candidate := filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+"-"+strconv.Itoa(i)+filepath.Ext(base))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func copyTrashFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}
