package library

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScanAndCache(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "Aurora - Night Drive.wav")
	if err := os.WriteFile(path, testWAV(0.4, 330), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	tr := tracks[0]
	if tr.Title != "Night Drive" || tr.Artist != "Aurora" {
		t.Fatalf("unexpected metadata: %+v", tr)
	}
	if _, err := exec.LookPath("ffprobe"); err == nil && tr.Duration <= 0 {
		t.Fatalf("duration was not probed: %v", tr.Duration)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "library.json")); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	again, err := lib.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Path != tr.Path || again[0].ModTime != tr.ModTime {
		t.Fatalf("cached scan changed: %+v", again)
	}

	if _, err := lib.Cover(tr); err != ErrNoArtwork {
		t.Fatalf("Cover error = %v, want ErrNoArtwork", err)
	}

	if err := lib.DeleteFile(tr); err != nil {
		t.Fatal(err)
	}
	empty, err := lib.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("library not empty after delete: %+v", empty)
	}
}

func TestIsSupported(t *testing.T) {
	for _, name := range []string{"a.mp3", "a.FLAC", "a.wav"} {
		if !IsSupported(name) {
			t.Errorf("%s should be supported", name)
		}
	}
	for _, name := range []string{"a.m4a", "a.ogg", "a.txt", "a"} {
		if IsSupported(name) {
			t.Errorf("%s should not be supported", name)
		}
	}
}

func TestEncodeCoverJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	data, err := EncodeCoverJPEG(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := decoded.Bounds()
	if b.Dx() != 200 || b.Dy() != 200 {
		t.Fatalf("cover is %dx%d, want 200x200", b.Dx(), b.Dy())
	}
}

func testWAV(seconds, freq float64) []byte {
	const sampleRate = 48000
	samples := int(seconds * sampleRate)
	dataSize := samples * 2 * 2

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for i := 0; i < samples; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/sampleRate) * 12000)
		_ = binary.Write(&buf, binary.LittleEndian, v)
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}
