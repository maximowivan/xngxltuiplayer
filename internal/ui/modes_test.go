package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/model"
)

func TestGlyphSetsAreConsistent(t *testing.T) {
	defer SetGlyphMode(GlyphsFull)
	for _, mode := range []GlyphMode{GlyphsFull, GlyphsCompat, GlyphsASCII} {
		SetGlyphMode(mode)
		fields := []string{
			currentGlyphs.BorderTopLeft, currentGlyphs.BorderTopRight,
			currentGlyphs.BorderBottomLeft, currentGlyphs.BorderBottomRight,
			currentGlyphs.BorderHorizontal, currentGlyphs.BorderVertical,
			currentGlyphs.Separator, currentGlyphs.Ellipsis, currentGlyphs.Dash,
			currentGlyphs.Prompt, currentGlyphs.Cursor, currentGlyphs.Checked,
			currentGlyphs.Active, currentGlyphs.Drop, currentGlyphs.Check,
			currentGlyphs.Cross, currentGlyphs.Partial, currentGlyphs.Warning,
			currentGlyphs.Bullet, currentGlyphs.Stop, currentGlyphs.Playlist,
			currentGlyphs.UpDown, currentGlyphs.LeftRight,
			currentGlyphs.UpDownWords, currentGlyphs.LeftRightWords,
			currentGlyphs.ProgressFull, currentGlyphs.ProgressEmpty,
			currentGlyphs.VolumeFull, currentGlyphs.VolumeEmpty,
			currentGlyphs.SpectrumBase,
		}
		for _, field := range fields {
			if field == "" {
				t.Fatalf("%s: empty glyph", mode)
			}
			if ansi.StringWidth(field) < 1 {
				t.Fatalf("%s: zero-width glyph %q", mode, field)
			}
		}
		if len(currentGlyphs.Spinner) == 0 {
			t.Fatalf("%s: empty spinner", mode)
		}
	}
}

func TestASCIIModeUsesPrintableASCII(t *testing.T) {
	defer func() {
		SetGlyphMode(GlyphsFull)
		SetColorProfile(ColorTrueColor)
		SetMouseAvailable(true)
	}()
	SetGlyphMode(GlyphsASCII)
	SetColorProfile(ColorTrueColor)
	SetMouseAvailable(false)

	m, cleanup := testModel(t)
	defer cleanup()
	track := model.Track{
		Path:     `C:\music\song.mp3`,
		Title:    "Night Drive",
		Artist:   "Aurora",
		Album:    "Signals",
		Year:     "2024",
		Duration: 3 * time.Minute,
	}
	m.tracks = []model.Track{track}
	m.visible = []model.Track{track}
	m.current = &track
	m.queue = []string{track.Path}
	m.queueIndex = 0
	m.loaded = true
	m.duration = 3 * time.Minute
	m.position = time.Minute
	m.search = "night"
	m.toast = "All good"
	m.toastKind = toastInfo
	m.toastUntil = time.Now().Add(time.Hour)
	m.modalErr = "bad link"
	m.pendingTrack = track
	m.pendingPaths = []string{track.Path}
	m.deleteTracks = []model.Track{track}
	m.deletePlaylist = "Mix"
	m.view = "Mix"
	m.jobs = nil
	m.downloadsCur = 0

	for _, modal := range []modalKind{
		modalNone, modalAddLink, modalEQ, modalNewPlaylist, modalAddToPlaylist,
		modalConfirmRemoveFromPlaylist, modalConfirmDeleteTrack,
		modalConfirmDeletePlaylist, modalHelp, modalDownloads,
		modalImportM3U, modalExportM3U, modalEditTags, modalDuplicates, modalThemes, modalUpdate,
	} {
		m.modal = modal
		m.width, m.height = 110, 32
		assertPrintableASCIIFrame(t, m, fmt.Sprintf("modal-%d", modal))
	}

	m.modal = modalNone
	m.coverMode = true
	assertPrintableASCIIFrame(t, m, "cover")

	m.coverMode = false
	m.loading = true
	m.loadErr = errors.New("boom")
	assertPrintableASCIIFrame(t, m, "loading")
}

func assertPrintableASCIIFrame(t *testing.T, m *Model, label string) {
	t.Helper()
	w, h := m.dims()
	lines := strings.Split(m.renderView(), "\n")
	if len(lines) != h {
		t.Fatalf("%s: got %d lines, want %d", label, len(lines), h)
	}
	for i, line := range lines {
		plain := ansi.Strip(line)
		if !utf8.ValidString(plain) {
			t.Fatalf("%s: line %d is not valid UTF-8", label, i)
		}
		if got := ansi.StringWidth(line); got > w {
			t.Fatalf("%s: line %d width = %d, want <= %d: %q", label, i, got, w, plain)
		}
		for _, r := range plain {
			if r < 0x20 || r > 0x7e {
				t.Fatalf("%s: line %d contains non-ASCII rune %q: %q", label, i, r, plain)
			}
		}
	}
}

func TestColorProfilesDoNotEmitUnsupportedSequences(t *testing.T) {
	defer SetColorProfile(ColorTrueColor)
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32

	SetColorProfile(ColorNone)
	if view := m.View(); strings.Contains(view, "\x1b[") {
		t.Fatalf("ColorNone view contains ANSI sequences: %q", view)
	}

	SetColorProfile(ColorANSI256)
	view := m.View()
	if strings.Contains(view, "\x1b[38;2;") || strings.Contains(view, "\x1b[48;2;") {
		t.Fatal("256-color view contains 24-bit colour sequences")
	}
	if !strings.Contains(view, "48;5;") && !strings.Contains(view, "38;5;") {
		t.Fatal("256-color view does not use the 256-colour palette")
	}

	SetColorProfile(ColorANSI16)
	view = m.View()
	if strings.Contains(view, "\x1b[38;2;") || strings.Contains(view, "\x1b[48;2;") {
		t.Fatal("16-color view contains 24-bit colour sequences")
	}
}

func TestASCIIModeHasASCIIIcons(t *testing.T) {
	defer SetGlyphMode(GlyphsFull)
	SetGlyphMode(GlyphsASCII)
	for _, icon := range []string{iconLogo, iconShuffle, iconRepeat, iconPlay, iconPause, iconFavorite} {
		for _, r := range icon {
			if r > 0x7f {
				t.Fatalf("ASCII icon %q contains non-ASCII rune %q", icon, r)
			}
		}
	}
}

func TestThemeLabelsAreASCII(t *testing.T) {
	for _, p := range themePalettes {
		for _, r := range p.Label {
			if r < 0x20 || r > 0x7e {
				t.Fatalf("theme label %q contains non-ASCII rune %q", p.Label, r)
			}
		}
	}
}
