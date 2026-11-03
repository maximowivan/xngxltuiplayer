package download

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// PreviewSeconds is the length of an audition clip.
	PreviewSeconds  = 20
	maxPreviewFiles = 10
	maxPreviewBytes = 50 << 20
)

// PreparePreview downloads and converts a short preview clip for a search
// result. Clips are cached for the current session under data/tmp/previews.
func (m *Manager) PreparePreview(ctx context.Context, result SearchResult) (string, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := m.checkTools(true); err != nil {
		return "", 0, err
	}
	if strings.TrimSpace(result.URL) == "" {
		return "", 0, errors.New("search result has no URL")
	}
	if result.Kind != KindYouTube && result.Kind != KindSoundCloud {
		return "", 0, errors.New("preview is only available for YouTube and SoundCloud results")
	}

	cacheDir := m.previewDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", 0, err
	}
	key := sha1.Sum([]byte(result.URL))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(key[:8])+".mp3")
	duration := previewLength(result.Duration)
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		return cachePath, duration, nil
	}
	_ = os.Remove(cachePath)

	start, length := previewWindow(result.Duration)
	tmpDir, err := os.MkdirTemp(cacheDir, "preview-job-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmpDir)

	args := []string{
		"--no-playlist", "--no-warnings", "--no-colors", "--no-progress",
		"-f", "bestaudio/best",
		"--download-sections", previewSection(start, length),
		"--force-keyframes-at-cuts",
		"--socket-timeout", "20", "--retries", "2",
		"-o", filepath.Join(tmpDir, "source.%(ext)s"),
	}
	args = append(args, m.cookieArgs()...)
	args = append(args, result.URL)

	cmd := exec.CommandContext(ctx, m.tools.YtDlp, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		return "", 0, toolError("yt-dlp preview", stderr.String(), err)
	}

	source, err := largestFile(tmpDir)
	if err != nil {
		return "", 0, err
	}
	partPath := cachePath + ".part"
	convertArgs := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", source, "-vn",
		"-ar", "48000", "-ac", "2",
		"-c:a", "libmp3lame", "-b:a", "128k",
		"-t", strconv.Itoa(int(length.Seconds())),
		"-f", "mp3", partPath,
	}
	ffmpeg := exec.CommandContext(ctx, m.tools.FFmpeg, convertArgs...)
	var ffErr strings.Builder
	ffmpeg.Stderr = &ffErr
	if err := ffmpeg.Run(); err != nil {
		_ = os.Remove(partPath)
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		return "", 0, toolError("ffmpeg preview", ffErr.String(), err)
	}
	if err := os.Rename(partPath, cachePath); err != nil {
		_ = os.Remove(partPath)
		return "", 0, err
	}
	if probed := m.probeDuration(ctx, cachePath); probed > 0 {
		duration = probed
	}
	m.prunePreviewCache()
	return cachePath, duration, nil
}

func (m *Manager) previewDir() string {
	return filepath.Join(m.opts.TempDir, "previews")
}

func (m *Manager) cleanPreviewCache() {
	m.removePreviewCache()
	_ = os.MkdirAll(m.previewDir(), 0o755)
}

func (m *Manager) removePreviewCache() {
	_ = os.RemoveAll(m.previewDir())
}

func (m *Manager) prunePreviewCache() {
	dir := m.previewDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type cached struct {
		path    string
		size    int64
		modTime time.Time
	}
	files := make([]cached, 0, len(entries))
	total := int64(0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, cached{
			path:    filepath.Join(dir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		total += info.Size()
	}
	if len(files) <= maxPreviewFiles && total <= maxPreviewBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for len(files) > maxPreviewFiles || total > maxPreviewBytes {
		file := files[0]
		files = files[1:]
		if err := os.Remove(file.path); err == nil {
			total -= file.size
		}
	}
}

func (m *Manager) probeDuration(ctx context.Context, path string) time.Duration {
	if m.tools.FFprobe == "" {
		return 0
	}
	cmd := exec.CommandContext(ctx, m.tools.FFprobe,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func previewWindow(trackDuration time.Duration) (start, length time.Duration) {
	length = PreviewSeconds * time.Second
	if trackDuration <= 0 {
		return 30 * time.Second, length
	}
	if trackDuration <= length+2*time.Second {
		return 0, trackDuration
	}
	start = trackDuration / 4
	if start < 10*time.Second {
		start = 10 * time.Second
	}
	if start > 45*time.Second {
		start = 45 * time.Second
	}
	if trackDuration > 0 && start+length > trackDuration-2*time.Second {
		start = trackDuration - length - 2*time.Second
		if start < 0 {
			start = 0
		}
	}
	return start, length
}

func previewLength(trackDuration time.Duration) time.Duration {
	if trackDuration > 0 && trackDuration < PreviewSeconds*time.Second {
		return trackDuration
	}
	return PreviewSeconds * time.Second
}

func previewSection(start, length time.Duration) string {
	return "*" + sectionTime(start) + "-" + sectionTime(start+length)
}

func sectionTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60)
}

func largestFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	best := ""
	var bestSize int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			best = filepath.Join(dir, name)
			bestSize = info.Size()
		}
	}
	if best == "" {
		return "", errors.New("preview download produced no audio file")
	}
	return best, nil
}
