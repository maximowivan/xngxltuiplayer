package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// handleMouse maps terminal clicks and wheel events to the visible widgets.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.loading || m.loadErr != nil {
		return m, nil
	}
	if m.modal != modalNone {
		return m.handleModalMouse(msg)
	}
	w, h := m.dims()
	if w < 54 || h < 16 {
		return m, nil
	}

	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		return m.handleWheel(msg)
	}
	if msg.Action == tea.MouseActionMotion {
		return m.handleMouseMotion(msg)
	}
	if msg.Action == tea.MouseActionRelease {
		return m.handleMouseRelease(msg)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	m.resetDrag()
	if m.coverMode {
		return m.handleCoverClick(msg)
	}

	_, _, contentH, sidebarW, rightW, centerW := m.layout()
	contentTop := 1
	playerTop := contentTop + contentH
	playerInnerTop := playerTop + 1
	if msg.Y == playerInnerTop {
		innerW := maxInt(1, w-4)
		x := msg.X - playerContentX
		layout := m.playerTitleLayout(innerW)
		switch {
		case x >= 0 && x < ansi.StringWidth(iconPause) && m.current != nil:
			return m, m.togglePlay()
		case x >= layout.shuffleStart && x < layout.shuffleEnd:
			m.toggleShuffle()
			return m, nil
		case x >= layout.prevStart && x < layout.prevEnd:
			return m, m.prevTrack()
		case x >= layout.playStart-1 && x < layout.playEnd+1:
			return m, m.togglePlay()
		case x >= layout.nextStart && x < layout.nextEnd:
			return m, m.nextTrack()
		case x >= layout.repeatStart && x < layout.repeatEnd:
			m.cycleRepeat()
			return m, nil
		case layout.volBarStart >= 0 && x >= layout.volBarStart && x < layout.volBarEnd:
			m.volDragging = true
			m.setVolumeFromX(msg.X, w)
			return m, nil
		case layout.volMinus >= 0 && x == layout.volMinus:
			m.setVolume(m.volume - 5)
			return m, nil
		case layout.volPlus >= 0 && x == layout.volPlus:
			m.setVolume(m.volume + 5)
			return m, nil
		}
		return m, nil
	}
	if msg.Y == playerInnerTop+2 {
		m.seekFromProgressBar(msg.X, w)
		return m, nil
	}
	if msg.Y < contentTop || msg.Y >= contentTop+contentH {
		return m, nil
	}
	innerTop := contentTop + 1
	centerX := 0
	if sidebarW > 0 {
		centerX = sidebarW + 1
	}
	rightX := centerX + centerW + 1

	switch {
	case sidebarW > 0 && msg.X < sidebarW:
		m.clickSidebar(msg.Y - innerTop)
		return m, nil

	case msg.X >= centerX && msg.X < centerX+centerW:
		return m.clickTracks(msg.Y-innerTop, msg.X, centerX, centerW, msg.Ctrl)

	case rightW > 0 && msg.X >= rightX && msg.X < rightX+rightW:
		m.clickNowPlaying(msg.X-rightX, msg.Y-innerTop, rightW, contentH)
		return m, nil
	}
	return m, nil
}

func (m *Model) handleWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.coverMode {
		return m, nil
	}
	up := msg.Button == tea.MouseButtonWheelUp
	step := 3
	sign := 1
	if up {
		step = -3
		sign = -1
	}

	_, _, contentH, sidebarW, _, centerW := m.layout()
	playerTop := 1 + contentH
	if msg.Y == playerTop+1 {
		// The player title line doubles as a volume slider.
		if up {
			m.setVolume(m.volume + 5)
		} else {
			m.setVolume(m.volume - 5)
		}
		return m, nil
	}
	if msg.Y < 1 || msg.Y >= 1+contentH {
		return m, nil
	}
	centerX := 0
	if sidebarW > 0 {
		centerX = sidebarW + 1
	}

	if sidebarW > 0 && msg.X < sidebarW {
		m.focus = focusSidebar
		m.moveSidebar(sign)
		return m, nil
	}
	if msg.X >= centerX && msg.X < centerX+centerW {
		m.focus = focusTracks
		m.moveCursor(step)
	}
	return m, nil
}

