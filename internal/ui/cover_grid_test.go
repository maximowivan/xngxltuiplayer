package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestCoverGridLayoutMetrics(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	layout := m.coverLayout(130, 36)
	if !layout.grid {
		t.Fatal("130x36 should use the four-pane grid")
	}
	if layout.cover.left != 1 || layout.cover.top != 1 {
		t.Fatalf("cover origin = %d,%d, want 1,1", layout.cover.left, layout.cover.top)
	}
	if layout.upNext.left != layout.cover.left+layout.cover.width+1 ||
		layout.upNext.top != layout.cover.top ||
		layout.upNext.height != layout.cover.height {
		t.Fatalf("up-next pane = %+v, cover = %+v", layout.upNext, layout.cover)
	}
	if layout.player.left != layout.cover.left ||
		layout.player.top != layout.cover.top+layout.cover.height+1 ||
		layout.player.height != 6 {
		t.Fatalf("player pane = %+v", layout.player)
	}
	if layout.visualizer.left != layout.upNext.left ||
		layout.visualizer.top != layout.player.top ||
		layout.visualizer.height != layout.player.height {
		t.Fatalf("visualizer pane = %+v", layout.visualizer)
	}
	if layout.cover.width+1+layout.upNext.width != 128 {
		t.Fatalf("column widths = %d + 1 + %d, want 128", layout.cover.width, layout.upNext.width)
	}
	if layout.upNext.width >= layout.cover.width {
		t.Fatalf("right column = %d, cover = %d; right should stay narrower", layout.upNext.width, layout.cover.width)
	}
	if layout.cover.height+1+layout.player.height != 33 {
		t.Fatalf("row heights = %d + 1 + %d, want 33", layout.cover.height, layout.player.height)
	}
}

func TestCoverGridShowsFourPanes(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36

	frame := m.renderCover(130, 36)
	for _, want := range []string{
		"COVER",
		"UP NEXT",
		"PLAYER",
		"VISUALIZER",
		"Paper Planes",
		"Slow Motion",
		"Night Drive",
		"Aurora",
		"vol 80%",
		"queue 1/3",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("cover grid is missing %q:\n%s", want, frame)
		}
	}
	for i, line := range strings.Split(frame, "\n") {
		if got := ansi.StringWidth(line); got != 130 {
			t.Fatalf("line %d width = %d, want 130\n%s", i, got, line)
		}
	}
}

func TestCoverGridBreakpoints(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	cases := []struct {
		w, h int
		grid bool
	}{
		{130, 36, true},
		{110, 32, true},
		{96, 28, true},
		{84, 24, true},
		{80, 22, true},
		{78, 22, false},
		{72, 20, false},
		{60, 24, false},
		{130, 18, false},
		{200, 20, false},
	}
	for _, tc := range cases {
		got := m.coverLayout(tc.w, tc.h).grid
		if got != tc.grid {
			t.Fatalf("%dx%d grid = %v, want %v", tc.w, tc.h, got, tc.grid)
		}
		m.coverMode = true
		m.width, m.height = tc.w, tc.h
		assertFrame(t, m, "cover-grid-breakpoint")
		frame := m.renderCover(tc.w, tc.h)
		if tc.grid && !strings.Contains(frame, "VISUALIZER") {
			t.Fatalf("%dx%d grid is missing VISUALIZER:\n%s", tc.w, tc.h, frame)
		}
		if !tc.grid && strings.Contains(frame, "VISUALIZER") {
			t.Fatalf("%dx%d compact layout should not show VISUALIZER:\n%s", tc.w, tc.h, frame)
		}
	}
}

func TestCoverGridResize(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	for _, size := range [][2]int{{130, 36}, {110, 32}, {84, 24}, {80, 22}, {78, 22}, {60, 24}, {84, 24}} {
		m.width, m.height = size[0], size[1]
		m.resizeInputs()
		assertFrame(t, m, "cover-grid-resize")
	}
}

func TestCoverUpNextHitbox(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	if !layout.grid {
		t.Fatal("expected grid")
	}
	tracks := m.upNextTracks(m.coverUpNextLimit(layout.upNext))
	if len(tracks) < 2 {
		t.Fatalf("up next = %d tracks, want at least 2", len(tracks))
	}
	got, ok := m.coverUpNextTrackAt(layout.upNext, layout.upNext.left+2, layout.upNext.top+2)
	if !ok || got.Title != tracks[0].Title {
		t.Fatalf("first row = %+v, %v, want %q", got, ok, tracks[0].Title)
	}
	got, ok = m.coverUpNextTrackAt(layout.upNext, layout.upNext.left+2, layout.upNext.top+3)
	if !ok || got.Title != tracks[1].Title {
		t.Fatalf("second row = %+v, %v, want %q", got, ok, tracks[1].Title)
	}
	if _, ok := m.coverUpNextTrackAt(layout.upNext, layout.upNext.left+2, layout.upNext.top+1); ok {
		t.Fatal("the position header should not be a track row")
	}
	if idx := m.queueIndexFor(got.Path); idx != 2 {
		t.Fatalf("queue index = %d, want 2", idx)
	}
}

func TestCoverGridProgressClick(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	m.ended = true
	m.duration = 200 * time.Second
	layout := m.coverLayout(130, 36)
	start, barWidth := m.coverProgressGeometry(130)
	if barWidth < 1 {
		t.Fatal("empty progress bar")
	}
	m.Update(tea.MouseMsg{
		X:      start + barWidth/2,
		Y:      layout.player.top + 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if m.position < 95*time.Second || m.position > 105*time.Second {
		t.Fatalf("position = %v, want about 100s", m.position)
	}
}

func TestCoverGridSpectrumBarCount(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	want := (layout.visualizer.width - 4) / 2
	if want < 4 {
		want = 4
	}
	if want > 32 {
		want = 32
	}
	if got := m.spectrumBarCount(); got != want {
		t.Fatalf("spectrum bars = %d, want %d", got, want)
	}
	m.width, m.height = 60, 24
	if got := m.spectrumBarCount(); got != 0 {
		t.Fatalf("compact cover bars = %d, want 0", got)
	}
}

func TestCoverGridHintsAreLastLines(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 120, 30
	lines := strings.Split(m.renderCover(120, 30), "\n")
	if len(lines) != 30 {
		t.Fatalf("lines = %d, want 30", len(lines))
	}
	for i := 28; i < len(lines); i++ {
		if strings.Contains(lines[i], "PLAYER") || strings.Contains(lines[i], "VISUALIZER") {
			t.Fatalf("stale pane title after hints on line %d:\n%s", i, strings.Join(lines, "\n"))
		}
	}
}

func TestCoverUpNextHidesClickHintWhenEmpty(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	m.queue = nil
	frame := m.renderCover(130, 36)
	if strings.Contains(frame, "click to play") {
		t.Fatalf("empty up-next should not advertise click to play:\n%s", frame)
	}
	if !strings.Contains(strings.ToLower(frame), "queue empty") {
		t.Fatalf("empty queue message missing:\n%s", frame)
	}
}
