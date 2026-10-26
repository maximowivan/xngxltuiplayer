package library

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bogem/id3v2/v2"
)

// TestUTF16TagFallback makes sure the id3v2 fallback can read files written
// with UTF-16 frames, which dhowden/tag cannot always decode.
func TestUTF16TagFallback(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}

	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(base, "in.wav")
	if err := os.WriteFile(wavPath, testWAV(0.5, 440), 0o644); err != nil {
		t.Fatal(err)
	}
	mp3Path := filepath.Join(musicDir, "fallback.mp3")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-i", wavPath, "-c:a", "libmp3lame", "-q:a", "5", mp3Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	cover := testJPEG(t)
	tag, err := id3v2.Open(mp3Path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	tag.SetVersion(3)
	tag.SetDefaultEncoding(id3v2.EncodingUTF16)
	tag.DeleteAllFrames()
	tag.SetTitle("Ночная поездка")
	tag.SetArtist("Аврора")
	tag.SetAlbum("Неоновый горизонт")
	tag.SetYear("2024")
	tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF16,
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "Обложка",
		Picture:     cover,
	})
	if err := tag.Save(); err != nil {
		t.Fatal(err)
	}
	if err := tag.Close(); err != nil {
		t.Fatal(err)
	}

	meta, err := readID3v2(mp3Path)
	if err != nil {
		t.Fatalf("readID3v2: %v", err)
	}
	if meta.Title != "Ночная поездка" {
		t.Fatalf("title = %q", meta.Title)
	}
	if meta.Artist != "Аврора" || meta.Album != "Неоновый горизонт" {
		t.Fatalf("artist/album = %q / %q", meta.Artist, meta.Album)
	}
	if meta.Year != "2024" {
		t.Fatalf("year = %q", meta.Year)
	}
	if !meta.HasArt {
		t.Fatal("fallback did not detect artwork")
	}

	gotCover, err := id3v2Picture(mp3Path)
	if err != nil {
		t.Fatalf("id3v2Picture: %v", err)
	}
	if !bytes.Equal(gotCover, cover) {
		t.Fatalf("cover bytes differ: got %d, want %d", len(gotCover), len(cover))
	}

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	if tracks[0].Title != "Ночная поездка" || tracks[0].Artist != "Аврора" {
		t.Fatalf("scan did not use the fallback: %+v", tracks[0])
	}
	if !tracks[0].HasArt {
		t.Fatal("scan did not report artwork")
	}

	coverPath, err := lib.Cover(tracks[0])
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	info, err := os.Stat(coverPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("cover cache was not written: %v", err)
	}
	f, err := os.Open(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("cached cover is not a valid image: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 200 {
		t.Fatalf("cached cover is %dx%d, want 200x200", b.Dx(), b.Dy())
	}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 140, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
