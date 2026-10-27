// Package m3u reads and writes extended M3U playlists.
package m3u

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tuiplay/internal/atomicfile"
	"tuiplay/internal/model"
)

// Entry is one track line from an M3U file.
type Entry struct {
	Path     string
	Title    string
	Artist   string
	Duration time.Duration
}

// ParseFile reads an M3U or M3U8 file. Relative paths are resolved against the
// playlist's folder.
func ParseFile(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, filepath.Dir(path))
}

// Parse reads M3U data. baseDir is used for relative track paths.
func Parse(data []byte, baseDir string) ([]Entry, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var entries []Entry
	var pending *Entry
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(line), "#EXTINF:") {
			pending = parseEXTINF(line)
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		path, err := resolvePath(line, baseDir)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		entry := Entry{Path: path}
		if pending != nil {
			entry.Title = pending.Title
			entry.Artist = pending.Artist
			entry.Duration = pending.Duration
			pending = nil
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func parseEXTINF(line string) *Entry {
	rest := line[len("#EXTINF:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return &Entry{}
	}
	meta := strings.TrimSpace(rest[comma+1:])
	entry := &Entry{}
	if idx := strings.Index(meta, " - "); idx > 0 {
		entry.Artist = strings.TrimSpace(meta[:idx])
		entry.Title = strings.TrimSpace(meta[idx+3:])
	} else {
		entry.Title = meta
	}
	seconds, _ := strconv.ParseFloat(strings.TrimSpace(rest[:comma]), 64)
	if seconds > 0 {
		entry.Duration = time.Duration(seconds * float64(time.Second))
	}
	return entry
}

func resolvePath(line, baseDir string) (string, error) {
	if strings.Contains(line, "://") {
		return "", fmt.Errorf("remote URLs are not supported in local playlists: %s", line)
	}
	line = strings.Trim(line, "\"'")
	line = filepath.FromSlash(line)
	if !filepath.IsAbs(line) {
		line = filepath.Join(baseDir, line)
	}
	return filepath.Clean(line), nil
}

// WriteFile writes tracks as an extended M3U8 playlist. Paths are written
// relative to the playlist when possible, which keeps a playlist portable
// together with the music folder.
func WriteFile(path string, tracks []model.Track) error {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	baseDir := filepath.Dir(path)
	for _, t := range tracks {
		seconds := int(t.Duration / time.Second)
		if seconds < 0 {
			seconds = 0
		}
		title := t.DisplayTitle()
		if strings.TrimSpace(t.Artist) != "" {
			title = t.DisplayArtist() + " - " + title
		}
		fmt.Fprintf(&b, "#EXTINF:%d,%s\n", seconds, title)

		trackPath := filepath.Clean(t.Path)
		if rel, err := filepath.Rel(baseDir, trackPath); err == nil && !strings.HasPrefix(rel, "..") {
			trackPath = rel
		}
		b.WriteString(filepath.ToSlash(trackPath))
		b.WriteByte('\n')
	}
	if b.Len() == len("#EXTM3U\n") {
		return errors.New("playlist is empty")
	}
	return atomicfile.Write(path, []byte(b.String()), 0o644)
}
