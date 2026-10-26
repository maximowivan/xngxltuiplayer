package ui

import (
	"fmt"
	"image"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tuiplay/internal/audio"
	"tuiplay/internal/config"
	"tuiplay/internal/download"
	"tuiplay/internal/library"
	"tuiplay/internal/model"
	"tuiplay/internal/playlist"
	"tuiplay/internal/presence"
	"tuiplay/internal/session"
)

type focusArea int

const (
	focusTracks focusArea = iota
	focusSidebar
)

const crossfadeDuration = 5 * time.Second

type modalKind int

const (
	modalNone modalKind = iota
	modalAddLink
	modalNewPlaylist
	modalAddToPlaylist
	modalConfirmRemoveFromPlaylist
	modalConfirmDeleteTrack
	modalConfirmDeletePlaylist
	modalHelp
	modalDownloads
)

type toastKind int

const (
	toastInfo toastKind = iota
	toastSuccess
	toastWarning
	toastError
)

type tickMsg time.Time

type libraryLoadedMsg struct {
	tracks     []model.Track
	err        error
	background bool
}

type artLoadedMsg struct {
	path string
	img  image.Image
	err  error
}

// Model is the Bubble Tea application state.
type Model struct {
	cfg            config.Config
	lib            *library.Library
	playlists      *playlist.Store
	engine         *audio.Engine
	mgr            *download.Manager
	initial        session.State
	presence       PresenceSink
	presenceStatus <-chan presence.Status
	presenceWarned bool

	width  int
	height int

	theme     string
	coverMode bool

	volDragging bool

	lastClickAt    time.Time
	lastClickRow   int
	lastClickValid bool

	loading     bool
	loadErr     error
	restored    bool
	refreshing  bool
	needRefresh bool

	tracks  []model.Track
	visible []model.Track
	view    string
	focus   focusArea

	sidebarCursor int
	sidebarOffset int
	cursor        int
	listOffset    int

	search      string
	searchInput textinput.Model
	searching   bool

	modal          modalKind
	linkInput      textinput.Model
	nameInput      textinput.Model
	modalCursor    int
	modalErr       string
	pendingTrack   model.Track
	pendingPaths   []string
	hasPending     bool
	deleteTracks   []model.Track
	deletePlaylist string

	selection map[string]bool

	dragPaths  []string
	dragSource string
	dragActive bool
	dragTarget string
	dragStartX int
	dragStartY int

	current        *model.Track
	loaded         bool
	paused         bool
	ended          bool
	position       time.Duration
	duration       time.Duration
	engineGen      uint64
	crossfadeTried bool

	volume   int
	repeat   string
	shuffle  bool
	spectrum []float64

	artImage   image.Image
	artFor     string
	artLoading bool
	artErr     error

	queue          []string
	queueIndex     int
	shuffleHistory []int

	jobs         []download.Job
	jobSeen      map[int]download.Status
	downloadsCur int
	downloadsOff int
	downloadsID  int

	toast      string
	toastKind  toastKind
	toastUntil time.Time

	lastSave  time.Time
	saveDirty bool
	spinner   int
}

// New constructs the UI model.
func New(cfg config.Config, lib *library.Library, playlists *playlist.Store,
	engine *audio.Engine, mgr *download.Manager, initial session.State) *Model {

	volume := initial.Volume
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	repeat := initial.Repeat
	if repeat != "off" && repeat != "all" && repeat != "one" {
		repeat = "off"
	}
	view := initial.View
	if view == "" {
		view = "all"
	}

	m := &Model{
		cfg:       cfg,
		lib:       lib,
		playlists: playlists,
		engine:    engine,
		mgr:       mgr,
		initial:   initial,
		width:     110,
		height:    32,
		loading:   true,
		view:      view,
		focus:     focusTracks,
		volume:    volume,
		repeat:    repeat,
		shuffle:   initial.Shuffle,
		theme:     applyTheme(initial.Theme),
		jobSeen:   make(map[int]download.Status),
		lastSave:  time.Now(),
	}

	m.linkInput = newInput("› ", "https://open.spotify.com/track/…", 4096)
	m.nameInput = newInput("› ", "Playlist name", 48)
	m.searchInput = newInput("/ ", "search title, artist, album…", 120)

	engine.SetVolume(float64(volume) / 100)
	return m
}

func newInput(prompt, placeholder string, limit int) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.Placeholder = placeholder
	in.CharLimit = limit
	in.PromptStyle = accent2Style
	in.TextStyle = textStyle
	in.PlaceholderStyle = faintStyle
	in.Cursor.Style = lipgloss.NewStyle().Foreground(colAccent2)
	in.Cursor.TextStyle = textStyle
	return in
}

// Init starts the first library scan, the UI ticker and window title.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("xngxltuiplayer"),
		m.loadLibraryCmd(false),
		m.waitPresenceStatus(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) loadLibraryCmd(background bool) tea.Cmd {
	return func() tea.Msg {
		tracks, err := m.lib.Scan()
		return libraryLoadedMsg{tracks: tracks, err: err, background: background}
	}
}

func (m *Model) startRefresh() tea.Cmd {
	if m.refreshing {
		m.needRefresh = true
		return nil
	}
	m.refreshing = true
	m.needRefresh = false
	return m.loadLibraryCmd(true)
}

