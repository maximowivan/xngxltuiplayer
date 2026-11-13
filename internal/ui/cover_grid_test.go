package ui

import (
	"fmt"
	"path/filepath"
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
		t.Fatal("130x36 should use the grid")
	}
	if layout.cover.left != 1 || layout.cover.top != 1 {
		t.Fatalf("cover origin = %d,%d, want 1,1", layout.cover.left, layout.cover.top)
	}
	if layout.upNext.left != layout.cover.left+layout.cover.width+1 ||
		layout.upNext.top != layout.cover.top ||
		layout.upNext.height != layout.cover.height {
		t.Fatalf("up-next pane = %+v, cover = %+v", layout.upNext, layout.cover)
	}
	if layout.cover.height != 33 {
		t.Fatalf("cover height = %d, want 33", layout.cover.height)
	}
	if layout.cover.width+1+layout.upNext.width != 128 {
		t.Fatalf("column widths = %d + 1 + %d, want 128", layout.cover.width, layout.upNext.width)
	}
	if layout.upNext.width >= layout.cover.width {
		t.Fatalf("right column = %d, cover = %d; right should stay narrower", layout.upNext.width, layout.cover.width)
	}
	ratio := float64(layout.upNext.width) / float64(layout.cover.width+1+layout.upNext.width)
	if ratio < 0.30 || ratio > 0.35 {
		t.Fatalf("right column ratio = %.3f, want 0.30..0.35", ratio)
	}
	if layout.art.width <= 0 || !coverPaneContains(layout.cover, layout.art.left, layout.art.top) {
		t.Fatalf("art pane = %+v, cover = %+v", layout.art, layout.cover)
	}
	if layout.art.left != layout.cover.left+2 {
		t.Fatalf("art should be left-aligned, art.left = %d", layout.art.left)
	}
	if layout.info.width < 24 {
		t.Fatalf("info width = %d, want at least 24", layout.info.width)
	}
	if layout.info.left <= layout.art.left+layout.art.width {
		t.Fatalf("info pane overlaps the art: art = %+v, info = %+v", layout.art, layout.info)
	}
	if layout.info.left+layout.info.width > layout.cover.left+layout.cover.width-2 {
		t.Fatalf("info pane escapes the cover panel: info = %+v, cover = %+v", layout.info, layout.cover)
	}
	if layout.art.height < 4 || layout.info.height < 4 {
		t.Fatalf("hero panes too small: art = %+v, info = %+v", layout.art, layout.info)
	}
	hero := m.coverHeroComposition(layout.cover)
	if len(hero.metaLines) == 0 {
		t.Fatal("hero metadata block is empty")
	}
	if hero.progressTop <= hero.metaTop || hero.controlsTop != hero.progressTop+1 ||
		hero.volumeTop != hero.controlsTop+2 {
		t.Fatalf("hero transport rows = meta %d, progress %d, controls %d, volume %d",
			hero.metaTop, hero.progressTop, hero.controlsTop, hero.volumeTop)
	}
	if hero.metaTop < 0 || hero.volumeTop <= hero.controlsTop {
		t.Fatalf("hero block rows = meta %d, volume %d", hero.metaTop, hero.volumeTop)
	}
	blockH := hero.volumeTop - hero.metaTop + 1
	artTop := hero.art.top - (layout.cover.top + 1)
	titleLines := m.coverTitleLines(hero.info.width)
	want := artTop + hero.art.height/2 - titleLines/2
	if bottom := want + blockH - 1; bottom > artTop+hero.art.height-1 {
		want -= bottom - (artTop + hero.art.height - 1)
	}
	if want < artTop {
		want = artTop
	}
	if hero.metaTop != want {
		t.Fatalf("hero block top = %d, want %d (title %d lines, art height %d, block %d)",
			hero.metaTop, want, titleLines, hero.art.height, blockH)
	}
	if hero.volumeTop >= layout.cover.height-2 {
		t.Fatalf("volume row = %d escapes the panel inner height %d", hero.volumeTop, layout.cover.height-2)
	}
}

func TestCoverHeroSpectrumHeight(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	cases := []struct {
		w, h int
		min  int
	}{
		{130, 36, 8},
		{180, 45, 8},
		{96, 28, 6},
	}
	for _, tc := range cases {
		layout := m.coverLayout(tc.w, tc.h)
		if !layout.grid {
			t.Fatalf("%dx%d should use the grid", tc.w, tc.h)
		}
		art, info, rows := coverHeroPanes(layout.cover)
		if rows < tc.min {
			t.Fatalf("%dx%d spectrum rows = %d, want at least %d", tc.w, tc.h, rows, tc.min)
		}
		if art.height+rows > layout.cover.height-2 {
			t.Fatalf("%dx%d art + spectrum = %d rows, pane inner = %d", tc.w, tc.h, art.height+rows, layout.cover.height-2)
		}
		if info.height != layout.cover.height-2-rows {
			t.Fatalf("%dx%d info height = %d", tc.w, tc.h, info.height)
		}
	}
}

