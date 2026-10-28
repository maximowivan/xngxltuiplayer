// Package library scans the music folder and maintains a small metadata cache.
package library

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"tuiplay/internal/atomicfile"
	"tuiplay/internal/model"
)

// ErrNoArtwork is returned when a track has no embedded cover.
var ErrNoArtwork = errors.New("track has no embedded artwork")

var audioExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".wav":  true,
}

// IsSupported reports whether the player can decode the file.
func IsSupported(path string) bool {
	return audioExtensions[strings.ToLower(filepath.Ext(path))]
}

// Library owns the music directory and its metadata cache.
type Library struct {
	MusicDir string
	DataDir  string

	mu     sync.Mutex
	scanMu sync.Mutex
	tracks []model.Track
}

// New creates a library rooted at musicDir with cache data in dataDir.
func New(musicDir, dataDir string) *Library {
	return &Library{MusicDir: musicDir, DataDir: dataDir}
}

const cacheVersion = 3

type cacheFile struct {
	Version int           `json:"version"`
	Tracks  []model.Track `json:"tracks"`
}

// Tracks returns a copy of the last scan result.
func (l *Library) Tracks() []model.Track {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]model.Track(nil), l.tracks...)
}

// Scan walks the music directory, reads tags for new or changed files and
// probes their durations. Results are sorted by artist and title.
func (l *Library) Scan() ([]model.Track, error) {
	l.scanMu.Lock()
	defer l.scanMu.Unlock()

	cache := l.loadCache()
	cached := make(map[string]model.Track, len(cache.Tracks))
	for _, t := range cache.Tracks {
		cached[filepath.Clean(t.Path)] = t
	}

	var tracks []model.Track
	var needProbe []int
	seen := make(map[string]bool)

	err := filepath.WalkDir(l.MusicDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != l.MusicDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsSupported(path) || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			return nil
		}
		seen[abs] = true

		if old, ok := cached[abs]; ok && old.Size == info.Size() && old.ModTime == info.ModTime().UnixNano() {
			if old.Added.IsZero() {
				old.Added = info.ModTime()
			}
			if old.Title == "" {
				old.Title = strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			}
			if old.Duration <= 0 {
				needProbe = append(needProbe, len(tracks))
			}
			tracks = append(tracks, old)
			return nil
		}

		t := readTrackMetadata(abs, info)
		needProbe = append(needProbe, len(tracks))
		tracks = append(tracks, t)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan music folder: %w", err)
	}

	if len(needProbe) > 0 {
		probeDurations(tracks, needProbe)
	}

	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].SortKey() < tracks[j].SortKey()
	})

	l.mu.Lock()
	l.tracks = append([]model.Track(nil), tracks...)
	l.mu.Unlock()

	_ = l.saveCache(tracks)
	return append([]model.Track(nil), tracks...), nil
}

func (l *Library) loadCache() cacheFile {
	var cache cacheFile
	if err := atomicfile.ReadJSON(filepath.Join(l.DataDir, "library.json"), &cache); err != nil {
		return cacheFile{}
	}
	if cache.Version != cacheVersion {
		return cacheFile{}
	}
	return cache
}

func (l *Library) saveCache(tracks []model.Track) error {
	if err := os.MkdirAll(l.DataDir, 0o755); err != nil {
		return err
	}
	return atomicfile.WriteJSON(filepath.Join(l.DataDir, "library.json"), cacheFile{Version: cacheVersion, Tracks: tracks})
}

func readTrackMetadata(path string, info os.FileInfo) model.Track {
	fallbackArtist, fallbackTitle := parseFilename(filepath.Base(path))
	t := model.Track{
		Path:    path,
		Title:   fallbackTitle,
		Artist:  fallbackArtist,
		Size:    info.Size(),
		ModTime: info.ModTime().UnixNano(),
		Added:   info.ModTime(),
	}

	f, err := os.Open(path)
	if err == nil {
		md, mdErr := tag.ReadFrom(f)
		_ = f.Close()
		primaryOK := mdErr == nil && md != nil
		if primaryOK {
			if s := strings.TrimSpace(md.Title()); s != "" {
				t.Title = s
			}
			if s := strings.TrimSpace(md.Artist()); s != "" {
				t.Artist = s
			}
			if s := strings.TrimSpace(md.Album()); s != "" {
				t.Album = s
			}
			if y := md.Year(); y > 0 {
				t.Year = strconv.Itoa(y)
			}
			if n, _ := md.Track(); n > 0 {
				t.TrackNumber = strconv.Itoa(n)
			}
			t.HasArt = md.Picture() != nil
		}
		if strings.EqualFold(filepath.Ext(path), ".mp3") {
			// bogem/id3v2 also exposes custom TXXX fields such as COVER_URL,
			// and handles UTF-16 frames that dhowden/tag cannot decode.
			if im, idErr := readID3v2(path); idErr == nil {
				if !primaryOK {
					if im.Title != "" {
						t.Title = im.Title
					}
					if im.Artist != "" {
						t.Artist = im.Artist
					}
					if im.Album != "" {
						t.Album = im.Album
					}
					if im.Year != "" {
						t.Year = im.Year
					}
					if im.TrackNumber != "" {
						t.TrackNumber = im.TrackNumber
					}
					t.HasArt = im.HasArt
				}
				if im.CoverURL != "" {
					t.CoverURL = im.CoverURL
				}
				if im.SourceURL != "" {
					t.Source = im.SourceURL
				}
			}
		}
	}
	return t
}

