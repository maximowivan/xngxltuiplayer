package download

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

func sanitizeFilename(s string) string {
	s = invalidFilenameChars.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " .")
	if s == "" {
		s = "track"
	}
	runes := []rune(s)
	if len(runes) > 120 {
		s = strings.TrimSpace(string(runes[:120]))
	}
	upper := strings.ToUpper(s)
	reservedBase := upper
	if idx := strings.IndexByte(reservedBase, '.'); idx >= 0 {
		reservedBase = reservedBase[:idx]
	}
	switch reservedBase {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		s = "_" + s
	}
	return s
}

func trackFilename(meta Meta) string {
	title := strings.TrimSpace(meta.Title)
	artist := strings.TrimSpace(meta.Artist)
	if title == "" {
		title = "Untitled"
	}
	if artist != "" && !hasArtistPrefix(title, artist) {
		title = artist + " - " + title
	}
	title = sanitizeFilename(title)
	if !utf8.ValidString(title) {
		title = strings.ToValidUTF8(title, "")
	}
	if title == "" {
		title = "track"
	}
	return title + ".mp3"
}

func hasArtistPrefix(title, artist string) bool {
	for _, sep := range []string{" - ", " – ", " — ", ": "} {
		idx := strings.Index(title, sep)
		if idx > 0 && strings.EqualFold(strings.TrimSpace(title[:idx]), strings.TrimSpace(artist)) {
			return true
		}
	}
	return false
}

func uniqueFilePath(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	if !pathExists(candidate) {
		return candidate
	}
	for i := 2; i < 10000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if !pathExists(candidate) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, os.Getpid(), ext))
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-volume move: copy next to the destination and rename into place so
	// a library scan never sees a partially written audio file.
	tmp := dst + ".tuiplay-part"
	if err := copyFile(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func appendWarning(existing, warning string) string {
	if warning == "" {
		return existing
	}
	if existing == "" {
		return warning
	}
	return existing + "; " + warning
}