func TestCoverHeaderReplacesLibraryCounters(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	m.paused = true

	header := strings.Split(ansi.Strip(m.renderCover(130, 36)), "\n")[0]
	if !strings.Contains(header, "PAUSED") || !strings.Contains(header, "vol 80%") {
		t.Fatalf("cover header = %q, want playback state and volume", header)
	}
	if strings.Contains(header, "tracks") || strings.Contains(header, "All tracks") {
		t.Fatalf("cover header still shows library counters: %q", header)
	}
}

func TestCoverHeroOwnsTrackInfo(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	frame := ansi.Strip(m.renderCover(130, 36))
	if !strings.Contains(frame, "Night Drive") || !strings.Contains(frame, "Aurora") {
		t.Fatalf("hero block is missing the track info:\n%s", frame)
	}
	if got := strings.Count(frame, "Night Drive"); got != 1 {
		t.Fatalf("track title appears %d times, want only in the hero block:\n%s", got, frame)
	}
	if strings.Contains(frame, "PLAYER") {
		t.Fatalf("the separate player panel should be gone:\n%s", frame)
	}
}

func TestCoverHeroInfoCentered(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 180, 45
	layout := m.coverLayout(180, 45)
	hero := m.coverHeroComposition(layout.cover)
	lines := m.coverInfoLines(hero.info.width)
	if len(lines) != 2 {
		t.Fatalf("hero info lines = %d, want title and artist only", len(lines))
	}
	for i, line := range lines {
		plain := ansi.Strip(line)
		if strings.TrimSpace(plain) == "" {
			t.Fatalf("hero info line %d is empty", i)
		}
		if lead := len(plain) - len(strings.TrimLeft(plain, " ")); lead == 0 {
			t.Fatalf("hero info line %d is not centred: %q", i, plain)
		}
		if trail := len(plain) - len(strings.TrimRight(plain, " ")); trail == 0 {
			t.Fatalf("hero info line %d has no trailing padding: %q", i, plain)
		}
	}
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(joined, "Neon Skyline") || strings.Contains(joined, "MP3") {
		t.Fatalf("album/format should not be in the hero block:\n%s", joined)
	}
}

// addQueueTracks appends unique tracks to the library and the queue so the
// UP NEXT panel can be tested with more rows than a single screen.
func addQueueTracks(m *Model, n int) {
	dir := filepath.Dir(m.tracks[0].Path)
	for i := 0; i < n; i++ {
		track := m.tracks[i%len(m.tracks)]
		track.Path = filepath.Join(dir, fmt.Sprintf("extra-%03d.mp3", i))
		track.Title = fmt.Sprintf("Extra %03d", i)
		m.tracks = append(m.tracks, track)
		m.queue = append(m.queue, track.Path)
	}
	m.rebuildVisible("")
}

func TestCoverUpNextFillsHeight(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	addQueueTracks(m, 40)
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	limit := m.coverUpNextLimit(layout.upNext)
	if limit != layout.upNext.height-4 {
		t.Fatalf("up next limit = %d, want pane height - 4 = %d", limit, layout.upNext.height-4)
	}
	tracks := m.upNextTracks(limit)
	if len(tracks) != limit {
		t.Fatalf("visible tracks = %d, want %d", len(tracks), limit)
	}

	frame := ansi.Strip(m.renderCover(130, 36))
	wantMore := fmt.Sprintf("+%d more", len(m.queue)-1-limit)
	if !strings.Contains(frame, wantMore) {
		t.Fatalf("queue footer is missing %q:\n%s", wantMore, frame)
	}
	footerY := layout.upNext.top + 1 + 1 + len(tracks)
	if _, ok := m.coverUpNextTrackAt(layout.upNext, layout.upNext.left+2, footerY); ok {
		t.Fatal("the queue footer should not be clickable")
	}
}

func TestCoverUpNextEndOfQueueFooter(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	frame := ansi.Strip(m.renderCover(130, 36))
	if !strings.Contains(frame, "End of queue") {
		t.Fatalf("short queue should end with an explicit footer:\n%s", frame)
	}
	if strings.Contains(frame, "more") {
		t.Fatalf("short queue should not advertise hidden tracks:\n%s", frame)
	}
}

