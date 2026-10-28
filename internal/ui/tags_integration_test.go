package ui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
)

func TestTagEditorSavesAndRemovesCover(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32

	trackPath := filepath.Join(m.cfg.MusicDir, "Aurora - Night Drive.mp3")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-ac", "2", "-ar", "44100", "-c:a", "libmp3lame", "-q:a", "7", trackPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	tracks, err := m.lib.Scan()
	if err != nil || len(tracks) != 1 {
		t.Fatalf("scan: %v, %d tracks", err, len(tracks))
	}
	m.tracks = tracks
	m.visible = tracks
	m.cursor = 0
	m.view = viewAll
	m.rebuildVisible("")

	coverPath := filepath.Join(t.TempDir(), "cover.jpg")
	var cover bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 180, A: 255})
		}
	}
	if err := jpeg.Encode(&cover, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverPath, cover.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if cmd := m.openTagEditor(); cmd == nil {
		t.Fatal("tag editor did not open")
	}
	m.tags.cover.SetValue(coverPath)
	save := m.submitTagEdit()
	if save == nil {
		t.Fatal("submitTagEdit returned nil")
	}
	msg, ok := save().(tagsSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("save result = %#v", msg)
	}
	m.handleTagsSaved(msg)
	if !m.tracks[0].HasArt {
		t.Fatal("track HasArt was not updated")
	}
	if _, err := m.lib.Cover(m.tracks[0]); err != nil {
		t.Fatalf("cover was not embedded: %v", err)
	}

	if cmd := m.openTagEditor(); cmd == nil {
		t.Fatal("tag editor did not reopen")
	}
	m.tags.removeCover = true
	save = m.submitTagEdit()
	msg, ok = save().(tagsSavedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("remove result = %#v", msg)
	}
	m.handleTagsSaved(msg)
	if m.tracks[0].HasArt {
		t.Fatal("track HasArt stayed true")
	}
	if _, err := m.lib.Cover(model.Track{Path: trackPath}); !errors.Is(err, library.ErrNoArtwork) {
		t.Fatalf("cover was not removed: %v", err)
	}
}
