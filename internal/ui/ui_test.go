package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/audio"
	"tuiplay/internal/config"
	"tuiplay/internal/download"
	"tuiplay/internal/library"
	"tuiplay/internal/model"
	"tuiplay/internal/playlist"
	"tuiplay/internal/presence"
	"tuiplay/internal/session"
	"tuiplay/internal/stats"
)

func testModel(t *testing.T) (*Model, func()) {
	t.Helper()
	base := t.TempDir()
	cfg := config.Config{
		BaseDir:   base,
		MusicDir:  filepath.Join(base, "music"),
		DataDir:   filepath.Join(base, "data"),
		CoversDir: filepath.Join(base, "data", "covers"),
		TempDir:   filepath.Join(base, "data", "tmp"),
	}
	for _, dir := range []string{cfg.MusicDir, cfg.DataDir, cfg.CoversDir, cfg.TempDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := playlist.Load(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	lib := library.New(cfg.MusicDir, cfg.DataDir)
	eng := audio.New(0.5)
	mgr := download.NewManager(download.Options{
		MusicDir: cfg.MusicDir,
		DataDir:  cfg.DataDir,
		TempDir:  cfg.TempDir,
	})
	m := New(cfg, lib, pl, stats.Load(cfg.DataDir), eng, mgr, session.Default())
	m.loading = false
	m.restored = true
	m.width, m.height = 110, 32

	tracks := []model.Track{
		{
			Path:     filepath.Join(cfg.MusicDir, "01 - Aurora - Night Drive.mp3"),
			Title:    "Night Drive",
			Artist:   "Aurora",
			Album:    "Neon Skyline",
			Year:     "2024",
			Duration: 3*time.Minute + 21*time.Second,
		},
		{
			Path:     filepath.Join(cfg.MusicDir, "02 - Lumen - Paper Planes.mp3"),
			Title:    "Paper Planes",
			Artist:   "Lumen",
			Album:    "Small Hours",
			Year:     "2022",
			Duration: 4*time.Minute + 2*time.Second,
		},
		{
			Path:     filepath.Join(cfg.MusicDir, "03 - Vela - Slow Motion.mp3"),
			Title:    "Slow Motion",
			Artist:   "Vela",
			Album:    "Drift",
			Year:     "2025",
			Duration: 2*time.Minute + 48*time.Second,
		},
	}
	m.tracks = tracks
	cur := tracks[0]
	m.current = &cur
	m.loaded = true
	m.paused = false
	m.duration = cur.Duration
	m.position = 42 * time.Second
	m.queue = []string{tracks[0].Path, tracks[1].Path, tracks[2].Path}
	m.queueIndex = 0
	m.rebuildVisible("")

	cleanup := func() {
		mgr.Close()
		eng.Stop()
	}
	return m, cleanup
}

func assertFrame(t *testing.T, m *Model, label string) {
	t.Helper()
	w, h := m.dims()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Fatalf("%s: got %d lines, want %d", label, len(lines), h)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("%s: line %d width = %d, want %d: %q", label, i, got, w, ansi.Strip(line))
		}
	}
}