func TestCoverGridShowsThreePanes(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36

	frame := m.renderCover(130, 36)
	for _, want := range []string{
		"NOW PLAYING",
		"SPECTRUM",
		"UP NEXT",
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
	if strings.Contains(frame, "PLAYER") {
		t.Fatalf("the separate player panel should be gone:\n%s", frame)
	}
	if strings.Contains(frame, "VISUALIZER") {
		t.Fatalf("visualizer should be inside the now-playing pane:\n%s", frame)
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
		{88, 24, true},
		{87, 24, false},
		{84, 24, false},
		{80, 22, false},
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
		if tc.grid && (!strings.Contains(frame, "NOW PLAYING") || !strings.Contains(frame, "SPECTRUM")) {
			t.Fatalf("%dx%d grid is missing the combined now-playing pane:\n%s", tc.w, tc.h, frame)
		}
		if !tc.grid && strings.Contains(frame, "SPECTRUM") {
			t.Fatalf("%dx%d compact layout should not show SPECTRUM:\n%s", tc.w, tc.h, frame)
		}
	}
}

func TestCoverGridResize(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	for _, size := range [][2]int{{130, 36}, {110, 32}, {96, 28}, {88, 24}, {87, 24}, {84, 24}, {60, 24}, {88, 24}} {
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
	hero := m.coverHeroComposition(layout.cover)
	start, barWidth := m.coverProgressGeometry(130)
	if barWidth < 1 {
		t.Fatal("empty progress bar")
	}
	m.Update(tea.MouseMsg{
		X:      start + barWidth/2,
		Y:      layout.cover.top + 1 + hero.progressTop,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if m.position < 90*time.Second || m.position > 110*time.Second {
		t.Fatalf("position = %v, want about 100s", m.position)
	}
}

func TestCoverVolumeSliderClick(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	hero := m.coverHeroComposition(layout.cover)
	start, end := m.coverVolumeBarBounds(130)
	if start < 0 || end <= start {
		t.Fatalf("volume slider bounds = %d..%d", start, end)
	}
	m.Update(tea.MouseMsg{
		X:      start + (end-start)/2,
		Y:      layout.cover.top + 1 + hero.volumeTop,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if m.volume < 45 || m.volume > 55 {
		t.Fatalf("volume = %d, want about 50", m.volume)
	}
	m.Update(tea.MouseMsg{
		X:      start - 1,
		Y:      layout.cover.top + 1 + hero.volumeTop,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if m.volume >= 50 {
		t.Fatalf("volume minus did not decrease volume: %d", m.volume)
	}
}

func TestCoverControlsCluster(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	hero := m.coverHeroComposition(layout.cover)
	controls := m.coverControlsLine(hero.info.width)
	if controls.shuffleStart < 0 || controls.repeatStart < 0 {
		t.Fatalf("shuffle/repeat hidden in a %d-col info column: %+v", hero.info.width, controls)
	}
	if controls.shuffleEnd > controls.prevStart {
		t.Fatalf("shuffle is not left of prev: %+v", controls)
	}
	if controls.repeatStart < controls.nextEnd {
		t.Fatalf("repeat is not right of next: %+v", controls)
	}
	if controls.prevEnd > controls.playStart || controls.playEnd > controls.nextStart {
		t.Fatalf("transport order is broken: %+v", controls)
	}
}

func TestCoverVolumeSliderCentered(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 180, 45
	layout := m.coverLayout(180, 45)
	if !layout.grid {
		t.Fatal("180x45 should use the grid")
	}
	hero := m.coverHeroComposition(layout.cover)
	line := ansi.Strip(m.coverVolumeLine(hero.info.width))
	lead := len(line) - len(strings.TrimLeft(line, " "))
	if lead == 0 {
		t.Fatalf("volume slider is not centred in an info column of %d cols: %q", hero.info.width, line)
	}
	if strings.TrimSpace(line) == "" {
		t.Fatal("volume slider line is empty")
	}
}

func TestCoverGridSpectrumBarCount(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	layout := m.coverLayout(130, 36)
	want := (layout.cover.width - 4) / 2
	if want < 4 {
		want = 4
	}
	if want > 128 {
		want = 128
	}
	if got := m.spectrumBarCount(); got != want {
		t.Fatalf("spectrum bars = %d, want %d", got, want)
	}
	m.width, m.height = 180, 45
	layout = m.coverLayout(180, 45)
	want = (layout.cover.width - 4) / 2
	if want <= 32 {
		t.Fatalf("wide cover should need more than 32 bars, got %d", want)
	}
	if got := m.spectrumBarCount(); got != want {
		t.Fatalf("wide spectrum bars = %d, want %d", got, want)
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
		if strings.Contains(lines[i], "PLAYER") || strings.Contains(lines[i], "UP NEXT") {
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
