package download

import (
	"fmt"
	"strings"

	"github.com/bogem/id3v2/v2"
)

func writeTags(path string, meta Meta, coverJPEG []byte) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open MP3 tag: %w", err)
	}
	defer tag.Close()

	tag.SetVersion(4)
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	tag.DeleteAllFrames()

	if title := strings.TrimSpace(meta.Title); title != "" {
		tag.SetTitle(title)
	}
	if artist := strings.TrimSpace(meta.Artist); artist != "" {
		tag.SetArtist(artist)
	}
	if album := strings.TrimSpace(meta.Album); album != "" {
		tag.SetAlbum(album)
	}
	if year := strings.TrimSpace(meta.Year); year != "" {
		tag.SetYear(year)
	}
	if source := strings.TrimSpace(meta.SourceURL); source != "" {
		tag.AddCommentFrame(id3v2.CommentFrame{
			Encoding:    id3v2.EncodingUTF8,
			Language:    "eng",
			Description: "source",
			Text:        source,
		})
	}
	if coverURL := strings.TrimSpace(meta.CoverURL); coverURL != "" {
		tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
			Encoding:    id3v2.EncodingUTF8,
			Description: "COVER_URL",
			Value:       coverURL,
		})
	}
	if len(coverJPEG) > 0 {
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    "image/jpeg",
			PictureType: id3v2.PTFrontCover,
			Description: "Cover",
			Picture:     coverJPEG,
		})
	}
	if err := tag.Save(); err != nil {
		return fmt.Errorf("write MP3 tag: %w", err)
	}
	return nil
}