func TestViewFrames(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	sizes := [][2]int{{130, 40}, {110, 32}, {92, 28}, {88, 26}, {84, 24}, {72, 20}, {60, 18}, {54, 16}}
	for _, size := range sizes {
		m.width, m.height = size[0], size[1]
		m.resizeInputs()
		assertFrame(t, m, "main")
	}

	m.width, m.height = 110, 32
	m.coverMode = true
	for _, size := range sizes {
		m.width, m.height = size[0], size[1]
		m.resizeInputs()
		assertFrame(t, m, "cover")
	}
	m.coverMode = false
	m.width, m.height = 110, 32

	for _, modal := range []modalKind{
		modalAddLink,
		modalPreview,
		modalNewPlaylist,
		modalAddToPlaylist,
		modalConfirmRemoveFromPlaylist,
		modalConfirmDeleteTrack,
		modalConfirmDeletePlaylist,
		modalHelp,
		modalDownloads,
		modalEditTags, modalDuplicates, modalThemes, modalUpdate,
	} {
		m.modal = modal
		if modal == modalConfirmDeleteTrack || modal == modalConfirmRemoveFromPlaylist {
			m.deleteTracks = []model.Track{m.tracks[0]}
		}
		if modal == modalConfirmRemoveFromPlaylist {
			m.view = "Mix"
		}
		if modal == modalConfirmDeletePlaylist {
			m.deletePlaylist = "Mix"
		}
		if modal == modalAddToPlaylist {
			m.pendingTrack = m.tracks[0]
			m.hasPending = true
		}
		if modal == modalPreview {
			m.preview = &download.Preview{
				Kind:     download.KindYouTubePlaylist,
				Title:    "Night Drive",
				Subtitle: "Aurora",
				Tracks: []download.PreviewTrack{
					{Title: "Night Drive", Artist: "Aurora", Duration: 3 * time.Minute},
					{Title: "Sunset", Artist: "Aurora", Duration: 2 * time.Minute},
				},
				TotalDuration: 5 * time.Minute,
			}
		}
		assertFrame(t, m, "modal")
	}

	m.modal = modalConfirmDeleteTrack
	m.deleteTracks = append([]model.Track(nil), m.tracks...)
	assertFrame(t, m, "multi delete modal")

	m.modal = modalAddToPlaylist
	m.pendingTrack = m.tracks[0]
	m.pendingPaths = []string{m.tracks[0].Path, m.tracks[1].Path}
	m.hasPending = true
	assertFrame(t, m, "multi add modal")
}

func TestLoadingAndTooSmallFrames(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.loading = true
	m.width, m.height = 100, 30
	assertFrame(t, m, "loading")

	m.loading = false
	m.loadErr = os.ErrNotExist
	assertFrame(t, m, "load error")
	m.loadErr = nil

	m.width, m.height = 40, 12
	assertFrame(t, m, "too small")
}

func TestSearchFlow(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}); cmd == nil {
		t.Fatal("expected focus command from search")
	}
	if !m.searching {
		t.Fatal("search input was not activated")
	}
	for _, r := range "paper" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.visible) != 1 || m.visible[0].Title != "Paper Planes" {
		t.Fatalf("unexpected search result: %+v", m.visible)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.search != "" {
		t.Fatal("escape should clear the search")
	}
	if len(m.visible) != 3 {
		t.Fatalf("visible tracks after clear = %d, want 3", len(m.visible))
	}
}

func TestPlaylistCreationAndAdd(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.openNewPlaylist()
	m.nameInput.SetValue("Road Trip")
	m.submitPlaylistName()
	if !m.playlists.Has("Road Trip") {
		t.Fatal("playlist was not created")
	}

	m.view = "all"
	m.cursor = 1
	m.openAddToPlaylist()
	m.addPendingToPlaylist()
	pl, ok := m.playlists.Get("Road Trip")
	if !ok || len(pl.Tracks) != 1 {
		t.Fatalf("playlist contents = %+v", pl)
	}
	if trackKey(pl.Tracks[0]) != trackKey(m.tracks[1].Path) {
		t.Fatalf("wrong track added: %s", pl.Tracks[0])
	}
}

func TestSidebarNavigation(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("One"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Two"); err != nil {
		t.Fatal(err)
	}
	m.focus = focusSidebar
	m.sidebarCursor = 0
	m.moveSidebar(3)
	if m.sidebarCursor != 3 {
		t.Fatalf("sidebar cursor = %d, want 3", m.sidebarCursor)
	}
	m.openViewAtCursor()
	if m.view != "One" {
		t.Fatalf("view = %q, want One", m.view)
	}
	m.moveSidebar(10)
	if m.sidebarCursor != 4 {
		t.Fatalf("sidebar cursor should clamp to 4, got %d", m.sidebarCursor)
	}
}

func TestVolumeAndRepeatControls(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.setVolume(90)
	m.setVolume(120)
	if m.volume != 100 {
		t.Fatalf("volume = %d, want 100", m.volume)
	}
	m.setVolume(-5)
	if m.volume != 0 {
		t.Fatalf("volume = %d, want 0", m.volume)
	}
	m.cycleRepeat()
	if m.repeat != "all" {
		t.Fatalf("repeat = %q, want all", m.repeat)
	}
	m.cycleRepeat()
	if m.repeat != "one" {
		t.Fatalf("repeat = %q, want one", m.repeat)
	}
	m.cycleRepeat()
	if m.repeat != "off" {
		t.Fatalf("repeat = %q, want off", m.repeat)
	}
}

