package library

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bogem/id3v2/v2"
	"github.com/dhowden/tag"

	"tuiplay/internal/model"
)

func makeAudio(t *testing.T, path string, codec string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	wav := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wav, testWAV(0.35, 440), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", wav}
	switch codec {
	case "mp3":
		args = append(args, "-c:a", "libmp3lame", "-q:a", "7")
	case "flac":
		args = append(args, "-c:a", "flac")
	default:
		t.Fatalf("unknown codec %q", codec)
	}
	args = append(args, path)
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

func TestUpdateMP3TagsPreservesCover(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "track.mp3")
	makeAudio(t, path, "mp3")

	cover := testJPEG(t)
	tagFile, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	tagFile.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "cover",
		Picture:     cover,
	})
	if err := tagFile.Save(); err != nil {
		t.Fatal(err)
	}
	if err := tagFile.Close(); err != nil {
		t.Fatal(err)
	}

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil || len(tracks) != 1 {
		t.Fatalf("scan: %v, %d tracks", err, len(tracks))
	}
	updated, err := lib.UpdateTags(tracks[0], TagEdit{
		Title:       "Night Drive",
		Artist:      "Aurora",
		Album:       "Signals",
		Year:        "2024",
		TrackNumber: "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Night Drive" || updated.TrackNumber != "3" {
		t.Fatalf("updated track = %+v", updated)
	}
	meta, err := readID3v2(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Night Drive" || meta.Artist != "Aurora" || meta.Album != "Signals" ||
		meta.Year != "2024" || meta.TrackNumber != "3" {
		t.Fatalf("tags = %+v", meta)
	}
	gotCover, err := id3v2Picture(path)
	if err != nil {
		t.Fatalf("cover was not preserved: %v", err)
	}
	if !bytes.Equal(gotCover, cover) {
		t.Fatalf("cover changed: got %d bytes, want %d", len(gotCover), len(cover))
	}

	// Empty fields remove the frames instead of writing empty values.
	if _, err := lib.UpdateTags(updated, TagEdit{Title: "Night Drive"}); err != nil {
		t.Fatal(err)
	}
	meta, err = readID3v2(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Artist != "" || meta.Album != "" || meta.Year != "" || meta.TrackNumber != "" {
		t.Fatalf("empty fields were not removed: %+v", meta)
	}
}

func TestUpdateFLACTags(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "track.flac")
	makeAudio(t, path, "flac")

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil || len(tracks) != 1 {
		t.Fatalf("scan: %v, %d tracks", err, len(tracks))
	}
	updated, err := lib.UpdateTags(tracks[0], TagEdit{
		Title:       "Paper Planes",
		Artist:      "Lumen",
		Album:       "Signals",
		Year:        "2023",
		TrackNumber: "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Paper Planes" || updated.TrackNumber != "7" {
		t.Fatalf("updated track = %+v", updated)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := tag.ReadFrom(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title() != "Paper Planes" || meta.Artist() != "Lumen" || meta.Album() != "Signals" {
		t.Fatalf("FLAC tags were not written: title=%q artist=%q album=%q", meta.Title(), meta.Artist(), meta.Album())
	}
	if n, _ := meta.Track(); n != 7 {
		t.Fatalf("FLAC track number = %d, want 7", n)
	}
}

func TestUpdateTagsRejectsUnsupportedFormat(t *testing.T) {
	lib := New(t.TempDir(), t.TempDir())
	_, err := lib.UpdateTags(model.Track{Path: "song.wav"}, TagEdit{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "MP3 and FLAC") {
		t.Fatalf("error = %v, want unsupported format", err)
	}
}

func TestUpdateMP3CoverReplaceAndRemove(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "cover.mp3")
	makeAudio(t, path, "mp3")

	// Start with a front cover and a back cover; replacing the front must not
	// discard the extra picture.
	tagFile, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	tagFile.AddAttachedPicture(id3v2.PictureFrame{
		Encoding: id3v2.EncodingUTF8, MimeType: "image/jpeg",
		PictureType: id3v2.PTBackCover, Description: "back", Picture: testJPEG(t),
	})
	if err := tagFile.Save(); err != nil {
		t.Fatal(err)
	}
	if err := tagFile.Close(); err != nil {
		t.Fatal(err)
	}

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil || len(tracks) != 1 {
		t.Fatalf("scan: %v, %d tracks", err, len(tracks))
	}
	updated, err := lib.UpdateTags(tracks[0], TagEdit{
		Title: "With Cover", CoverData: testJPEG(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasArt {
		t.Fatal("HasArt was not set after embedding a cover")
	}
	pictures := readPictures(t, path)
	if len(pictures) != 2 {
		t.Fatalf("pictures = %d, want front and back", len(pictures))
	}
	var front, back bool
	for _, pic := range pictures {
		switch pic.PictureType {
		case id3v2.PTFrontCover:
			front = true
		case id3v2.PTBackCover:
			back = true
		}
	}
	if !front || !back {
		t.Fatalf("front=%v back=%v, pictures=%+v", front, back, pictures)
	}
	if _, err := lib.Cover(updated); err != nil {
		t.Fatalf("cover cache was not written: %v", err)
	}

	updated, err = lib.UpdateTags(updated, TagEdit{Title: "No Cover", RemoveCover: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.HasArt {
		t.Fatal("HasArt stayed true after removing the cover")
	}
	pictures = readPictures(t, path)
	if len(pictures) != 1 || pictures[0].PictureType != id3v2.PTBackCover {
		t.Fatalf("back cover was not preserved: %+v", pictures)
	}
}

func TestUpdateFLACCover(t *testing.T) {
	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(musicDir, "cover.flac")
	makeAudio(t, path, "flac")

	lib := New(musicDir, dataDir)
	tracks, err := lib.Scan()
	if err != nil || len(tracks) != 1 {
		t.Fatalf("scan: %v, %d tracks", err, len(tracks))
	}
	updated, err := lib.UpdateTags(tracks[0], TagEdit{
		Title: "Covered", Artist: "Aurora", CoverData: testJPEG(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasArt {
		t.Fatal("HasArt was not set for FLAC")
	}
	meta := readFLACMeta(t, path)
	if meta.Picture() == nil {
		t.Fatal("FLAC picture was not embedded")
	}
	if meta.Title() != "Covered" || meta.Artist() != "Aurora" {
		t.Fatalf("tags changed unexpectedly: title=%q artist=%q", meta.Title(), meta.Artist())
	}

	updated, err = lib.UpdateTags(updated, TagEdit{Title: "Covered", Artist: "Aurora", RemoveCover: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.HasArt {
		t.Fatal("HasArt stayed true after removing FLAC picture")
	}
	meta = readFLACMeta(t, path)
	if meta.Picture() != nil {
		t.Fatal("FLAC picture was not removed")
	}
}

func TestUpdateTagsRejectsOversizedCover(t *testing.T) {
	lib := New(t.TempDir(), t.TempDir())
	_, err := lib.UpdateTags(model.Track{Path: "song.mp3"}, TagEdit{
		CoverData: make([]byte, MaxCoverInput+1),
	})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error = %v, want size error", err)
	}
}

func readPictures(t *testing.T, path string) []id3v2.PictureFrame {
	t.Helper()
	tagFile, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tagFile.Close()
	var out []id3v2.PictureFrame
	for _, frame := range tagFile.GetFrames("APIC") {
		switch pic := frame.(type) {
		case id3v2.PictureFrame:
			out = append(out, pic)
		case *id3v2.PictureFrame:
			if pic != nil {
				out = append(out, *pic)
			}
		}
	}
	return out
}

func readFLACMeta(t *testing.T, path string) tag.Metadata {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		t.Fatal(err)
	}
	return meta
}
