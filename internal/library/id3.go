package library

import (
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
)

// id3Metadata is the subset of tag data needed by the library.
type id3Metadata struct {
	Title       string
	Artist      string
	Album       string
	Year        string
	TrackNumber string
	CoverURL    string
	SourceURL   string
	HasArt      bool
}

// readID3v2 reads MP3 tags with bogem/id3v2. It is used as a fallback because
// dhowden/tag cannot decode some UTF-16 ID3v2 tags written by other tools.
func readID3v2(path string) (id3Metadata, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return id3Metadata{}, err
	}
	defer tag.Close()
	return id3Metadata{
		Title:       strings.TrimSpace(tag.Title()),
		Artist:      strings.TrimSpace(tag.Artist()),
		Album:       strings.TrimSpace(tag.Album()),
		Year:        strings.TrimSpace(tag.Year()),
		TrackNumber: strings.TrimSpace(tag.GetTextFrame(tag.CommonID("Track number/Position in set")).Text),
		CoverURL:    id3CoverURL(tag),
		SourceURL:   id3SourceURL(tag),
		HasArt:      id3PictureFrame(tag) != nil,
	}, nil
}

func id3SourceURL(tag *id3v2.Tag) string {
	for _, frame := range tag.GetFrames("COMM") {
		comment, ok := frame.(id3v2.CommentFrame)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(comment.Description), "source") {
			return strings.TrimSpace(comment.Text)
		}
	}
	return ""
}

func id3CoverURL(tag *id3v2.Tag) string {
	for _, frame := range tag.GetFrames("TXXX") {
		udtf, ok := frame.(id3v2.UserDefinedTextFrame)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(udtf.Description), "COVER_URL") {
			return strings.TrimSpace(udtf.Value)
		}
	}
	return ""
}

// id3v2Picture returns the first embedded front-cover picture in an MP3.
func id3v2Picture(path string) ([]byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".mp3") {
		return nil, ErrNoArtwork
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
	}
	defer tag.Close()
	pic := id3PictureFrame(tag)
	if pic == nil || len(pic.Picture) == 0 {
		return nil, ErrNoArtwork
	}
	return pic.Picture, nil
}

func id3PictureFrame(tag *id3v2.Tag) *id3v2.PictureFrame {
	frames := tag.GetFrames("APIC")
	if len(frames) == 0 {
		return nil
	}
	var fallback *id3v2.PictureFrame
	for _, frame := range frames {
		var pic id3v2.PictureFrame
		switch f := frame.(type) {
		case id3v2.PictureFrame:
			pic = f
		case *id3v2.PictureFrame:
			if f == nil {
				continue
			}
			pic = *f
		default:
			continue
		}
		if pic.PictureType == id3v2.PTFrontCover {
			cp := pic
			return &cp
		}
		if fallback == nil {
			cp := pic
			fallback = &cp
		}
	}
	return fallback
}