func (m *Model) clickSidebar(innerLine int) {
	innerH := m.contentH() - 2
	if innerLine < 0 || innerLine >= innerH {
		return
	}
	m.focus = focusSidebar
	switch {
	case innerLine == 0:
		m.sidebarCursor = 0
		m.view = viewAll
		m.rebuildVisible("")
		m.markDirty()
	case innerLine == 1:
		m.sidebarCursor = 1
		m.view = viewFavorites
		m.rebuildVisible("")
		m.markDirty()
	case innerLine == 2:
		m.sidebarCursor = 2
		m.view = viewRecent
		m.rebuildVisible("")
		m.markDirty()
	case innerLine >= 5:
		idx := m.sidebarOffset + innerLine - 5
		names := m.playlists.Names()
		if idx >= 0 && idx < len(names) {
			m.sidebarCursor = idx + 3
			m.view = names[idx]
			m.rebuildVisible("")
			m.markDirty()
		}
	}
}

func (m *Model) clickTracks(innerLine, x, centerX, centerW int, ctrl bool) (tea.Model, tea.Cmd) {
	innerH := m.contentH() - 2
	if innerLine < 0 || innerLine >= innerH {
		return m, nil
	}
	if x < centerX+1 || x >= centerX+centerW-1 {
		return m, nil
	}

	searchOffset := 0
	if m.searching {
		searchOffset = 1
	}
	row := innerLine - searchOffset
	if row < 0 || row >= m.listRows() {
		return m, nil
	}
	idx := m.listOffset + row
	if idx < 0 || idx >= len(m.visible) {
		return m, nil
	}

	if m.searching {
		m.exitSearch(false)
	}
	m.focus = focusTracks
	path := m.visible[idx].Path

	if ctrl {
		m.cursor = idx
		m.clampListOffset()
		m.toggleSelected(path)
		m.lastClickValid = false
		return m, nil
	}
	if !m.isSelected(path) {
		m.clearSelection()
	}

	now := time.Now()
	doubleClick := m.lastClickValid &&
		m.lastClickRow == idx &&
		now.Sub(m.lastClickAt) <= 450*time.Millisecond

	m.cursor = idx
	m.clampListOffset()
	m.startDragCandidate(path, x, 2+innerLine)
	m.lastClickAt = now
	m.lastClickRow = idx
	m.lastClickValid = !doubleClick

	if doubleClick {
		m.lastClickValid = false
		return m, m.playSelected()
	}
	return m, nil
}

// clickNowPlaying opens the full-screen cover when the artwork is clicked.
func (m *Model) clickNowPlaying(innerX, innerY, width, contentH int) {
	innerW := width - 4
	innerH := contentH - 2
	artW, artH := m.rightArtSize(innerW, innerH)
	if artW <= 0 || artH <= 0 {
		return
	}
	pad := maxInt(0, (innerW-artW)/2)
	if innerY < 0 || innerY >= artH {
		return
	}
	if innerX < 2+pad || innerX >= 2+pad+artW {
		return
	}
	m.coverMode = true
}

func (m *Model) handleCoverClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	w, h := m.dims()
	artW, artH, artTop := m.coverArtSize(w, h)
	pad := maxInt(0, (w-artW)/2)
	if msg.Y >= artTop && msg.Y < artTop+artH {
		if msg.X >= pad && msg.X < pad+artW {
			m.coverMode = false
		}
		return m, nil
	}
	// The compact player is bottom-aligned: title, progress, hints, toast.
	if msg.Y == h-3 {
		m.seekFromProgressBar(msg.X, w)
	}
	return m, nil
}

func (m *Model) seekFromProgressBar(x, width int) {
	if m.current == nil || m.duration <= 0 {
		return
	}
	var start, barWidth int
	if m.coverMode {
		start, barWidth = m.coverProgressGeometry(width)
	} else {
		start, barWidth = m.playerProgressGeometry(width)
	}
	if barWidth <= 0 || x < start || x >= start+barWidth {
		return
	}
	frac := float64(x-start) / float64(barWidth)
	m.seekTo(time.Duration(float64(m.duration) * frac))
}