func TestThemeSwitching(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	start := m.theme
	m.cycleTheme(1)
	if m.theme == start {
		t.Fatal("theme did not change")
	}
	if want := themeByIndex(themeIndex(start) + 1).Name; m.theme != want {
		t.Fatalf("theme = %q, want %q", m.theme, want)
	}
	if !m.saveDirty {
		t.Fatal("theme change should mark the session dirty")
	}
	m.cycleTheme(-1)
	if m.theme != start {
		t.Fatalf("theme = %q, want %q after cycling back", m.theme, start)
	}
	applyTheme(defaultTheme)
}

func TestInvalidThemeFallsBack(t *testing.T) {
	if got := applyTheme("definitely-not-a-theme"); got != defaultTheme {
		t.Fatalf("theme = %q, want %q", got, defaultTheme)
	}
}

func TestMouseSelectsTrackAndOpensPlaylist(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.cursor = 0
	m.Update(tea.MouseMsg{X: 40, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after clicking the second row", m.cursor)
	}

	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	m.view = "all"
	m.sidebarCursor = 0
	m.Update(tea.MouseMsg{X: 4, Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.view != "Mix" {
		t.Fatalf("view = %q, want Mix after clicking the playlist", m.view)
	}
}

func TestMouseWheelScrollsTracks(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.cursor = 0
	m.Update(tea.MouseMsg{X: 40, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.cursor == 0 {
		t.Fatal("wheel did not move the selection")
	}
}

func TestPlayerTitleLayoutResponsive(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	for _, width := range []int{130, 110, 92, 84, 72, 60, 54} {
		m.width, m.height = width, 32
		innerW := width - 4
		layout := m.playerTitleLayout(innerW)
		if got := ansi.StringWidth(layout.line); got != innerW {
			t.Fatalf("width %d: title line = %d cells, want %d", width, got, innerW)
		}
		for name, pair := range map[string][2]int{
			"shuffle": {layout.shuffleStart, layout.shuffleEnd},
			"prev":    {layout.prevStart, layout.prevEnd},
			"play":    {layout.playStart, layout.playEnd},
			"next":    {layout.nextStart, layout.nextEnd},
			"repeat":  {layout.repeatStart, layout.repeatEnd},
		} {
			if pair[0] < 0 || pair[1] > innerW || pair[0] > pair[1] {
				t.Fatalf("width %d: %s range %v outside inner width %d", width, name, pair, innerW)
			}
		}
		if layout.playEnd > layout.playStart {
			center := float64(layout.playStart+layout.playEnd) / 2
			if diff := center - float64(innerW)/2; diff < -1 || diff > 1 {
				t.Fatalf("width %d: play button center %.1f, want about %.1f", width, center, float64(innerW)/2)
			}
		}
		if layout.volBarStart >= 0 && layout.volBarEnd > innerW {
			t.Fatalf("width %d: volume bar ends at %d, inner width %d", width, layout.volBarEnd, innerW)
		}
	}
}

func TestPlayerTransportWidthsStayStable(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	defer SetASCIIIcons(false)
	m.width, m.height = 110, 32

	type layoutCheck struct {
		name string
		a, b int
	}
	compare := func(label string, a, b playerTitleLayout) {
		t.Helper()
		checks := []layoutCheck{
			{"shuffleStart", a.shuffleStart, b.shuffleStart},
			{"prevStart", a.prevStart, b.prevStart},
			{"playStart", a.playStart, b.playStart},
			{"playEnd", a.playEnd, b.playEnd},
			{"nextStart", a.nextStart, b.nextStart},
			{"repeatStart", a.repeatStart, b.repeatStart},
			{"repeatEnd", a.repeatEnd, b.repeatEnd},
		}
		for _, check := range checks {
			if check.a != check.b {
				t.Fatalf("%s: %s = %d, want %d", label, check.name, check.a, check.b)
			}
		}
	}

	for _, ascii := range []bool{false, true} {
		SetASCIIIcons(ascii)
		m.paused = false
		playing := m.playerTitleLayout(106)
		m.paused = true
		paused := m.playerTitleLayout(106)
		compare("play/pause", playing, paused)

		m.paused = false
		m.repeat = "off"
		off := m.playerTitleLayout(106)
		m.repeat = "all"
		all := m.playerTitleLayout(106)
		m.repeat = "one"
		one := m.playerTitleLayout(106)
		compare("repeat off/all", off, all)
		compare("repeat all/one", all, one)
	}
}

func TestMouseVolumeBarAndWheel(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	w, _ := m.dims()
	for _, width := range []int{w, 59} {
		start, end := m.volumeBarBounds(width)
		rows := strings.Split(m.renderPlayer(width), "\n")
		if len(rows) < 2 {
			t.Fatalf("width %d: player has %d rows, want at least 2", width, len(rows))
		}
		line := []rune(ansi.Strip(rows[1]))
		if start < 0 || end > len(line) {
			t.Fatalf("volume bounds %d..%d outside line of %d runes", start, end, len(line))
		}
		want := volumeBar(m.volume, end-start)
		if got := string(line[start:end]); got != want {
			t.Fatalf("width %d: bar at %d..%d is %q, want %q", width, start, end, got, want)
		}
	}

	_, _, contentH, _, _, _ := m.layout()
	y := 1 + contentH + 1

	start, end := m.volumeBarBounds(w)
	m.Update(tea.MouseMsg{X: end - 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.volume != 100 {
		t.Fatalf("volume = %d, want 100 after clicking the right edge", m.volume)
	}

	start, _ = m.volumeBarBounds(w)
	m.Update(tea.MouseMsg{X: start, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.volume != 0 {
		t.Fatalf("volume = %d, want 0 after clicking the left edge", m.volume)
	}

	m.setVolume(50)
	start, end = m.volumeBarBounds(w)
	m.Update(tea.MouseMsg{X: start - 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.volume != 45 {
		t.Fatalf("volume = %d, want 45 after clicking minus", m.volume)
	}
	m.Update(tea.MouseMsg{X: end, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.volume != 50 {
		t.Fatalf("volume = %d, want 50 after clicking plus", m.volume)
	}

	start, _ = m.volumeBarBounds(w)
	m.Update(tea.MouseMsg{X: start, Y: y, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.volume != 55 {
		t.Fatalf("volume = %d, want 55 after wheel up", m.volume)
	}
}

func TestASCIIControlLayout(t *testing.T) {
	SetASCIIIcons(true)
	defer SetASCIIIcons(false)

	m, cleanup := testModel(t)
	defer cleanup()
	w, _ := m.dims()
	rows := strings.Split(m.renderPlayer(w), "\n")
	line := []rune(ansi.Strip(rows[1]))
	start, end := m.volumeBarBounds(w)
	if start < 0 || end > len(line) {
		t.Fatalf("ASCII volume bounds %d..%d outside line of %d runes", start, end, len(line))
	}
	if got := string(line[start:end]); got != volumeBar(m.volume, end-start) {
		t.Fatalf("ASCII volume bar at %d..%d is %q", start, end, got)
	}
	if !strings.Contains(string(line), "SHF") || !strings.Contains(string(line), "RPT") {
		t.Fatalf("ASCII icons missing: %q", string(line))
	}
}

func TestMousePlayerControls(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	w, _ := m.dims()
	_, _, contentH, _, _, _ := m.layout()
	y := 1 + contentH + 1
	innerW := w - 4

	layout := m.playerTitleLayout(innerW)
	m.Update(tea.MouseMsg{X: playerContentX + layout.shuffleStart, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.shuffle {
		t.Fatal("clicking the shuffle icon should enable shuffle")
	}

	layout = m.playerTitleLayout(innerW)
	m.Update(tea.MouseMsg{X: playerContentX + layout.repeatStart, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.repeat != "all" {
		t.Fatalf("repeat = %q, want all", m.repeat)
	}

	volStart, volEnd := m.volumeBarBounds(w)
	m.Update(tea.MouseMsg{X: volStart, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.volDragging {
		t.Fatal("pressing the slider should start a volume drag")
	}
	m.Update(tea.MouseMsg{X: volEnd - 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if m.volume != 100 {
		t.Fatalf("volume = %d, want 100 while dragging", m.volume)
	}
	m.Update(tea.MouseMsg{X: volEnd - 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if m.volDragging {
		t.Fatal("release should stop the volume drag")
	}
}

func TestMultiSelectAndDelete(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	paths := []string{m.tracks[0].Path, m.tracks[1].Path}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m.cursor = 0
	m.Update(tea.MouseMsg{X: 40, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Ctrl: true})
	m.Update(tea.MouseMsg{X: 40, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Ctrl: true})
	if m.selectionCount() != 2 {
		t.Fatalf("selected %d tracks, want 2", m.selectionCount())
	}
	m.openDeleteTrack()
	if m.modal != modalConfirmDeleteTrack || len(m.deleteTracks) != 2 {
		t.Fatalf("delete confirmation = %v with %d tracks", m.modal, len(m.deleteTracks))
	}
	for i, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file %d was deleted before confirmation: %v", i, err)
		}
	}
	m.deleteTrackNow()
	for i, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("track %d still exists: %v", i, err)
		}
	}
	if m.selectionCount() != 0 {
		t.Fatal("selection should be cleared after deletion")
	}
}

func TestMultiSelectAllAndEscape(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.selectionCount() != len(m.visible) {
		t.Fatalf("selected %d tracks, want %d", m.selectionCount(), len(m.visible))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selectionCount() != 0 {
		t.Fatal("escape should clear the selection")
	}
}

func TestMultiAddSkipsDuplicates(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Add("Mix", m.tracks[0].Path); err != nil {
		t.Fatal(err)
	}
	m.toggleSelected(m.tracks[0].Path)
	m.toggleSelected(m.tracks[1].Path)
	m.openAddToPlaylist()
	if len(m.pendingPaths) != 2 {
		t.Fatalf("pending paths = %d, want 2", len(m.pendingPaths))
	}
	m.modalCursor = 0
	m.addPendingToPlaylist()
	pl, _ := m.playlists.Get("Mix")
	if len(pl.Tracks) != 2 {
		t.Fatalf("playlist has %d tracks, want 2", len(pl.Tracks))
	}

	m.openAddToPlaylist()
	m.modalCursor = 0
	m.addPendingToPlaylist()
	pl, _ = m.playlists.Get("Mix")
	if len(pl.Tracks) != 2 {
		t.Fatalf("duplicates were added: %+v", pl.Tracks)
	}
}

func TestMouseDragMovesTracksBetweenPlaylists(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Source"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Target"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Add("Source", m.tracks[0].Path); err != nil {
		t.Fatal(err)
	}
	m.view = "Source"
	m.rebuildVisible("")
	m.focus = focusTracks

	m.Update(tea.MouseMsg{X: 40, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if !m.dragActive || m.dragTarget != "Target" {
		t.Fatalf("drag active=%v target=%q", m.dragActive, m.dragTarget)
	}
	m.Update(tea.MouseMsg{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	source, _ := m.playlists.Get("Source")
	target, _ := m.playlists.Get("Target")
	if len(source.Tracks) != 0 {
		t.Fatalf("source still has tracks: %+v", source.Tracks)
	}
	if len(target.Tracks) != 1 || trackKey(target.Tracks[0]) != trackKey(m.tracks[0].Path) {
		t.Fatalf("target tracks = %+v", target.Tracks)
	}
}

type fakePresence struct {
	last    presence.Activity
	has     bool
	cleared bool
}

func (f *fakePresence) Update(a presence.Activity) {
	f.last = a
	f.has = true
	f.cleared = false
}

func (f *fakePresence) Clear() {
	f.cleared = true
	f.has = false
}

func (f *fakePresence) Status() <-chan presence.Status {
	return nil
}

func TestPresenceSync(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.current.CoverURL = "https://example.com/cover.jpg"
	m.current.Source = "https://open.spotify.com/track/abc"
	m.loaded = true
	m.paused = true
	m.position = 42 * time.Second
	m.duration = 3*time.Minute + 21*time.Second
	sink := &fakePresence{}
	m.SetPresence(sink)
	if !sink.has {
		t.Fatal("presence was not published")
	}
	if !sink.last.Paused || sink.last.Title != "Night Drive" || sink.last.Artist != "Aurora" {
		t.Fatalf("activity = %+v", sink.last)
	}
	if sink.last.CoverURL != "https://example.com/cover.jpg" {
		t.Fatalf("cover = %q", sink.last.CoverURL)
	}
	if sink.last.SourceURL != m.current.Source {
		t.Fatalf("source = %q", sink.last.SourceURL)
	}
	if sink.last.Position != 42*time.Second || sink.last.Duration != 3*time.Minute+21*time.Second {
		t.Fatalf("timing = %v / %v", sink.last.Position, sink.last.Duration)
	}

	m.current = nil
	m.syncPresence()
	if !sink.cleared {
		t.Fatal("presence should be cleared when nothing is playing")
	}
}

func TestPresenceClearsUnloadedSelection(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	sink := &fakePresence{}
	m.loaded = false
	m.ended = false
	m.SetPresence(sink)
	if sink.has || !sink.cleared {
		t.Fatal("a selected but unplayed track must not be published")
	}
}

func TestPresenceStatusWarns(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.presenceStatus = make(chan presence.Status, 1)
	cmd := m.handlePresenceStatus(presenceStatusMsg{status: presence.Status{Err: os.ErrPermission}})
	if m.toastKind != toastWarning || !strings.Contains(m.toast, "administrator") {
		t.Fatalf("toast = %q, kind = %v", m.toast, m.toastKind)
	}
	if cmd == nil {
		t.Fatal("expected the model to keep listening for presence status")
	}
	m.presenceStatus = nil
}

func TestMouseSeeksProgressBar(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.loaded = false
	m.paused = true
	m.duration = 100 * time.Second
	m.position = 42 * time.Second

	w, _ := m.dims()
	_, _, contentH, _, _, _ := m.layout()
	y := 1 + contentH + 3
	start, barWidth := m.playerProgressGeometry(w)
	m.Update(tea.MouseMsg{X: start + barWidth/2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.position < 49*time.Second || m.position > 51*time.Second {
		t.Fatalf("position = %v, want about 50s", m.position)
	}
}

func TestCoverModeTogglesFromArtClick(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.Update(tea.MouseMsg{X: 85, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.coverMode {
		t.Fatal("clicking the artwork should open cover mode")
	}
	m.Update(tea.MouseMsg{X: 55, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.coverMode {
		t.Fatal("clicking the large artwork should close cover mode")
	}
}

func TestLearnDurationUpdatesList(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	path := m.current.Path
	m.current.Duration = 0
	m.duration = 0
	for i := range m.tracks {
		if trackKey(m.tracks[i].Path) == trackKey(path) {
			m.tracks[i].Duration = 0
		}
	}
	m.rebuildVisible(path)

	m.learnDuration(3*time.Minute + 7*time.Second)

	want := 3*time.Minute + 7*time.Second
	if m.current.Duration != want {
		t.Fatalf("current duration = %v, want %v", m.current.Duration, want)
	}
	if got := m.tracks[0].Duration; got != want {
		t.Fatalf("library duration = %v, want %v", got, want)
	}
	if got := m.visible[0].Duration; got != want {
		t.Fatalf("visible duration = %v, want %v", got, want)
	}
}

func TestDeleteKeyInSidebarTargetsPlaylist(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	m.view = "Mix"
	m.syncSidebarCursor()
	m.focus = focusSidebar
	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.modal != modalConfirmDeletePlaylist || m.deletePlaylist != "Mix" {
		t.Fatalf("modal = %v, playlist = %q", m.modal, m.deletePlaylist)
	}
}

func TestDeleteInPlaylistRemovesOnlyFromPlaylist(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	for _, t0 := range m.tracks[:2] {
		if _, err := m.playlists.Add("Mix", t0.Path); err != nil {
			t.Fatal(err)
		}
	}
	m.view = "Mix"
	m.focus = focusTracks
	m.rebuildVisible("")
	m.cursor = 0

	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if m.modal != modalConfirmRemoveFromPlaylist {
		t.Fatalf("modal = %v, want remove-from-playlist confirmation", m.modal)
	}
	if len(m.deleteTracks) != 1 {
		t.Fatalf("deleteTracks = %d, want 1", len(m.deleteTracks))
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.modal != modalNone {
		t.Fatalf("modal = %v, want closed", m.modal)
	}
	pl, ok := m.playlists.Get("Mix")
	if !ok {
		t.Fatal("playlist disappeared")
	}
	if len(pl.Tracks) != 1 {
		t.Fatalf("playlist has %d tracks, want 1", len(pl.Tracks))
	}
	if len(m.tracks) != 3 {
		t.Fatalf("library has %d tracks, want 3", len(m.tracks))
	}
}

func TestRemoveFromPlaylistCanSwitchToDiskDelete(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Add("Mix", m.tracks[0].Path); err != nil {
		t.Fatal(err)
	}
	m.view = "Mix"
	m.focus = focusTracks
	m.rebuildVisible("")
	m.cursor = 0

	m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.modal != modalConfirmDeleteTrack {
		t.Fatalf("modal = %v, want disk delete confirmation", m.modal)
	}
	if len(m.deleteTracks) != 1 {
		t.Fatalf("deleteTracks = %d, want 1", len(m.deleteTracks))
	}
}

func TestDeletePlaylistFlow(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if _, err := m.playlists.Create("Trash"); err != nil {
		t.Fatal(err)
	}
	m.view = "Trash"
	m.sidebarCursor = 3
	m.openDeletePlaylist()
	m.deletePlaylistNow()
	if m.playlists.Has("Trash") {
		t.Fatal("playlist was not deleted")
	}
	if m.view != "all" {
		t.Fatalf("view = %q, want all", m.view)
	}
}

func TestFavoritesView(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if err := m.stats.SetFavorite(m.tracks[0].Path, true); err != nil {
		t.Fatal(err)
	}
	m.view = viewFavorites
	m.rebuildVisible("")
	if len(m.visible) != 1 || m.visible[0].Title != m.tracks[0].Title {
		t.Fatalf("favorites = %+v", m.visible)
	}

	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if len(m.visible) != 0 {
		t.Fatalf("favorite was not removed: %+v", m.visible)
	}
	if m.stats.FavoriteCount() != 0 {
		t.Fatalf("favorite count = %d, want 0", m.stats.FavoriteCount())
	}
}

func TestSleepTimerPausesPlayback(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if !m.sleepAt.IsZero() {
		t.Fatal("timer should start disabled")
	}
	m.cycleSleepTimer()
	if m.sleepAt.IsZero() {
		t.Fatal("timer was not armed")
	}
	m.paused = false
	m.sleepAt = time.Now().Add(-time.Second)
	m.checkSleepTimer()
	if !m.paused {
		t.Fatal("sleep timer did not pause playback")
	}
	if !m.sleepAt.IsZero() {
		t.Fatal("timer was not cleared after firing")
	}
}

func TestSortByYear(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.sortMode = "year"
	m.view = viewAll
	m.rebuildVisible("")
	if len(m.visible) != 3 {
		t.Fatalf("visible = %d, want 3", len(m.visible))
	}
	want := []string{"Slow Motion", "Night Drive", "Paper Planes"}
	for i, title := range want {
		if m.visible[i].Title != title {
			t.Fatalf("order[%d] = %q, want %q (all: %+v)", i, m.visible[i].Title, title, m.visible)
		}
	}
}

func TestM3UImportAndExport(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	src := filepath.Join(m.cfg.MusicDir, "import.m3u8")
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, tr := range m.tracks[:2] {
		fmt.Fprintf(&b, "#EXTINF:0,%s - %s\n%s\n", tr.DisplayArtist(), tr.DisplayTitle(), tr.Path)
	}
	if err := os.WriteFile(src, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	m.openImportM3U()
	m.m3uInput.SetValue(src)
	m.submitImportM3U()
	if !m.playlists.Has("import") {
		t.Fatal("import playlist was not created")
	}
	pl, _ := m.playlists.Get("import")
	if len(pl.Tracks) != 2 {
		t.Fatalf("imported tracks = %d, want 2", len(pl.Tracks))
	}

	dst := filepath.Join(m.cfg.MusicDir, "export.m3u8")
	m.view = viewAll
	m.rebuildVisible("")
	m.openExportM3U()
	m.m3uInput.SetValue(dst)
	m.submitExportM3U()
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("exported playlist missing: %v", err)
	}
}

func TestDeleteCurrentTrackRemovesFile(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	path := filepath.Join(m.cfg.MusicDir, "delete-me.wav")
	if err := os.WriteFile(path, []byte("not really audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := model.Track{Path: path, Title: "Delete Me", Artist: "Tester"}
	m.tracks = append(m.tracks, tr)
	m.rebuildVisible(path)
	m.current = &tr
	m.loaded = true
	m.deleteTracks = []model.Track{tr}
	m.modal = modalConfirmDeleteTrack

	m.deleteTrackNow()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("track file still exists: %v", err)
	}
	if m.current != nil {
		t.Fatalf("current track was not cleared: %+v", m.current)
	}
	if m.loaded {
		t.Fatal("loaded flag should be false after deleting the current track")
	}
}
