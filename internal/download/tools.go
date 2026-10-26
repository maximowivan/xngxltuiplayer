package download

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Tools holds paths to external command-line tools.
type Tools struct {
	YtDlp   string
	FFmpeg  string
	FFprobe string
}

// FindTools locates yt-dlp, ffmpeg and ffprobe on PATH.
func FindTools() Tools {
	t := Tools{}
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		t.YtDlp = p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		t.FFmpeg = p
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		t.FFprobe = p
	}
	return t
}

// Missing returns human-readable names of missing required tools.
func (t Tools) Missing(needsYtDlp bool) []string {
	var missing []string
	if needsYtDlp && t.YtDlp == "" {
		missing = append(missing, "yt-dlp")
	}
	if t.FFmpeg == "" {
		missing = append(missing, "ffmpeg")
	}
	return missing
}

// Version runs a tool's version command and returns its first line.
func Version(ctx context.Context, bin string, args ...string) string {
	if bin == "" {
		return "not found"
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "found, version check failed"
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if line == "" {
		return "found"
	}
	return line
}

// InstallHint returns platform-friendly installation advice.
func InstallHint(tool string) string {
	switch tool {
	case "yt-dlp":
		return "install yt-dlp (winget install yt-dlp.yt-dlp) and reopen the app"
	case "ffmpeg":
		return "install ffmpeg (winget install Gyan.FFmpeg) and reopen the app"
	default:
		return fmt.Sprintf("install %s and reopen the app", tool)
	}
}
