package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"

	"tuiplay/internal/model"
)

// TagEdit is the editable subset of a track's tags. Cover art is deliberately
// not changed here: it is preserved byte-for-byte by the writers.
type TagEdit struct {
	Title       string
	Artist      string
	Album       string
	Year        string
	TrackNumber string
}

// UpdateTags writes tags atomically and updates the library cache entry.
// The caller should stop playback first when editing the current track: on
// Windows the audio decoder may keep the original file open.
func (l *Library) UpdateTags(track model.Track, edit TagEdit) (model.Track, error) {
	ext := strings.ToLower(filepath.Ext(track.Path))
	if ext != ".mp3" && ext != ".flac" {
		return track, fmt.Errorf("tag editing is supported for MP3 and FLAC only")
	}
	edit = normalizeTagEdit(edit)

	l.scanMu.Lock()
	defer l.scanMu.Unlock()

	info, err := os.Stat(track.Path)
	if err != nil {
		return track, err
	}
	dir := filepath.Dir(track.Path)
	tmp, err := os.CreateTemp(dir, ".xngxl-tag-*"+ext)
	if err != nil {
		return track, fmt.Errorf("create tag temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	src, err := os.Open(track.Path)
	if err != nil {
		_ = tmp.Close()
		return track, err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = src.Close()
		_ = tmp.Close()
		return track, fmt.Errorf("copy track for tag edit: %w", err)
	}
	_ = src.Close()
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return track, err
	}
	if err := tmp.Close(); err != nil {
		return track, err
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return track, err
	}

	if ext == ".mp3" {
		err = writeMP3Tags(tmpName, edit)
	} else {
		err = writeFLACTags(tmpName, edit)
	}
	if err != nil {
		return track, err
	}
	if err := os.Rename(tmpName, track.Path); err != nil {
		return track, fmt.Errorf("replace track after tag edit: %w", err)
	}
	cleanup = false
	_ = os.Remove(CoverPath(l.DataDir, track.Path))

	newInfo, statErr := os.Stat(track.Path)
	if statErr == nil {
		track.Size = newInfo.Size()
		track.ModTime = newInfo.ModTime().UnixNano()
	}
	track.Title = edit.Title
	track.Artist = edit.Artist
	track.Album = edit.Album
	track.Year = edit.Year
	track.TrackNumber = edit.TrackNumber

	l.mu.Lock()
	for i := range l.tracks {
		if filepath.Clean(l.tracks[i].Path) == filepath.Clean(track.Path) {
			l.tracks[i] = track
			break
		}
	}
	l.mu.Unlock()
	return track, nil
}

func normalizeTagEdit(edit TagEdit) TagEdit {
	normalize := func(s string) string {
		s = strings.ToValidUTF8(s, "?")
		s = strings.ReplaceAll(s, "\r", " ")
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.ReplaceAll(s, "\t", " ")
		s = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, s)
		runes := []rune(strings.TrimSpace(s))
		if len(runes) > 512 {
			runes = runes[:512]
		}
		return string(runes)
	}
	edit.Title = normalize(edit.Title)
	edit.Artist = normalize(edit.Artist)
	edit.Album = normalize(edit.Album)
	edit.Year = normalize(edit.Year)
	edit.TrackNumber = normalize(edit.TrackNumber)
	return edit
}

func writeMP3Tags(path string, edit TagEdit) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open MP3 tags: %w", err)
	}
	defer tag.Close()
	tag.SetVersion(4)
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)

	setID3(tag, tag.CommonID("Title"), edit.Title)
	setID3(tag, tag.CommonID("Artist"), edit.Artist)
	setID3(tag, tag.CommonID("Album/Movie/Show title"), edit.Album)
	setID3(tag, tag.CommonID("Year"), edit.Year)
	setID3(tag, tag.CommonID("Track number/Position in set"), edit.TrackNumber)
	if err := tag.Save(); err != nil {
		return fmt.Errorf("save MP3 tags: %w", err)
	}
	return nil
}

func setID3(tag *id3v2.Tag, id, value string) {
	if value == "" {
		tag.DeleteFrames(id)
		return
	}
	tag.AddTextFrame(id, id3v2.EncodingUTF8, value)
}

func writeFLACTags(path string, edit TagEdit) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("open FLAC tags: %w", err)
	}
	var vc *flacvorbis.MetaDataBlockVorbisComment
	index := -1
	for i, block := range f.Meta {
		if block.Type != flac.VorbisComment {
			continue
		}
		parsed, parseErr := flacvorbis.ParseFromMetaDataBlock(*block)
		if parseErr != nil {
			return fmt.Errorf("parse FLAC comments: %w", parseErr)
		}
		vc = parsed
		index = i
		break
	}
	if vc == nil {
		vc = flacvorbis.New()
	}
	setVorbis(vc, "TITLE", edit.Title)
	setVorbis(vc, "ARTIST", edit.Artist)
	setVorbis(vc, "ALBUM", edit.Album)
	setVorbis(vc, "DATE", edit.Year)
	setVorbis(vc, "TRACKNUMBER", edit.TrackNumber)

	block := vc.Marshal()
	if index >= 0 {
		f.Meta[index] = &block
	} else {
		f.Meta = append(f.Meta, &block)
	}
	data := f.Marshal()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write FLAC tags: %w", err)
	}
	return nil
}

func setVorbis(vc *flacvorbis.MetaDataBlockVorbisComment, key, value string) {
	comments := vc.Comments[:0]
	for _, comment := range vc.Comments {
		before, _, ok := strings.Cut(comment, "=")
		if ok && strings.EqualFold(before, key) {
			continue
		}
		comments = append(comments, comment)
	}
	vc.Comments = comments
	if value != "" {
		_ = vc.Add(strings.ToUpper(key), value)
	}
}
