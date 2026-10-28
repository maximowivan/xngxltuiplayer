package library

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"

	"tuiplay/internal/model"
)

// MaxCoverInput limits the source image accepted by the tag editor. The
// image is re-encoded as JPEG before it is embedded.
const MaxCoverInput = 20 << 20

// TagEdit is the editable subset of a track's tags. When CoverData is nil and
// RemoveCover is false, the existing cover is preserved byte-for-byte.
type TagEdit struct {
	Title       string
	Artist      string
	Album       string
	Year        string
	TrackNumber string
	CoverData   []byte
	RemoveCover bool
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
	if len(edit.CoverData) > MaxCoverInput {
		return track, fmt.Errorf("cover image is too large: %d bytes (limit %d)", len(edit.CoverData), MaxCoverInput)
	}
	var coverJPEG []byte
	if len(edit.CoverData) > 0 {
		var coverErr error
		coverJPEG, coverErr = NormalizeCover(edit.CoverData)
		if coverErr != nil {
			return track, fmt.Errorf("prepare cover: %w", coverErr)
		}
	}

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
		err = writeMP3Tags(tmpName, edit, coverJPEG)
	} else {
		err = writeFLACTags(tmpName, edit, coverJPEG)
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
	if len(coverJPEG) > 0 {
		track.HasArt = true
	} else if edit.RemoveCover {
		track.HasArt = false
	}

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

func writeMP3Tags(path string, edit TagEdit, coverJPEG []byte) error {
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
	if len(coverJPEG) > 0 {
		replaceFrontCoverMP3(tag, coverJPEG)
	} else if edit.RemoveCover {
		removeFrontCoverMP3(tag)
	}
	if err := tag.Save(); err != nil {
		return fmt.Errorf("save MP3 tags: %w", err)
	}
	return nil
}

// replaceFrontCoverMP3 preserves additional pictures (back cover, artist,
// booklet) while replacing the front cover.
func replaceFrontCoverMP3(tag *id3v2.Tag, data []byte) {
	keep := picturesExceptFront(tag)
	tag.DeleteFrames("APIC")
	for _, pic := range keep {
		tag.AddAttachedPicture(pic)
	}
	tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "cover",
		Picture:     data,
	})
}

func removeFrontCoverMP3(tag *id3v2.Tag) {
	keep := picturesExceptFront(tag)
	tag.DeleteFrames("APIC")
	for _, pic := range keep {
		tag.AddAttachedPicture(pic)
	}
}

func picturesExceptFront(tag *id3v2.Tag) []id3v2.PictureFrame {
	var keep []id3v2.PictureFrame
	for _, frame := range tag.GetFrames("APIC") {
		switch pic := frame.(type) {
		case id3v2.PictureFrame:
			if pic.PictureType != id3v2.PTFrontCover {
				keep = append(keep, pic)
			}
		case *id3v2.PictureFrame:
			if pic != nil && pic.PictureType != id3v2.PTFrontCover {
				keep = append(keep, *pic)
			}
		}
	}
	return keep
}

func setID3(tag *id3v2.Tag, id, value string) {
	if value == "" {
		tag.DeleteFrames(id)
		return
	}
	tag.AddTextFrame(id, id3v2.EncodingUTF8, value)
}

func writeFLACTags(path string, edit TagEdit, coverJPEG []byte) error {
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

	if len(coverJPEG) > 0 {
		if err := replaceFrontCoverFLAC(f, coverJPEG); err != nil {
			return err
		}
	} else if edit.RemoveCover {
		removeFrontCoverFLAC(f)
	}

	data := f.Marshal()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write FLAC tags: %w", err)
	}
	return nil
}

func replaceFrontCoverFLAC(f *flac.File, data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read cover dimensions: %w", err)
	}
	removeFrontCoverFLAC(f)
	block := encodeFLACPicture(data, cfg.Width, cfg.Height)
	insertFLACBlock(f, block)
	return nil
}

// removeFrontCoverFLAC drops only PICTURE blocks marked as a front cover.
func removeFrontCoverFLAC(f *flac.File) {
	kept := f.Meta[:0]
	for _, block := range f.Meta {
		if block.Type == flac.Picture && flacPictureType(block) == 3 {
			continue
		}
		kept = append(kept, block)
	}
	f.Meta = kept
}

func flacPictureType(block *flac.MetaDataBlock) uint32 {
	if block == nil || block.Type != flac.Picture || len(block.Data) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(block.Data[:4])
}

func encodeFLACPicture(data []byte, width, height int) flac.MetaDataBlock {
	var buf bytes.Buffer
	writeUint32 := func(v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	const mime = "image/jpeg"
	const description = "cover"
	writeUint32(3) // front cover
	writeUint32(uint32(len(mime)))
	buf.WriteString(mime)
	writeUint32(uint32(len(description)))
	buf.WriteString(description)
	writeUint32(uint32(width))
	writeUint32(uint32(height))
	writeUint32(24) // bits per pixel
	writeUint32(0)  // palette colors
	writeUint32(uint32(len(data)))
	buf.Write(data)
	return flac.MetaDataBlock{Type: flac.Picture, Data: buf.Bytes()}
}

// insertFLACBlock puts a PICTURE before padding when possible so the file
// keeps its conventional metadata layout.
func insertFLACBlock(f *flac.File, block flac.MetaDataBlock) {
	for i, existing := range f.Meta {
		if existing.Type == flac.Padding {
			f.Meta = append(f.Meta, nil)
			copy(f.Meta[i+1:], f.Meta[i:])
			f.Meta[i] = &block
			return
		}
	}
	f.Meta = append(f.Meta, &block)
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