func (m *Model) setVolumeFromX(x, width int) {
	start, end := m.volumeBarBounds(width)
	span := end - start - 1
	if span < 1 {
		span = 1
	}
	m.setVolume((x - start) * 100 / span)
}

func (m *Model) handleMouseMotion(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	w, _ := m.dims()
	if m.volDragging {
		m.setVolumeFromX(msg.X, w)
		return m, nil
	}
	if m.coverMode || len(m.dragPaths) == 0 {
		return m, nil
	}
	if !m.dragActive {
		if absInt(msg.X-m.dragStartX)+absInt(msg.Y-m.dragStartY) < 3 {
			return m, nil
		}
		m.dragActive = true
	}
	m.dragTarget = m.playlistAt(msg.X, msg.Y)
	return m, nil
}

func (m *Model) handleMouseRelease(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.volDragging {
		m.volDragging = false
		return m, nil
	}
	if m.dragActive {
		return m, m.finishTrackDrag()
	}
	m.resetDrag()
	return m, nil
}

func (m *Model) contentH() int {
	_, _, contentH, _, _, _ := m.layout()
	return contentH
}

// handleModalMouse adds click and wheel support to the download list.
func (m *Model) handleModalMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal == modalHelp {
		if msg.Button == tea.MouseButtonWheelUp {
			m.modalCursor -= 3
			if m.modalCursor < 0 {
				m.modalCursor = 0
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.modalCursor += 3
		}
		return m, nil
	}
	if m.modal == modalAddToPlaylist {
		items := m.playlists.List()
		if len(items) == 0 {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.modalCursor--
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.modalCursor++
		} else if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			w, _ := m.dims()
			_, _, contentH, _, _, _ := m.layout()
			width := modalWidth(w)
			height := modalHeight(contentH, minInt(18, 8+len(items)))
			left := (w - width) / 2
			top := (contentH - height) / 2
			rows := maxInt(1, height-6)
			offset := 0
			if m.modalCursor >= rows {
				offset = m.modalCursor - rows + 1
			}
			row := msg.Y - (top + 4)
			if msg.X >= left && msg.X < left+width && row >= 0 && row < rows {
				idx := offset + row
				if idx >= 0 && idx < len(items) {
					m.modalCursor = idx
				}
			}
		}
		if m.modalCursor < 0 {
			m.modalCursor = 0
		}
		if m.modalCursor >= len(items) {
			m.modalCursor = len(items) - 1
		}
		return m, nil
	}
	if m.modal != modalDownloads {
		return m, nil
	}
	m.refreshJobs()
	_, _, contentH, _, _, _ := m.layout()
	w, _ := m.dims()
	width := minInt(86, maxInt(48, w-8))
	desired := 22
	if len(m.jobs) == 0 {
		desired = 8
	}
	height := modalHeight(contentH, desired)
	left := (w - width) / 2
	top := (contentH - height) / 2

	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.Button == tea.MouseButtonWheelUp {
			m.downloadsCur--
		} else {
			m.downloadsCur++
		}
		if m.downloadsCur < 0 {
			m.downloadsCur = 0
		}
		if m.downloadsCur >= len(m.jobs) {
			m.downloadsCur = maxInt(0, len(m.jobs)-1)
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if msg.X < left || msg.X >= left+width {
		return m, nil
	}
	rows := maxInt(1, height-2-5)
	offset := 0
	if m.downloadsCur >= rows {
		offset = m.downloadsCur - rows + 1
	}
	// Header (1 line inside), blank line, then one line per job.
	row := msg.Y - (1 + top + 3)
	if row < 0 || row >= rows {
		return m, nil
	}
	idx := offset + row
	if idx < 0 || idx >= len(m.jobs) {
		return m, nil
	}
	m.downloadsCur = idx
	return m, nil
}
