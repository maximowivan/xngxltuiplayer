package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/download"
)

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if _, _, _, sidebarW, _, _ := m.layout(); sidebarW == 0 {
			m.focus = focusTracks
		}
		m.resizeInputs()
		m.clampListOffset()
		m.clampSidebarOffset()
		return m, nil

	case tickMsg:
		return m, m.handleTick()

	case libraryLoadedMsg:
		return m, m.handleLibraryLoaded(msg)

	case artLoadedMsg:
		m.handleArtLoaded(msg)
		return m, nil

	case presenceStatusMsg:
		return m, m.handlePresenceStatus(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.saveSession()
		return m, tea.Quit
	}

	if m.modal != modalNone {
		return m.handleModalKey(msg)
	}
	if m.searching {
		return m.handleSearchKey(msg)
	}

	key = normalizeShortcut(key)
	if m.loadErr != nil {
		switch key {
		case "q":
			return m, m.quit()
		case "r":
			m.loadErr = nil
			m.loading = true
			return m, m.loadLibraryCmd(false)
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, m.quit()
	case "?":
		m.openHelp()
		return m, nil
	case "a":
		return m, m.openAddLink()
	case "P":
		return m, m.openNewPlaylist()
	case "I":
		return m, m.openImportM3U()
	case "E":
		return m, m.openExportM3U()
	case "/":
		return m, m.startSearch()
	case "tab":
		if w, _, _, sidebarW, _, _ := m.layout(); sidebarW > 0 && w >= 72 {
			if m.focus == focusTracks {
				m.focus = focusSidebar
			} else {
				m.focus = focusTracks
			}
		}
		return m, nil
	case "enter":
		if m.focus == focusSidebar {
			m.openViewAtCursor()
			return m, nil
		}
		return m, m.playSelected()
	case " ":
		return m, m.togglePlay()
	case "up", "k":
		m.moveFocus(-1)
		return m, nil
	case "down", "j":
		m.moveFocus(1)
		return m, nil
	case "pgup":
		step := m.listRows()
		if m.focus == focusSidebar {
			step = m.sidebarRows()
		}
		m.moveFocus(-step)
		return m, nil
	case "pgdown":
		step := m.listRows()
		if m.focus == focusSidebar {
			step = m.sidebarRows()
		}
		m.moveFocus(step)
		return m, nil
	case "home", "g":
		m.moveFocusToStart()
		return m, nil
	case "end", "G":
		m.moveFocusToEnd()
		return m, nil
	case "n":
		return m, m.nextTrack()
	case "p":
		return m, m.prevTrack()
	case "left", "H":
		step := 5 * time.Second
		if key == "H" {
			step = 30 * time.Second
		}
		m.seek(-step)
		return m, nil
	case "right", "L":
		step := 5 * time.Second
		if key == "L" {
			step = 30 * time.Second
		}
		m.seek(step)
		return m, nil
	case "-", "_":
		m.setVolume(m.volume - 5)
		return m, nil
	case "=", "+":
		m.setVolume(m.volume + 5)
		return m, nil
	case "ctrl+a":
		m.focus = focusTracks
		m.selectAllVisible()
		return m, nil
	case "ctrl+space", "ctrl+@":
		if t, ok := m.selectedTrack(); ok {
			m.toggleSelected(t.Path)
		}
		return m, nil
	case "s":
		m.toggleShuffle()
		return m, nil
	case "r":
		m.cycleRepeat()
		return m, nil
	case "z":
		m.cycleSleepTimer()
		return m, nil
	case "o":
		m.cycleSort()
		return m, nil
	case "t":
		m.cycleTheme(1)
		return m, nil
	case "T":
		m.cycleTheme(-1)
		return m, nil
	case "c":
		m.coverMode = !m.coverMode
		return m, nil
	case "d":
		m.openDownloads()
		return m, nil
	case "x":
		return m, m.openAddToPlaylist()
	case "X":
		m.removeFromPlaylist()
		return m, nil
	case "f":
		m.toggleFavorite()
		return m, nil
	case "D":
		if m.focus == focusSidebar && m.sidebarCursor > 2 {
			m.openDeletePlaylist()
		} else if m.view != viewAll && !isSpecialView(m.view) {
			m.openRemoveFromPlaylist()
		} else {
			m.openDeleteTrack()
		}
		return m, nil
	case "delete":
		if m.focus == focusSidebar && m.sidebarCursor > 2 {
			m.openDeletePlaylist()
		} else if m.view != viewAll && !isSpecialView(m.view) {
			m.openRemoveFromPlaylist()
		} else {
			m.openDeleteTrack()
		}
		return m, nil
	case "u":
		if cmd := m.startRefresh(); cmd != nil {
			m.setToast("Refreshing library…", toastInfo, 2*time.Second)
			return m, cmd
		}
		return m, nil
	case "esc":
		if m.coverMode {
			m.coverMode = false
			return m, nil
		}
		if m.selectionCount() > 0 {
			m.clearSelection()
			return m, nil
		}
		if m.search != "" {
			m.search = ""
			m.searchInput.SetValue("")
			m.rebuildVisible("")
			m.setToast("Search cleared", toastInfo, 1200*time.Millisecond)
		} else if m.toast != "" {
			m.toast = ""
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) quit() tea.Cmd {
	m.saveSession()
	return tea.Quit
}

func (m *Model) moveFocus(delta int) {
	if m.focus == focusSidebar {
		m.moveSidebar(delta)
		return
	}
	m.moveCursor(delta)
}

func (m *Model) moveFocusToStart() {
	if m.focus == focusSidebar {
		m.sidebarCursor = 0
		m.clampSidebarOffset()
		return
	}
	m.cursor = 0
	m.clampListOffset()
}

func (m *Model) moveFocusToEnd() {
	if m.focus == focusSidebar {
		m.sidebarCursor = len(m.playlists.Names())
		m.clampSidebarOffset()
		return
	}
	if len(m.visible) > 0 {
		m.cursor = len(m.visible) - 1
	}
	m.clampListOffset()
}

func (m *Model) moveSidebar(delta int) {
	count := len(m.playlists.Names())
	m.sidebarCursor += delta
	if m.sidebarCursor < 0 {
		m.sidebarCursor = 0
	}
	if m.sidebarCursor > count+2 {
		m.sidebarCursor = count + 2
	}
	m.clampSidebarOffset()
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.exitSearch(true)
		return m, nil
	case "enter":
		m.exitSearch(false)
		return m, m.playSelected()
	case "up":
		m.moveCursor(-1)
		return m, nil
	case "down":
		m.moveCursor(1)
		return m, nil
	case "pgup":
		m.moveCursor(-m.listRows())
		return m, nil
	case "pgdown":
		m.moveCursor(m.listRows())
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.updateSearch()
	return m, cmd
}

func (m *Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := normalizeShortcut(msg.String())
	switch m.modal {
	case modalAddLink, modalNewPlaylist:
		switch key {
		case "esc":
			m.closeModal()
			return m, nil
		case "enter":
			if m.modal == modalAddLink {
				return m, m.submitLink()
			}
			return m, m.submitPlaylistName()
		}
		var cmd tea.Cmd
		if m.modal == modalAddLink {
			m.linkInput, cmd = m.linkInput.Update(msg)
		} else {
			m.nameInput, cmd = m.nameInput.Update(msg)
		}
		return m, cmd

	case modalImportM3U, modalExportM3U:
		switch key {
		case "esc":
			m.closeModal()
			return m, nil
		case "enter":
			if m.modal == modalImportM3U {
				return m, m.submitImportM3U()
			}
			return m, m.submitExportM3U()
		}
		var cmd tea.Cmd
		m.m3uInput, cmd = m.m3uInput.Update(msg)
		return m, cmd

	case modalAddToPlaylist:
		items := m.playlists.List()
		switch key {
		case "esc":
			m.closeModal()
		case "up", "k":
			m.modalCursor--
			if m.modalCursor < 0 {
				m.modalCursor = 0
			}
		case "down", "j":
			m.modalCursor++
			if m.modalCursor >= len(items) {
				m.modalCursor = maxInt(0, len(items)-1)
			}
		case "enter":
			return m, m.addPendingToPlaylist()
		}
		return m, nil

	case modalConfirmRemoveFromPlaylist:
		switch key {
		case "y", "enter":
			return m, m.removePlaylistTracksNow()
		case "d":
			// Keep deleteTracks intact and switch to the destructive dialog.
			m.modal = modalConfirmDeleteTrack
		case "n", "esc", "q":
			m.closeModal()
		}
		return m, nil

	case modalConfirmDeleteTrack:
		switch key {
		case "y", "enter":
			return m, m.deleteTrackNow()
		case "n", "esc", "q":
			m.closeModal()
		}
		return m, nil

	case modalConfirmDeletePlaylist:
		switch key {
		case "y", "enter":
			return m, m.deletePlaylistNow()
		case "n", "esc", "q":
			m.closeModal()
		}
		return m, nil

	case modalHelp:
		switch key {
		case "esc", "?", "enter", "q":
			m.closeModal()
		case "up", "k":
			if m.modalCursor > 0 {
				m.modalCursor--
			}
		case "down", "j":
			m.modalCursor++
		case "pgup":
			m.modalCursor -= 8
			if m.modalCursor < 0 {
				m.modalCursor = 0
			}
		case "pgdown":
			m.modalCursor += 8
		}
		return m, nil

	case modalDownloads:
		switch key {
		case "esc", "d", "q":
			m.closeModal()
			return m, nil
		case "up", "k":
			m.downloadsCur--
			if m.downloadsCur < 0 {
				m.downloadsCur = 0
			}
			return m, nil
		case "down", "j":
			m.downloadsCur++
			if m.downloadsCur >= len(m.jobs) {
				m.downloadsCur = maxInt(0, len(m.jobs)-1)
			}
			return m, nil
		case "home", "g":
			m.downloadsCur = 0
			return m, nil
		case "end", "G":
			m.downloadsCur = maxInt(0, len(m.jobs)-1)
			return m, nil
		case "c":
			if job, ok := m.selectedJob(); ok && !job.Status.Terminal() {
				m.mgr.Cancel(job.ID)
				m.setToast("Canceling download…", toastInfo, 2*time.Second)
			} else if ok {
				m.setToast("This download has already finished", toastInfo, 2*time.Second)
			}
			return m, nil
		case "C":
			if n := m.mgr.ClearFinished(); n > 0 {
				m.setToast("Cleared finished downloads", toastInfo, 2*time.Second)
				m.refreshJobs()
			} else {
				m.setToast("No finished downloads to clear", toastInfo, 2*time.Second)
			}
			return m, nil
		case "a":
			return m, m.openAddLink()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) downloadStatusIcon(status download.Status) string {
	switch status {
	case download.StatusDone:
		return "✓"
	case download.StatusFailed:
		return "✕"
	case download.StatusCanceled:
		return "–"
	case download.StatusQueued:
		return "·"
	default:
		return "↓"
	}
}

func (m *Model) jobLabel(job download.Job) string {
	title := strings.TrimSpace(job.Title)
	if title == "" {
		title = job.URL
	}
	if job.Artist != "" {
		title = job.Artist + " — " + title
	}
	return title
}
