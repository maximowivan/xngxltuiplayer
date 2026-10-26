package download

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36 xngxltuiplayer/1.0"

type progressFunc func(progress float64, detail string)

func (m *Manager) fetchInfo(ctx context.Context, target string, flat bool) (*ytInfo, error) {
	if m.tools.YtDlp == "" {
		return nil, errors.New("yt-dlp is not installed")
	}
	args := []string{"-J", "--no-warnings", "--no-colors", "--no-progress", "--no-playlist",
		"--socket-timeout", "20", "--retries", "3", "--extractor-retries", "2"}
	if flat {
		args = append(args, "--flat-playlist")
	}
	args = append(args, m.cookieArgs()...)
	args = append(args, target)

	cmd := exec.CommandContext(ctx, m.tools.YtDlp, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, toolError("yt-dlp", stderr.String(), err)
	}
	var info ytInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	return &info, nil
}

func (m *Manager) fetchYtDlp(ctx context.Context, target, tmpDir string, progress progressFunc) (string, error) {
	if m.tools.YtDlp == "" {
		return "", errors.New("yt-dlp is not installed")
	}
	args := []string{
		"--no-playlist", "--no-warnings", "--no-colors", "--newline",
		"--socket-timeout", "20", "--retries", "3", "--fragment-retries", "3",
		"--progress-delta", "0.4",
		"--progress-template", "download:TUI %(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s",
		"-f", "bestaudio/best",
		"-o", filepath.Join(tmpDir, "source.%(ext)s"),
	}
	args = append(args, m.cookieArgs()...)
	args = append(args, target)

	cmd := exec.CommandContext(ctx, m.tools.YtDlp, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), "\r"))
		if p, detail, ok := parseProgressLine(line); ok {
			progress(p, detail)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if waitErr != nil {
		return "", toolError("yt-dlp", stderr.String(), waitErr)
	}
	if scanErr != nil {
		return "", fmt.Errorf("read yt-dlp output: %w", scanErr)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", err
	}
	best := ""
	var bestSize int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			best = filepath.Join(tmpDir, name)
			bestSize = info.Size()
		}
	}
	if best == "" {
		return "", errors.New("yt-dlp reported success but produced no audio file")
	}
	return best, nil
}

func (m *Manager) fetchDirect(ctx context.Context, rawURL, tmpDir string, progress progressFunc) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ext := strings.ToLower(path.Ext(resp.Request.URL.Path))
	if ext == "" || len(ext) > 6 {
		ext = ".bin"
	}
	outPath := filepath.Join(tmpDir, "source"+ext)
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 64*1024)
	lastReport := time.Time{}
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			written += int64(n)
			if time.Since(lastReport) > 300*time.Millisecond {
				lastReport = time.Now()
				if total > 0 {
					p := float64(written) / float64(total)
					progress(p, fmt.Sprintf("%s / %s", humanBytes(written), humanBytes(total)))
				} else {
					progress(-1, humanBytes(written)+" downloaded")
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if written == 0 {
		return "", errors.New("downloaded file is empty")
	}
	progress(1, "download complete")
	return outPath, nil
}

func (m *Manager) convert(ctx context.Context, input, output string, duration time.Duration, progress progressFunc) error {
	if m.tools.FFmpeg == "" {
		return errors.New("ffmpeg is not installed")
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", input,
		"-vn", "-map_metadata", "-1",
		"-ar", "48000", "-ac", "2",
		"-c:a", "libmp3lame", "-q:a", "0",
		"-progress", "pipe:1", "-nostats",
		"-f", "mp3", output,
	}
	cmd := exec.CommandContext(ctx, m.tools.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 32*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us":
			us, err := strconv.ParseInt(value, 10, 64)
			if err != nil || duration <= 0 {
				continue
			}
			p := float64(time.Duration(us)*time.Microsecond) / float64(duration)
			if p < 0 {
				p = 0
			}
			if p > 1 {
				p = 1
			}
			progress(p, "transcoding to MP3")
		case "progress":
			if value == "end" {
				progress(1, "transcoding to MP3")
			}
		}
	}
	_ = scanner.Err()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return toolError("ffmpeg", stderr.String(), waitErr)
	}
	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		return errors.New("ffmpeg produced no output")
	}
	return nil
}

func (m *Manager) fetchCover(ctx context.Context, rawURL string) ([]byte, error) {
	if rawURL == "" {
		return nil, errors.New("no cover URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/*")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty cover")
	}
	return data, nil
}

func (m *Manager) cookieArgs() []string {
	if m.opts.CookiesFile == "" {
		return nil
	}
	if info, err := os.Stat(m.opts.CookiesFile); err == nil && info.Size() > 0 {
		return []string{"--cookies", m.opts.CookiesFile}
	}
	return nil
}

var (
	percentLine = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)%`)
	tuiLine     = regexp.MustCompile(`^TUI\s*(.*)$`)
)

func parseProgressLine(line string) (float64, string, bool) {
	body := line
	if match := tuiLine.FindStringSubmatch(line); len(match) == 2 {
		body = match[1]
	} else if !strings.Contains(line, "[download]") && !strings.Contains(line, "%") {
		return 0, "", false
	}

	parts := strings.Split(body, "|")
	p := -1.0
	if len(parts) > 0 {
		if match := percentLine.FindStringSubmatch(parts[0]); len(match) == 2 {
			if v, err := strconv.ParseFloat(match[1], 64); err == nil {
				p = v / 100
				if p < 0 {
					p = 0
				}
				if p > 1 {
					p = 1
				}
			}
		}
	}
	details := make([]string, 0, 2)
	if len(parts) > 1 {
		if s := cleanToolField(parts[1]); s != "" {
			details = append(details, s)
		}
	}
	if len(parts) > 2 {
		if s := cleanToolField(parts[2]); s != "" {
			details = append(details, "ETA "+s)
		}
	}
	detail := strings.Join(details, " · ")
	if detail == "" {
		if p >= 0 {
			detail = fmt.Sprintf("%.0f%% downloaded", p*100)
		} else {
			detail = "downloading audio"
		}
	}
	if p < 0 && !strings.Contains(line, "%") {
		return 0, "", false
	}
	return p, detail, true
}

func cleanToolField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "~")
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "n/a", "unknown", "na":
		return ""
	}
	return s
}

func toolError(tool, stderr string, err error) error {
	msg := sanitizeToolOutput(stderr)
	if msg == "" {
		return fmt.Errorf("%s failed: %w", tool, err)
	}
	return fmt.Errorf("%s: %s", tool, msg)
}

func sanitizeToolOutput(s string) string {
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var useful []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WARNING:") || strings.HasPrefix(line, "[download]") {
			continue
		}
		useful = append(useful, line)
	}
	if len(useful) > 4 {
		useful = useful[len(useful)-4:]
	}
	out := strings.Join(useful, " · ")
	if len(out) > 700 {
		out = out[:700] + "…"
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n >= div*unit && exp < 5 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func titleFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "Downloaded audio"
	}
	name := path.Base(u.Path)
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = strings.TrimSuffix(name, path.Ext(name))
	name = strings.TrimSpace(strings.ReplaceAll(name, "_", " "))
	if name == "" {
		return "Downloaded audio"
	}
	return cleanText(name)
}
