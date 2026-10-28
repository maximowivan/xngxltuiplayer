// Package updater downloads and stages the latest Windows release.
package updater

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Owner = "maximowivan"
	Repo  = "xngxltuiplayer"
)

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release is the subset of the GitHub release API used by the updater.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Progress reports a stage of the update download. Done/Total are meaningful
// while Phase is "download"; other phases are indeterminate.
type Progress struct {
	Phase   string
	Done    int64
	Total   int64
	Percent float64
}

// Latest fetches the latest published release.
func Latest(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo), nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", "xngxltuiplayer-updater")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, err
	}
	if rel.TagName == "" {
		return Release{}, errors.New("release has no tag")
	}
	return rel, nil
}

// WindowsAssets returns the zip and its .sha256 companion.
// WindowsAssets returns the zip and its .sha256 companion. Both the current
// v-prefixed asset names and older names without the "v" are accepted.
func (r Release) WindowsAssets() (zip Asset, sha Asset, err error) {
	var fallback Asset
	for _, a := range r.Assets {
		if !strings.HasPrefix(a.Name, "xngxltuiplayer-") || !strings.HasSuffix(a.Name, "-windows-amd64.zip") {
			continue
		}
		if strings.HasPrefix(a.Name, "xngxltuiplayer-v") {
			zip = a
			break
		}
		if fallback.Name == "" {
			fallback = a
		}
	}
	if zip.Name == "" {
		zip = fallback
	}
	if zip.Name == "" {
		return Asset{}, Asset{}, errors.New("release has no Windows amd64 zip")
	}
	for _, a := range r.Assets {
		if a.Name == zip.Name+".sha256" {
			sha = a
			break
		}
	}
	if sha.Name == "" {
		return Asset{}, Asset{}, errors.New("release has no SHA256 file")
	}
	return zip, sha, nil
}

// Stage downloads, verifies and extracts a release into dataDir/tmp.
func Stage(ctx context.Context, dataDir string, rel Release) (string, error) {
	return StageWithProgress(ctx, dataDir, rel, nil)
}

// StageWithProgress is Stage with optional download progress callbacks.
func StageWithProgress(ctx context.Context, dataDir string, rel Release, report func(Progress)) (string, error) {
	zipAsset, shaAsset, err := rel.WindowsAssets()
	if err != nil {
		return "", err
	}

	base := filepath.Join(dataDir, "tmp", "update-"+sanitize(rel.TagName))
	_ = os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(base)
		}
	}()

	zipPath := filepath.Join(base, zipAsset.Name)
	shaPath := zipPath + ".sha256"
	if err := downloadWithProgress(ctx, zipAsset.BrowserDownloadURL, zipPath, zipAsset.Size, report); err != nil {
		return "", fmt.Errorf("download zip: %w", err)
	}
	if err := download(ctx, shaAsset.BrowserDownloadURL, shaPath); err != nil {
		return "", fmt.Errorf("download sha256: %w", err)
	}
	if report != nil {
		report(Progress{Phase: "verify"})
	}
	if err := verifySHA256(zipPath, shaPath); err != nil {
		return "", err
	}

	filesDir := filepath.Join(base, "files")
	if report != nil {
		report(Progress{Phase: "extract"})
	}
	if err := extractZip(zipPath, filesDir); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	if _, err := os.Stat(filepath.Join(filesDir, "xngxltuiplayer.exe")); err != nil {
		return "", fmt.Errorf("archive has no xngxltuiplayer.exe: %w", err)
	}
	if report != nil {
		report(Progress{Phase: "ready", Percent: 1})
	}
	complete = true
	return filesDir, nil
}

func download(ctx context.Context, url, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "xngxltuiplayer-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func downloadWithProgress(ctx context.Context, url, path string, fallbackTotal int64, report func(Progress)) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "xngxltuiplayer-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = fallbackTotal
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	reader := io.Reader(resp.Body)
	if report != nil {
		reader = &progressReader{
			r:      resp.Body,
			total:  total,
			report: report,
		}
	}
	if _, err := io.Copy(out, reader); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

type progressReader struct {
	r      io.Reader
	total  int64
	done   int64
	last   time.Time
	report func(Progress)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	p.done += int64(n)
	if p.report != nil && (p.last.IsZero() || time.Since(p.last) >= 100*time.Millisecond || err == io.EOF) {
		p.last = time.Now()
		percent := 0.0
		if p.total > 0 {
			percent = float64(p.done) / float64(p.total)
			if percent > 1 {
				percent = 1
			}
		}
		p.report(Progress{
			Phase:   "download",
			Done:    p.done,
			Total:   p.total,
			Percent: percent,
		})
	}
	return n, err
}

func verifySHA256(zipPath, shaPath string) error {
	data, err := os.ReadFile(shaPath)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return errors.New("empty SHA256 file")
	}
	want := strings.ToLower(fields[0])
	if len(want) != 64 {
		return fmt.Errorf("invalid SHA256 value %q", want)
	}

	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, want)
	}
	return nil
}

func extractZip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		_ = in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// IsNewer reports whether latest should replace current.
func IsNewer(latest, current string) bool {
	l := versionParts(latest)
	c := versionParts(current)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	// 0.2.0-dev is older than the 0.2.0 release.
	return strings.Contains(current, "-") && !strings.Contains(latest, "-")
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
	if s == "" {
		return "release"
	}
	return s
}
