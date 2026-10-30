package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/download"
	"tuiplay/internal/model"
)

func hostileText() string {
	return "bad\tline\nnext\r\x1b[31mRED\x1b[0m " + string([]byte{0xff, 0xfe}) + " " + strings.Repeat("Ж", 80)
}

func TestHostileTextDoesNotBreakLayout(t *testing.T) {
	h := hostileText()
	sizes := [][2]int{{130, 40}, {110, 32}, {92, 28}, {84, 24}, {60, 18}}
	modes := []GlyphMode{GlyphsFull, GlyphsCompat, GlyphsASCII}
	modals := []modalKind{
		modalNone, modalAddLink, modalPreview, modalEQ, modalNewPlaylist, modalAddToPlaylist,
		modalConfirmRemoveFromPlaylist, modalConfirmDeleteTrack,
		modalConfirmDeletePlaylist, modalHelp, modalDownloads,
		modalImportM3U, modalExportM3U, modalEditTags, modalDuplicates, modalThemes, modalUpdate,
	}
	defer func() {
		SetGlyphMode(GlyphsFull)
		SetColorProfile(ColorTrueColor)
	}()

	for _, mode := range modes {
		SetGlyphMode(mode)
		for _, size := range sizes {
			for _, modal := range modals {
				t.Run(fmt.Sprintf("%s-%dx%d-modal%d", mode, size[0], size[1], modal), func(t *testing.T) {
					m, cleanup := testModel(t)
					defer cleanup()
					if _, err := m.playlists.Create("Mix"); err != nil {
						t.Fatal(err)
					}
					track := model.Track{
						Path:     `C:\music\bad-tab\song.mp3`,
						Title:    h,
						Artist:   h,
						Album:    h,
						Year:     h,
						Duration: 3 * time.Minute,
					}
					m.tracks = []model.Track{track}
					m.visible = []model.Track{track}
					m.current = &track
					m.queue = []string{track.Path}
					m.queueIndex = 0
					m.loaded = true
					m.paused = false
					m.duration = 3 * time.Minute
					m.position = time.Minute
					m.search = h
					m.toast = h
					m.toastKind = toastError
					m.toastUntil = time.Now().Add(time.Hour)
					m.modalErr = h
					m.pendingTrack = track
					m.pendingPaths = []string{track.Path}
					m.deleteTracks = []model.Track{track}
					m.deletePlaylist = h
					m.view = "Mix"
					m.jobs = []download.Job{{
						ID:      1,
						URL:     "https://soundcloud.com/rostiko5/birki",
						Title:   h,
						Artist:  h,
						Stage:   h,
						Detail:  h,
						Status:  download.StatusFailed,
						Err:     h,
						Warning: h,
						Path:    h,
					}}
					m.downloadsCur = 0
					m.preview = &download.Preview{
						Kind:     download.KindSoundCloudSet,
						Title:    h,
						Subtitle: h,
						Tracks: []download.PreviewTrack{
							{Title: h, Artist: h, Duration: time.Minute},
						},
						TotalDuration: time.Minute,
						Skipped:       1,
					}
					m.linkInput.SetValue(h)
					m.nameInput.SetValue(h)
					m.m3uInput.SetValue(h)
					m.searchInput.SetValue(h)
					m.modal = modal
					m.coverMode = false
					m.loading = false
					m.loadErr = nil
					m.width, m.height = size[0], size[1]
					assertRawFrame(t, m, "hostile")
				})
			}
		}
	}

	for _, mode := range modes {
		SetGlyphMode(mode)
		t.Run(fmt.Sprintf("%s-cover", mode), func(t *testing.T) {
			m, cleanup := testModel(t)
			defer cleanup()
			m.coverMode = true
			m.current = &model.Track{Path: `C:\music\song.mp3`, Title: h, Artist: h}
			m.width, m.height = 110, 32
			assertRawFrame(t, m, "cover")
		})

		t.Run(fmt.Sprintf("%s-loading", mode), func(t *testing.T) {
			m, cleanup := testModel(t)
			defer cleanup()
			m.loading = true
			m.loadErr = errors.New(h)
			m.cfg.MusicDir = h
			m.width, m.height = 110, 32
			assertRawFrame(t, m, "loading")
		})
	}
}

func assertRawFrame(t *testing.T, m *Model, label string) {
	t.Helper()
	w, h := m.dims()
	view := m.renderView()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Fatalf("%s: got %d lines, want %d", label, len(lines), h)
	}
	for i, line := range lines {
		plain := ansi.Strip(line)
		if !utf8.ValidString(plain) {
			t.Fatalf("%s: line %d is not valid UTF-8: %q", label, i, plain)
		}
		if strings.ContainsAny(plain, "\t\r") {
			t.Fatalf("%s: line %d contains a control character: %q", label, i, plain)
		}
		if got := ansi.StringWidth(line); got > w {
			t.Fatalf("%s: line %d width = %d, want <= %d: %q", label, i, got, w, plain)
		}
	}
}
