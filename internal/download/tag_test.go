package download

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dhowden/tag"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
)

// TestWriteTags checks that the tags we write are readable by dhowden/tag and
// that the embedded cover can be extracted by the library.
func TestWriteTags(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}

	base := t.TempDir()
	wavPath := filepath.Join(base, "in.wav")
	if err := os.WriteFile(wavPath, testWAV(0.4, 440), 0o644); err != nil {
		t.Fatal(err)
	}
	mp3Path := filepath.Join(base, "tagged.mp3")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-i", wavPath, "-c:a", "libmp3lame", "-q:a", "5", mp3Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	cover := testCoverJPEG(t)
	meta := Meta{
		Title:     "Night Drive",
		Artist:    "Aurora",
		Album:     "Neon Skyline",
		Year:      "2024",
		SourceURL: "https://example.com/track",
		CoverURL:  "https://example.com/cover.jpg",
	}
	if err := writeTags(mp3Path, meta, cover); err != nil {
		t.Fatalf("writeTags: %v", err)
	}

	f, err := os.Open(mp3Path)
	if err != nil {
		t.Fatal(err)
	}
	md, err := tag.ReadFrom(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("dhowden/tag cannot read our output: %v", err)
	}
	if md.Title() != meta.Title || md.Artist() != meta.Artist || md.Album() != meta.Album {
		t.Fatalf("metadata = %q / %q / %q", md.Title(), md.Artist(), md.Album())
	}
	if md.Year() != 2024 {
		t.Fatalf("year = %d", md.Year())
	}
	pic := md.Picture()
	if pic == nil || !bytes.Equal(pic.Data, cover) {
		t.Fatal("embedded cover was not preserved")
	}

	lib := library.New(base, filepath.Join(base, "data"))
	tracks, err := lib.Scan()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tr := range tracks {
		if filepath.Clean(tr.Path) == filepath.Clean(mp3Path) {
			found = true
			if tr.CoverURL != meta.CoverURL {
				t.Fatalf("scanned cover URL = %q, want %q", tr.CoverURL, meta.CoverURL)
			}
		}
	}
	if !found {
		t.Fatal("tagged file was not found by the library scan")
	}
	coverPath, err := lib.Cover(model.Track{Path: mp3Path})
	if err != nil {
		t.Fatalf("library.Cover: %v", err)
	}
	cached, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(cached)); err != nil {
		t.Fatalf("cached cover is not JPEG: %v", err)
	}
}

func testCoverJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: 90, B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