func (m *Model) loadArtCmd(path string) tea.Cmd {
	return func() tea.Msg {
		coverPath, err := m.lib.Cover(model.Track{Path: path})
		if err != nil {
			return artLoadedMsg{path: path, err: err}
		}
		f, err := os.Open(coverPath)
		if err != nil {
			return artLoadedMsg{path: path, err: err}
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		return artLoadedMsg{path: path, img: img, err: err}
	}
}

func (m *Model) beginArt(path string) tea.Cmd {
	m.artFor = path
	m.artImage = nil
	m.artErr = nil
	m.artLoading = true
	return m.loadArtCmd(path)
}

func (m *Model) dims() (w, h int) {
	w, h = m.width, m.height
	if w <= 0 {
		w = 110
	}
	if h <= 0 {
		h = 32
	}
	return w, h
}

func (m *Model) layout() (w, h, contentH, sidebarW, rightW, centerW int) {
	w, h = m.dims()
	sidebarW, rightW = paneWidths(w)
	contentH = h - 3 - playerHeightFor(w, h, rightW)
	if contentH < 8 {
		contentH = 8
	}
	for {
		centerW = w - sidebarW - rightW
		if sidebarW > 0 {
			centerW--
		}
		if rightW > 0 {
			centerW--
		}
		if centerW >= 28 {
			break
		}
		if rightW > 0 {
			rightW = 0
			continue
		}
		if sidebarW > 0 {
			sidebarW = 0
			continue
		}
		break
	}
	return
}

func paneWidths(w int) (sidebar, right int) {
	switch {
	case w >= 124:
		return 22, 42
	case w >= 104:
		return 20, 36
	case w >= 92:
		return 20, 32
	case w >= 84:
		return 18, 30
	case w >= 72:
		return 18, 0
	default:
		return 0, 0
	}
}

// playerHeightFor keeps the transport framed at normal sizes and falls back to
// a compact three-line player in short terminals.
func playerHeightFor(w, h, rightW int) int {
	if h < 20 {
		return 5 // title, progress, transport + borders
	}
	if rightW == 0 && w >= 54 && h >= 24 {
		return 7 // title, progress, transport, meta, compact FFT + borders
	}
	return 6 // title, progress, transport, meta + borders
}

func (m *Model) playerHeight() int {
	w, h := m.dims()
	_, rightW := paneWidths(w)
	return playerHeightFor(w, h, rightW)
}

func (m *Model) listRows() int {
	_, _, contentH, _, _, _ := m.layout()
	rows := contentH - 2 - 1
	if m.searching {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) sidebarRows() int {
	_, _, contentH, _, _, _ := m.layout()
	rows := contentH - 2 - 3
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) spectrumBarCount() int {
	w, _ := m.dims()
	_, _, _, _, rightW, _ := m.layout()
	var avail int
	switch {
	case rightW > 0:
		avail = rightW - 4
	case w >= 54:
		avail = w - 4
	default:
		return 0
	}
	bars := avail / 2
	if bars < 4 {
		bars = 4
	}
	if bars > 32 {
		bars = 32
	}
	return bars
}

func trackKey(path string) string {
	p := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

func (m *Model) findTrack(path string) (model.Track, bool) {
	key := trackKey(path)
	for _, t := range m.tracks {
		if trackKey(t.Path) == key {
			return t, true
		}
	}
	return model.Track{}, false
}

func (m *Model) selectedTrack() (model.Track, bool) {
	if m.cursor >= 0 && m.cursor < len(m.visible) {
		return m.visible[m.cursor], true
	}
	return model.Track{}, false
}

func (m *Model) baseTracks() []model.Track {
	if m.view == "all" {
		return m.tracks
	}
	pl, ok := m.playlists.Get(m.view)
	if !ok {
		return m.tracks
	}
	byPath := make(map[string]model.Track, len(m.tracks))
	for _, t := range m.tracks {
		byPath[trackKey(t.Path)] = t
	}
	out := make([]model.Track, 0, len(pl.Tracks))
	for _, p := range pl.Tracks {
		if t, ok := byPath[trackKey(p)]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (m *Model) rebuildVisible(preservePath string) {
	if m.view != "all" && !m.playlists.Has(m.view) {
		m.view = "all"
	}
	if preservePath == "" {
		if t, ok := m.selectedTrack(); ok {
			preservePath = t.Path
		}
	}

	base := m.baseTracks()
	q := strings.ToLower(strings.TrimSpace(m.search))
	if q == "" {
		m.visible = append([]model.Track(nil), base...)
	} else {
		filtered := make([]model.Track, 0, len(base))
		for _, t := range base {
			hay := strings.ToLower(t.DisplayTitle() + " " + t.DisplayArtist() + " " + t.Album + " " + filepath.Base(t.Path))
			if strings.Contains(hay, q) {
				filtered = append(filtered, t)
			}
		}
		m.visible = filtered
	}

	m.cursor = 0
	if preservePath != "" {
		for i, t := range m.visible {
			if trackKey(t.Path) == trackKey(preservePath) {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = maxInt(0, len(m.visible)-1)
	}
	m.clampListOffset()
	m.syncSidebarCursor()
	m.lastClickValid = false
	m.clearSelection()
}

func (m *Model) clampListOffset() {
	rows := m.listRows()
	if len(m.visible) == 0 {
		m.listOffset = 0
		return
	}
	if m.cursor < m.listOffset {
		m.listOffset = m.cursor
	}
	if m.cursor >= m.listOffset+rows {
		m.listOffset = m.cursor - rows + 1
	}
	maxOff := maxInt(0, len(m.visible)-rows)
	if m.listOffset > maxOff {
		m.listOffset = maxOff
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

func (m *Model) clampSidebarOffset() {
	rows := m.sidebarRows()
	count := len(m.playlists.Names())
	idx := m.sidebarCursor
	if idx > 0 {
		idx--
	} else {
		idx = 0
	}
	if idx < m.sidebarOffset {
		m.sidebarOffset = idx
	}
	if idx >= m.sidebarOffset+rows {
		m.sidebarOffset = idx - rows + 1
	}
	maxOff := maxInt(0, count-rows)
	if m.sidebarOffset > maxOff {
		m.sidebarOffset = maxOff
	}
	if m.sidebarOffset < 0 {
		m.sidebarOffset = 0
	}
}

func (m *Model) syncSidebarCursor() {
	if m.view == "all" {
		m.sidebarCursor = 0
		m.clampSidebarOffset()
		return
	}
	names := m.playlists.Names()
	for i, name := range names {
		if strings.EqualFold(name, m.view) {
			m.sidebarCursor = i + 1
			m.clampSidebarOffset()
			return
		}
	}
	m.sidebarCursor = 0
	m.clampSidebarOffset()
}

func (m *Model) openViewAtCursor() {
	names := m.playlists.Names()
	if m.sidebarCursor <= 0 {
		m.view = "all"
	} else if m.sidebarCursor-1 < len(names) {
		m.view = names[m.sidebarCursor-1]
	} else {
		m.sidebarCursor = 0
		m.view = "all"
	}
	m.rebuildVisible("")
	m.markDirty()
}

func (m *Model) viewLabel() string {
	if m.view == "all" {
		return "All tracks"
	}
	return m.view
}

func (m *Model) markDirty() {
	m.saveDirty = true
}

func (m *Model) setToast(message string, kind toastKind, d time.Duration) {
	message = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(message, "\r", " "), "\n", " "))
	m.toast = message
	m.toastKind = kind
	if d <= 0 {
		d = 3 * time.Second
	}
	m.toastUntil = time.Now().Add(d)
}

func (m *Model) currentPosition() time.Duration {
	if m.loaded && m.engine.Active() {
		return m.engine.Position()
	}
	return m.position
}

func (m *Model) saveSession() {
	st := session.State{
		Volume:  m.volume,
		Repeat:  m.repeat,
		Shuffle: m.shuffle,
		View:    m.view,
		Theme:   m.theme,
	}
	if m.current != nil {
		st.Track = m.current.Path
		st.PositionMS = m.currentPosition().Milliseconds()
	}
	_ = session.Save(m.cfg.DataDir, st)
	m.saveDirty = false
}

// SaveSession persists the current playback state. It is safe to call after
// the Bubble Tea program has stopped.
func (m *Model) SaveSession() {
	m.saveSession()
}

func (m *Model) restoreSession() {
	m.restored = true
	if m.initial.View != "" && m.initial.View != "all" && m.playlists.Has(m.initial.View) {
		m.view = m.initial.View
	} else {
		m.view = "all"
	}
	m.syncSidebarCursor()

	if m.initial.Track == "" {
		m.rebuildVisible("")
		return
	}
	t, ok := m.findTrack(m.initial.Track)
	if !ok {
		m.rebuildVisible("")
		return
	}
	m.current = &t
	m.duration = t.Duration
	pos := time.Duration(m.initial.PositionMS) * time.Millisecond
	if t.Duration > 0 && pos > t.Duration {
		pos = t.Duration
	}
	m.position = pos
	m.paused = true
	m.loaded = false
	m.ended = false
	m.crossfadeTried = false
	m.rebuildVisible(t.Path)
	m.buildQueueFor(t.Path)
	m.artFor = ""
	m.syncPresence()
}

func (m *Model) queueIndexFor(path string) int {
	for i, p := range m.queue {
		if trackKey(p) == trackKey(path) {
			return i
		}
	}
	return -1
}

func (m *Model) buildQueueFor(path string) {
	paths := make([]string, 0, len(m.visible))
	for _, t := range m.visible {
		paths = append(paths, t.Path)
	}
	m.queue = paths
	idx := m.queueIndexFor(path)
	if idx < 0 {
		m.queue = []string{path}
		idx = 0
	}
	m.queueIndex = idx
	if m.shuffle {
		m.shuffleHistory = []int{idx}
	} else {
		m.shuffleHistory = nil
	}
}

func (m *Model) playTrack(t model.Track, start time.Duration, rebuildQueue bool) (tea.Cmd, bool) {
	if t.Duration > 0 && start >= t.Duration {
		start = 0
	}
	if start < 0 {
		start = 0
	}
	if err := m.engine.Play(t.Path, t.Duration, start); err != nil {
		m.setToast("Playback failed: "+err.Error(), toastError, 5*time.Second)
		return nil, false
	}
	cp := t
	m.current = &cp
	m.loaded = true
	m.paused = false
	m.ended = false
	m.position = start
	m.duration = t.Duration
	m.engineGen = m.engine.Gen()
	m.crossfadeTried = false
	if rebuildQueue {
		m.buildQueueFor(t.Path)
	} else if len(m.queue) == 0 {
		m.queue = []string{t.Path}
		m.queueIndex = 0
	}
	m.markDirty()
	m.syncPresence()
	return m.beginArt(t.Path), true
}

func (m *Model) playSelected() tea.Cmd {
	t, ok := m.selectedTrack()
	if !ok {
		return nil
	}
	cmd, _ := m.playTrack(t, 0, true)
	return cmd
}

func (m *Model) togglePlay() tea.Cmd {
	if m.current == nil {
		return m.playSelected()
	}
	if m.ended {
		m.ended = false
		cmd, _ := m.playTrack(*m.current, 0, false)
		return cmd
	}
	if !m.loaded {
		start := m.position
		if m.duration > 0 && start >= m.duration {
			start = 0
		}
		cmd, _ := m.playTrack(*m.current, start, false)
		return cmd
	}
	m.paused = m.engine.TogglePause()
	m.syncPresence()
	return nil
}

func (m *Model) nextIndex() int {
	if len(m.queue) == 0 {
		return -1
	}
	if m.shuffle {
		if m.queueIndex >= 0 && !containsInt(m.shuffleHistory, m.queueIndex) {
			m.shuffleHistory = append(m.shuffleHistory, m.queueIndex)
		}
		if len(m.queue) == 1 {
			if m.repeat == "all" {
				return 0
			}
			return -1
		}
		candidates := make([]int, 0, len(m.queue))
		for i := range m.queue {
			if i == m.queueIndex || containsInt(m.shuffleHistory, i) {
				continue
			}
			candidates = append(candidates, i)
		}
		if len(candidates) == 0 {
			if m.repeat != "all" {
				return -1
			}
			m.shuffleHistory = nil
			for i := range m.queue {
				if i != m.queueIndex {
					candidates = append(candidates, i)
				}
			}
		}
		if len(candidates) == 0 {
			return -1
		}
		return candidates[rand.Intn(len(candidates))]
	}
	if m.queueIndex+1 < len(m.queue) {
		return m.queueIndex + 1
	}
	if m.repeat == "all" {
		return 0
	}
	return -1
}

func (m *Model) prevIndex() int {
	if len(m.queue) == 0 {
		return -1
	}
	if m.shuffle {
		if len(m.shuffleHistory) >= 2 {
			idx := m.shuffleHistory[len(m.shuffleHistory)-2]
			m.shuffleHistory = m.shuffleHistory[:len(m.shuffleHistory)-1]
			return idx
		}
		return -1
	}
	if m.queueIndex > 0 {
		return m.queueIndex - 1
	}
	if m.repeat == "all" {
		return len(m.queue) - 1
	}
	return -1
}

func (m *Model) playQueueIndex(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.queue) {
		return nil
	}
	t, ok := m.findTrack(m.queue[idx])
	if !ok {
		m.setToast("Track file is missing", toastError, 3*time.Second)
		return nil
	}
	cmd, ok := m.playTrack(t, 0, false)
	if !ok {
		return nil
	}
	m.queueIndex = idx
	if m.shuffle && !containsInt(m.shuffleHistory, idx) {
		m.shuffleHistory = append(m.shuffleHistory, idx)
	}
	return cmd
}

func (m *Model) nextTrack() tea.Cmd {
	if m.current == nil {
		return m.playSelected()
	}
	if len(m.queue) == 0 {
		m.queue = []string{m.current.Path}
		m.queueIndex = 0
	}
	idx := m.nextIndex()
	if idx < 0 {
		m.setToast("End of queue", toastInfo, 1500*time.Millisecond)
		return nil
	}
	return m.playQueueIndex(idx)
}

func (m *Model) prevTrack() tea.Cmd {
	if m.current == nil {
		return m.playSelected()
	}
	if len(m.queue) == 0 {
		m.queue = []string{m.current.Path}
		m.queueIndex = 0
	}
	idx := m.prevIndex()
	if idx < 0 {
		m.setToast("Start of queue", toastInfo, 1500*time.Millisecond)
		return nil
	}
	return m.playQueueIndex(idx)
}

// learnDuration fills in a duration discovered during playback, so tracks
// without ffprobe metadata stop showing 0:00 in the list.
func (m *Model) learnDuration(d time.Duration) {
	if d <= 0 || m.current == nil {
		return
	}
	key := trackKey(m.current.Path)
	if m.current.Duration <= 0 {
		m.current.Duration = d
	}
	for i := range m.tracks {
		if m.tracks[i].Duration <= 0 && trackKey(m.tracks[i].Path) == key {
			m.tracks[i].Duration = d
		}
	}
	for i := range m.visible {
		if m.visible[i].Duration <= 0 && trackKey(m.visible[i].Path) == key {
			m.visible[i].Duration = d
		}
	}
	m.syncPresence()
}

func (m *Model) onTrackEnded() tea.Cmd {
	if m.current == nil {
		return nil
	}
	m.ended = true
	m.paused = true
	m.loaded = false
	if m.duration > 0 {
		m.position = m.duration
	}
	m.engine.Stop()
	m.engineGen = m.engine.Gen()
	m.syncPresence()
	if m.repeat == "one" {
		m.ended = false
		cmd, _ := m.playTrack(*m.current, 0, false)
		return cmd
	}
	idx := m.nextIndex()
	if idx < 0 {
		m.markDirty()
		return nil
	}
	return m.playQueueIndex(idx)
}

// maybeCrossfade starts the next queue entry shortly before the current track
// ends. The engine overlaps both streams, while the UI switches to the new
// track as soon as the fade begins.
func (m *Model) maybeCrossfade() tea.Cmd {
	if !m.loaded || m.paused || m.ended || m.current == nil || m.duration <= 0 || m.crossfadeTried {
		return nil
	}
	if m.duration <= 2*crossfadeDuration || m.position < 500*time.Millisecond {
		return nil
	}
	remaining := m.duration - m.position
	if remaining <= 0 || remaining > crossfadeDuration {
		return nil
	}
	m.crossfadeTried = true

	idx := -1
	if m.repeat == "one" {
		idx = m.queueIndex
	} else {
		idx = m.nextIndex()
	}
	if idx < 0 || idx >= len(m.queue) {
		return nil
	}
	next, ok := m.findTrack(m.queue[idx])
	if !ok {
		return nil
	}
	fade := crossfadeDuration
	if remaining < fade {
		fade = remaining
	}
	if fade < 500*time.Millisecond {
		return nil
	}
	if next.Duration > 0 && next.Duration < 2*fade {
		// Avoid starting a track that would be gone before it faded in.
		return nil
	}
	if err := m.engine.CrossfadeTo(next.Path, next.Duration, 0, fade); err != nil {
		return nil
	}

	m.queueIndex = idx
	if m.shuffle && !containsInt(m.shuffleHistory, idx) {
		m.shuffleHistory = append(m.shuffleHistory, idx)
	}
	cp := next
	m.current = &cp
	m.loaded = true
	m.paused = false
	m.ended = false
	m.position = 0
	m.duration = next.Duration
	m.engineGen = m.engine.Gen()
	m.crossfadeTried = false
	m.markDirty()
	m.syncPresence()
	return m.beginArt(next.Path)
}

// seekTo jumps to an absolute position in the current track.
func (m *Model) seekTo(target time.Duration) {
	if m.current == nil {
		return
	}
	if target < 0 {
		target = 0
	}
	if m.duration > 0 && target > m.duration {
		target = m.duration
	}
	if !m.loaded || m.ended {
		m.position = target
		m.ended = false
		m.paused = true
		m.crossfadeTried = false
		m.markDirty()
		m.syncPresence()
		return
	}
	if err := m.engine.SeekTo(target); err != nil {
		m.setToast("Seek failed: "+err.Error(), toastError, 3*time.Second)
		return
	}
	m.position = m.engine.Position()
	m.paused = m.engine.Paused()
	m.ended = false
	m.engineGen = m.engine.Gen()
	m.crossfadeTried = false
	m.markDirty()
	m.syncPresence()
}

func (m *Model) seek(delta time.Duration) {
	if m.current == nil {
		return
	}
	if !m.loaded || m.ended {
		target := m.position + delta
		if target < 0 {
			target = 0
		}
		if m.duration > 0 && target > m.duration {
			target = m.duration
		}
		m.position = target
		m.ended = false
		m.paused = true
		m.crossfadeTried = false
		m.markDirty()
		m.syncPresence()
		return
	}
	if err := m.engine.SeekRelative(delta); err != nil {
		m.setToast("Seek failed: "+err.Error(), toastError, 3*time.Second)
		return
	}
	m.position = m.engine.Position()
	m.paused = m.engine.Paused()
	m.ended = false
	m.engineGen = m.engine.Gen()
	m.crossfadeTried = false
	m.markDirty()
	m.syncPresence()
}

func (m *Model) setVolume(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	m.volume = v
	m.engine.SetVolume(float64(v) / 100)
	m.markDirty()
}

func (m *Model) cycleRepeat() {
	switch m.repeat {
	case "off":
		m.repeat = "all"
	case "all":
		m.repeat = "one"
	default:
		m.repeat = "off"
	}
	m.markDirty()
	m.setToast("Repeat: "+m.repeat, toastInfo, 1200*time.Millisecond)
}

func (m *Model) toggleShuffle() {
	m.shuffle = !m.shuffle
	m.shuffleHistory = nil
	if m.shuffle && m.queueIndex >= 0 {
		m.shuffleHistory = []int{m.queueIndex}
	}
	m.markDirty()
	state := "off"
	if m.shuffle {
		state = "on"
	}
	m.setToast("Shuffle: "+state, toastInfo, 1200*time.Millisecond)
}

func (m *Model) moveCursor(delta int) {
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.clampListOffset()
}

func (m *Model) removeFromPlaylist() {
	if m.view == "all" {
		m.setToast("Open a playlist to remove tracks from it", toastInfo, 2*time.Second)
		return
	}
	paths := m.selectionPaths()
	if len(paths) == 0 {
		t, ok := m.selectedTrack()
		if !ok {
			return
		}
		paths = []string{t.Path}
	}
	removed, err := m.playlists.RemoveMany(m.view, paths)
	if err != nil {
		m.setToast("Could not update playlist: "+err.Error(), toastError, 4*time.Second)
		return
	}
	if removed == 0 {
		return
	}
	noun := "track"
	if removed > 1 {
		noun = "tracks"
	}
	m.rebuildVisible("")
	m.setToast(fmt.Sprintf("Removed %d %s from %s", removed, noun, m.view), toastInfo, 2*time.Second)
	m.markDirty()
}

// openRemoveFromPlaylist asks before removing tracks from the current
// playlist. The audio files stay on disk; the confirmation also offers a disk
// delete for users who really want to remove the files.
func (m *Model) openRemoveFromPlaylist() {
	if m.view == "all" {
		m.openDeleteTrack()
		return
	}
	tracks := m.selectionTracks()
	if len(tracks) == 0 {
		if t, ok := m.selectedTrack(); ok {
			tracks = []model.Track{t}
		}
	}
	if len(tracks) == 0 {
		m.setToast("Select a track first", toastInfo, 2*time.Second)
		return
	}
	m.deleteTracks = tracks
	m.modal = modalConfirmRemoveFromPlaylist
}

func (m *Model) removePlaylistTracksNow() tea.Cmd {
	tracks := append([]model.Track(nil), m.deleteTracks...)
	if len(tracks) == 0 || m.view == "all" {
		m.closeModal()
		return nil
	}
	paths := make([]string, 0, len(tracks))
	for _, t := range tracks {
		paths = append(paths, t.Path)
	}
	removed, err := m.playlists.RemoveMany(m.view, paths)
	if err != nil {
		m.setToast("Could not update playlist: "+err.Error(), toastError, 4*time.Second)
		m.closeModal()
		return nil
	}
	m.closeModal()
	if removed == 0 {
		return nil
	}
	m.rebuildVisible("")
	noun := "track"
	if removed > 1 {
		noun = "tracks"
	}
	m.setToast(fmt.Sprintf("Removed %d %s from %s · files kept", removed, noun, m.view), toastSuccess, 3*time.Second)
	m.markDirty()
	return nil
}

func (m *Model) deleteTrackNow() tea.Cmd {
	tracks := append([]model.Track(nil), m.deleteTracks...)
	if len(tracks) == 0 {
		m.closeModal()
		return nil
	}
	deleting := make(map[string]bool, len(tracks))
	deletingCurrent := false
	for _, t := range tracks {
		key := trackKey(t.Path)
		deleting[key] = true
		if m.current != nil && trackKey(m.current.Path) == key {
			deletingCurrent = true
		}
	}
	if deletingCurrent {
		// Windows keeps an exclusive handle on the file while it is decoded,
		// so stop playback before trying to remove it.
		m.engine.Stop()
		m.loaded = false
		m.paused = true
		m.ended = false
		m.engineGen = m.engine.Gen()
	}

	deleted := 0
	var firstErr error
	playlistErr := false
	for _, t := range tracks {
		if err := m.lib.DeleteFile(t); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deleted++
		if _, err := m.playlists.RemovePathEverywhere(t.Path); err != nil {
			playlistErr = true
		}
	}

	filtered := m.tracks[:0]
	for _, tr := range m.tracks {
		if !deleting[trackKey(tr.Path)] {
			filtered = append(filtered, tr)
		}
	}
	m.tracks = filtered

	if deletingCurrent {
		m.current = nil
		m.loaded = false
		m.paused = false
		m.ended = false
		m.position = 0
		m.duration = 0
		m.artImage = nil
		m.artFor = ""
		m.artErr = nil
		m.spectrum = nil
		m.queue = nil
		m.queueIndex = 0
		m.coverMode = false
	} else if len(m.queue) > 0 {
		keptQueue := m.queue[:0]
		removedBefore := 0
		for i, p := range m.queue {
			if deleting[trackKey(p)] {
				if i < m.queueIndex {
					removedBefore++
				}
				continue
			}
			keptQueue = append(keptQueue, p)
		}
		m.queue = keptQueue
		m.queueIndex -= removedBefore
		if m.queueIndex < 0 {
			m.queueIndex = 0
		}
		if len(m.queue) == 0 {
			m.queueIndex = 0
		}
		m.shuffleHistory = nil
		if m.shuffle && len(m.queue) > 0 {
			m.shuffleHistory = []int{m.queueIndex}
		}
	}

	m.closeModal()
	m.rebuildVisible("")
	switch {
	case deleted == 0 && firstErr != nil:
		m.setToast("Delete failed: "+firstErr.Error(), toastError, 5*time.Second)
	case firstErr != nil:
		m.setToast(fmt.Sprintf("Deleted %d of %d tracks · %v", deleted, len(tracks), firstErr), toastWarning, 5*time.Second)
	case len(tracks) == 1:
		m.setToast("Deleted "+tracks[0].DisplayTitle(), toastSuccess, 3*time.Second)
	default:
		m.setToast(fmt.Sprintf("Deleted %d tracks", deleted), toastSuccess, 3*time.Second)
	}
	if playlistErr {
		m.setToast("Tracks deleted, but playlists could not be saved", toastWarning, 4*time.Second)
	}
	m.syncPresence()
	m.markDirty()
	return nil
}

func (m *Model) deletePlaylistNow() tea.Cmd {
	if m.deletePlaylist == "" {
		m.closeModal()
		return nil
	}
	name := m.deletePlaylist
	ok, err := m.playlists.Delete(name)
	if err != nil {
		m.setToast("Could not delete playlist: "+err.Error(), toastError, 4*time.Second)
		m.closeModal()
		return nil
	}
	if ok {
		if m.view == name {
			m.view = "all"
		}
		m.syncSidebarCursor()
		m.rebuildVisible("")
		m.setToast("Playlist \""+name+"\" deleted", toastSuccess, 3*time.Second)
		m.markDirty()
	}
	m.closeModal()
	return nil
}

func (m *Model) refreshJobs() {
	m.jobs = m.mgr.Jobs()
	if len(m.jobs) == 0 {
		m.downloadsCur = 0
		return
	}
	if m.downloadsCur >= len(m.jobs) {
		m.downloadsCur = len(m.jobs) - 1
	}
	if m.downloadsCur < 0 {
		m.downloadsCur = 0
	}
}

func (m *Model) selectedJob() (download.Job, bool) {
	m.refreshJobs()
	if m.downloadsCur >= 0 && m.downloadsCur < len(m.jobs) {
		return m.jobs[m.downloadsCur], true
	}
	return download.Job{}, false
}

func (m *Model) resizeInputs() {
	_, _, _, _, _, centerW := m.layout()
	modalW := modalWidth(m.width)
	m.linkInput.Width = maxInt(20, modalW-8)
	m.nameInput.Width = maxInt(20, modalW-8)
	m.searchInput.Width = maxInt(16, centerW-16)
}

func (m *Model) openAddLink() tea.Cmd {
	m.modal = modalAddLink
	m.modalErr = ""
	m.linkInput.SetValue("")
	m.resizeInputs()
	return m.linkInput.Focus()
}

func (m *Model) openNewPlaylist() tea.Cmd {
	m.modal = modalNewPlaylist
	m.modalErr = ""
	m.nameInput.SetValue("")
	m.resizeInputs()
	return m.nameInput.Focus()
}

func (m *Model) openAddToPlaylist() tea.Cmd {
	tracks := m.selectionTracks()
	if len(tracks) == 0 {
		if t, ok := m.selectedTrack(); ok {
			tracks = []model.Track{t}
		} else if m.current != nil {
			tracks = []model.Track{*m.current}
		}
	}
	if len(tracks) == 0 {
		m.setToast("Select a track first", toastInfo, 2*time.Second)
		return nil
	}
	items := m.playlists.List()
	if len(items) == 0 {
		m.setToast("No playlists yet — press P to create one", toastInfo, 3*time.Second)
		return nil
	}
	m.pendingTrack = tracks[0]
	m.pendingPaths = m.pendingPaths[:0]
	for _, t := range tracks {
		m.pendingPaths = append(m.pendingPaths, t.Path)
	}
	m.hasPending = true
	m.modalCursor = 0
	m.modalErr = ""
	m.modal = modalAddToPlaylist
	return nil
}

func (m *Model) openDeleteTrack() {
	tracks := m.selectionTracks()
	if len(tracks) == 0 {
		if t, ok := m.selectedTrack(); ok {
			tracks = []model.Track{t}
		}
	}
	if len(tracks) == 0 {
		return
	}
	m.deleteTracks = tracks
	m.modal = modalConfirmDeleteTrack
}

func (m *Model) openDeletePlaylist() {
	if m.sidebarCursor <= 0 {
		m.setToast("Select a playlist first", toastInfo, 2*time.Second)
		return
	}
	names := m.playlists.Names()
	if m.sidebarCursor-1 >= len(names) {
		return
	}
	m.deletePlaylist = names[m.sidebarCursor-1]
	m.modal = modalConfirmDeletePlaylist
}

func (m *Model) openDownloads() {
	m.modal = modalDownloads
	m.refreshJobs()
	if len(m.jobs) > 0 {
		m.downloadsCur = len(m.jobs) - 1
	}
	m.downloadsOff = 0
}

func (m *Model) openHelp() {
	m.modal = modalHelp
	m.modalCursor = 0
}

func (m *Model) closeModal() {
	switch m.modal {
	case modalAddLink:
		m.linkInput.Blur()
	case modalNewPlaylist:
		m.nameInput.Blur()
	}
	m.modal = modalNone
	m.modalErr = ""
	m.hasPending = false
	m.pendingPaths = nil
	m.deleteTracks = nil
	m.deletePlaylist = ""
}

func (m *Model) submitLink() tea.Cmd {
	raw := strings.TrimSpace(m.linkInput.Value())
	if raw == "" {
		m.modalErr = "Paste a Spotify, SoundCloud or YouTube link first."
		return nil
	}
	job, err := m.mgr.Enqueue(raw)
	if err != nil {
		m.modalErr = err.Error()
		return nil
	}
	m.jobSeen[job.ID] = job.Status
	m.linkInput.Blur()
	m.modal = modalDownloads
	m.modalErr = ""
	m.refreshJobs()
	m.downloadsCur = len(m.jobs) - 1
	return nil
}

func (m *Model) submitPlaylistName() tea.Cmd {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.modalErr = "Enter a playlist name."
		return nil
	}
	if _, err := m.playlists.Create(name); err != nil {
		m.modalErr = err.Error()
		return nil
	}
	m.nameInput.Blur()
	m.closeModal()
	m.setToast("Playlist \""+name+"\" created", toastSuccess, 3*time.Second)
	m.markDirty()
	return nil
}

func (m *Model) addPendingToPlaylist() tea.Cmd {
	items := m.playlists.List()
	if m.modalCursor < 0 || m.modalCursor >= len(items) {
		return nil
	}
	name := items[m.modalCursor].Name
	paths := append([]string(nil), m.pendingPaths...)
	if len(paths) == 0 {
		paths = []string{m.pendingTrack.Path}
	}
	added, skipped, err := m.playlists.AddMany(name, paths)
	if err != nil {
		m.modalErr = err.Error()
		return nil
	}
	m.closeModal()
	if added == 0 {
		if len(paths) > 1 {
			m.setToast(fmt.Sprintf("All %d tracks are already in %s", len(paths), name), toastInfo, 3*time.Second)
		} else {
			m.setToast("Already in "+name, toastInfo, 2*time.Second)
		}
		return nil
	}
	if m.view != "all" && strings.EqualFold(m.view, name) {
		m.rebuildVisible("")
	}
	msg := "Added to " + name
	if added > 1 {
		msg = fmt.Sprintf("Added %d tracks to %s", added, name)
	}
	if skipped > 0 {
		msg += fmt.Sprintf(" · %d already there", skipped)
	}
	m.setToast(msg, toastSuccess, 3*time.Second)
	m.markDirty()
	return nil
}

func (m *Model) startSearch() tea.Cmd {
	m.searching = true
	m.focus = focusTracks
	m.resizeInputs()
	m.searchInput.SetValue(m.search)
	return m.searchInput.Focus()
}

func (m *Model) updateSearch() {
	m.search = m.searchInput.Value()
	m.rebuildVisible("")
}

func (m *Model) exitSearch(clear bool) {
	m.searching = false
	m.searchInput.Blur()
	if clear {
		m.search = ""
		m.searchInput.SetValue("")
		m.rebuildVisible("")
	}
	m.focus = focusTracks
}

func (m *Model) handleLibraryLoaded(msg libraryLoadedMsg) tea.Cmd {
	m.loading = false
	if msg.background {
		m.refreshing = false
	}
	if msg.err != nil {
		if msg.background {
			m.setToast("Library refresh failed: "+msg.err.Error(), toastError, 5*time.Second)
		} else {
			m.loadErr = msg.err
		}
		return nil
	}
	m.loadErr = nil

	selectedPath := ""
	if t, ok := m.selectedTrack(); ok {
		selectedPath = t.Path
	}
	currentPath := ""
	if m.current != nil {
		currentPath = m.current.Path
	}

	m.tracks = msg.tracks

	if currentPath != "" {
		if t, ok := m.findTrack(currentPath); ok {
			cp := t
			m.current = &cp
			if m.duration <= 0 {
				m.duration = t.Duration
			}
		} else {
			m.engine.Stop()
			m.current = nil
			m.loaded = false
			m.paused = false
			m.ended = false
			m.position = 0
			m.duration = 0
			m.queue = nil
			m.queueIndex = 0
		}
	}

	if !m.restored {
		m.restoreSession()
	} else {
		m.rebuildVisible(selectedPath)
	}
	m.clampListOffset()
	m.syncPresence()

	var cmds []tea.Cmd
	if m.current != nil && m.artFor == "" && !m.artLoading {
		cmds = append(cmds, m.beginArt(m.current.Path))
	}
	if msg.background && m.needRefresh {
		cmds = append(cmds, m.startRefresh())
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleArtLoaded(msg artLoadedMsg) {
	if m.current == nil || trackKey(m.current.Path) != trackKey(msg.path) {
		return
	}
	if trackKey(m.artFor) != trackKey(msg.path) {
		return
	}
	m.artLoading = false
	m.artImage = msg.img
	m.artErr = msg.err
}

func (m *Model) pollDownloads() tea.Cmd {
	jobs := m.mgr.Jobs()
	newSeen := make(map[int]download.Status, len(jobs))
	var cmds []tea.Cmd
	for _, j := range jobs {
		newSeen[j.ID] = j.Status
		prev, seen := m.jobSeen[j.ID]
		if !seen {
			continue
		}
		if prev == j.Status {
			continue
		}
		switch j.Status {
		case download.StatusDone:
			title := j.Title
			if title == "" {
				title = j.URL
			}
			if j.Warning != "" {
				m.setToast("Downloaded with warnings: "+title, toastWarning, 4*time.Second)
			} else {
				m.setToast("Downloaded: "+title, toastSuccess, 4*time.Second)
			}
			cmds = append(cmds, m.startRefresh())
		case download.StatusFailed:
			msg := j.Err
			if msg == "" {
				msg = "unknown error"
			}
			m.setToast("Download failed: "+truncateText(msg, 120), toastError, 6*time.Second)
		case download.StatusCanceled:
			m.setToast("Download canceled", toastInfo, 2*time.Second)
		}
	}
	m.jobSeen = newSeen
	m.jobs = jobs
	if len(m.jobs) == 0 {
		m.downloadsCur = 0
	} else if m.downloadsCur >= len(m.jobs) {
		m.downloadsCur = len(m.jobs) - 1
	}
	if m.downloadsCur < 0 {
		m.downloadsCur = 0
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleTick() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}
	m.spinner++

	if m.toast != "" && time.Now().After(m.toastUntil) {
		m.toast = ""
	}

	if m.loaded && m.current != nil {
		if m.engine.Active() {
			m.position = m.engine.Position()
			m.paused = m.engine.Paused()
			if m.duration <= 0 {
				if d := m.engine.Duration(); d > 0 {
					m.duration = d
					m.learnDuration(d)
				}
			}
		}
	}
	if cmd := m.maybeCrossfade(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	bars := m.spectrumBarCount()
	if bars > 0 {
		raw := m.engine.Spectrum(bars)
		if len(m.spectrum) != bars {
			next := make([]float64, bars)
			copy(next, m.spectrum)
			m.spectrum = next
		}
		playing := m.loaded && !m.paused && !m.ended
		for i := 0; i < bars; i++ {
			target := 0.0
			if playing && i < len(raw) {
				target = raw[i]
			}
			if target > m.spectrum[i] {
				m.spectrum[i] = m.spectrum[i]*0.3 + target*0.7
			} else {
				m.spectrum[i] *= 0.78
				if m.spectrum[i] < 0.002 {
					m.spectrum[i] = 0
				}
			}
		}
	} else if len(m.spectrum) > 0 {
		m.spectrum = nil
	}

	for {
		select {
		case ev := <-m.engine.Events():
			if ev.Gen != m.engineGen || m.current == nil {
				continue
			}
			if ev.Path != "" && trackKey(ev.Path) != trackKey(m.current.Path) {
				continue
			}
			if ev.Kind == audio.EventDone {
				if cmd := m.onTrackEnded(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		default:
			goto drained
		}
	}
drained:

	if cmd := m.pollDownloads(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	if time.Since(m.lastSave) > 10*time.Second && (m.current != nil || m.saveDirty) {
		m.saveSession()
		m.lastSave = time.Now()
	}

	m.clampListOffset()
	m.clampSidebarOffset()
	return tea.Batch(cmds...)
}

func containsInt(values []int, target int) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