var trackNumberPrefix = regexp.MustCompile(`^\d{1,3}\s*[\.\-_]\s+`)

func parseFilename(name string) (artist, title string) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = trackNumberPrefix.ReplaceAllString(base, "")
	base = strings.TrimSpace(base)
	if base == "" {
		return "", "Untitled"
	}
	if idx := strings.Index(base, " - "); idx > 0 && idx < len(base)-3 {
		return strings.TrimSpace(base[:idx]), strings.TrimSpace(base[idx+3:])
	}
	return "", base
}

func probeDurations(tracks []model.Track, indexes []int) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return
	}
	jobs := make(chan int, len(indexes))
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				d := probeDuration(ctx, ffprobe, tracks[idx].Path)
				cancel()
				if d > 0 {
					tracks[idx].Duration = d
				}
			}
		}()
	}
	for _, idx := range indexes {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
}

func probeDuration(ctx context.Context, ffprobe, path string) time.Duration {
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(string(out))
	if text == "" || text == "N/A" {
		return 0
	}
	seconds, err := strconv.ParseFloat(text, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

// DeleteFile removes a track and its cached cover.
func (l *Library) DeleteFile(track model.Track) error {
	l.scanMu.Lock()
	defer l.scanMu.Unlock()

	if err := os.Remove(track.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(CoverPath(l.DataDir, track.Path))

	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.tracks[:0]
	for _, t := range l.tracks {
		if filepath.Clean(t.Path) != filepath.Clean(track.Path) {
			out = append(out, t)
		}
	}
	l.tracks = out
	return nil
}

// CoverPath returns the deterministic cache path for a track cover.
func CoverPath(dataDir, trackPath string) string {
	sum := sha1.Sum([]byte(filepath.Clean(trackPath)))
	return filepath.Join(dataDir, "covers", hex.EncodeToString(sum[:])+".jpg")
}

// Cover returns a JPEG cover for the track, extracting it from the audio file
// when needed.
func (l *Library) Cover(track model.Track) (string, error) {
	out := CoverPath(l.DataDir, track.Path)
	if info, err := os.Stat(out); err == nil && info.Size() > 0 {
		if audioInfo, err := os.Stat(track.Path); err != nil || !audioInfo.ModTime().After(info.ModTime()) {
			return out, nil
		}
	}

	f, err := os.Open(track.Path)
	if err != nil {
		return "", err
	}
	md, err := tag.ReadFrom(f)
	_ = f.Close()

	var picData []byte
	if err == nil && md != nil {
		if pic := md.Picture(); pic != nil {
			picData = pic.Data
		}
	}
	if len(picData) == 0 && strings.EqualFold(filepath.Ext(track.Path), ".mp3") {
		// dhowden/tag cannot decode some UTF-16 ID3v2 files, so retry with
		// bogem/id3v2 before giving up on the artwork.
		if data, idErr := id3v2Picture(track.Path); idErr == nil {
			picData = data
		}
	}
	if len(picData) == 0 {
		return "", ErrNoArtwork
	}
	img, _, err := image.Decode(bytes.NewReader(picData))
	if err != nil {
		return "", fmt.Errorf("decode embedded artwork: %w", err)
	}
	jpegData, err := encodeCoverJPEG(img)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, jpegData, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(out)
		if err := os.Rename(tmp, out); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	return out, nil
}

// NormalizeCover decodes arbitrary image bytes and returns a square JPEG.
func NormalizeCover(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode cover: %w", err)
	}
	return encodeCoverJPEG(img)
}

// WriteCoverJPEG stores already-normalized JPEG bytes in the cover cache.
func WriteCoverJPEG(dataDir, trackPath string, jpegData []byte) (string, error) {
	out := CoverPath(dataDir, trackPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, jpegData, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(out)
		if err := os.Rename(tmp, out); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	return out, nil
}

// SaveCoverBytes converts raw image data to a normalized JPEG cover in the
// cache and returns its path.
func SaveCoverBytes(dataDir, trackPath string, data []byte) (string, error) {
	jpegData, err := NormalizeCover(data)
	if err != nil {
		return "", err
	}
	return WriteCoverJPEG(dataDir, trackPath, jpegData)
}

// EncodeCoverJPEG center-crops and resizes an image, returning JPEG bytes.
func EncodeCoverJPEG(img image.Image) ([]byte, error) {
	return encodeCoverJPEG(img)
}

func encodeCoverJPEG(img image.Image) ([]byte, error) {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, errors.New("empty image")
	}

	// Center-crop to a square so video thumbnails look like album art.
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	crop := image.Rect(0, 0, side, side)
	crop = crop.Add(image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2))

	target := side
	if target > 900 {
		target = 900
	}
	dst := image.NewRGBA(image.Rect(0, 0, target, target))
	bg := color.RGBA{R: 20, G: 20, B: 30, A: 255}
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, xdraw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
