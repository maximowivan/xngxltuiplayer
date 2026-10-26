package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *Model) startDragCandidate(path string, x, y int) {
	paths := []string{path}
	if m.isSelected(path) && m.selectionCount() > 0 {
		if selected := m.selectionPaths(); len(selected) > 0 {
			paths = selected
		}
	}
	m.dragPaths = paths
	m.dragSource = m.view
	m.dragStartX = x
	m.dragStartY = y
	m.dragActive = false
	m.dragTarget = ""
}

func (m *Model) resetDrag() {
	m.dragPaths = nil
	m.dragSource = ""
	m.dragActive = false
	m.dragTarget = ""
	m.volDragging = false
}

// playlistAt returns the playlist under the pointer while dragging.
func (m *Model) playlistAt(x, y int) string {
	_, _, contentH, sidebarW, _, _ := m.layout()
	if sidebarW <= 0 || x < 0 || x >= sidebarW {
		return ""
	}
	innerLine := y - 2
	if innerLine < 3 || innerLine >= contentH-2 {
		return ""
	}
	idx := m.sidebarOffset + innerLine - 3
	names := m.playlists.Names()
	if idx < 0 || idx >= len(names) {
		return ""
	}
	return names[idx]
}

// finishTrackDrag applies a drop on a playlist. Dragging from All tracks
// copies; dragging between playlists moves the selected paths.
func (m *Model) finishTrackDrag() tea.Cmd {
	source := m.dragSource
	paths := append([]string(nil), m.dragPaths...)
	target := m.dragTarget
	active := m.dragActive
	m.resetDrag()
	if !active || target == "" || len(paths) == 0 {
		return nil
	}
	if strings.EqualFold(source, target) {
		m.setToast("Already in "+target, toastInfo, 2*time.Second)
		return nil
	}

	if source == "all" {
		added, skipped, err := m.playlists.AddMany(target, paths)
		if err != nil {
			m.setToast("Could not update playlist: "+err.Error(), toastError, 4*time.Second)
			return nil
		}
		m.clearSelection()
		if added == 0 {
			m.setToast("All tracks are already in "+target, toastInfo, 3*time.Second)
			return nil
		}
		msg := "Added " + trackCount(added) + " to " + target
		if skipped > 0 {
			msg += " · " + strconv.Itoa(skipped) + " already there"
		}
		m.setToast(msg, toastSuccess, 4*time.Second)
		m.markDirty()
		return nil
	}

	added, moved, skipped, err := m.playlists.MoveMany(source, target, paths)
	if err != nil {
		m.setToast("Could not move tracks: "+err.Error(), toastError, 4*time.Second)
		return nil
	}
	if strings.EqualFold(m.view, source) {
		m.rebuildVisible("")
	} else {
		m.clearSelection()
	}
	var msg string
	kind := toastSuccess
	switch {
	case moved == 0 && added == 0:
		msg = "Already in " + target
		kind = toastInfo
	case added == 0 && skipped > 0:
		msg = "Removed " + trackCount(moved) + " from " + source + " · already in " + target
	case moved > 0:
		msg = "Moved " + trackCount(moved) + " to " + target
	default:
		msg = "Added " + trackCount(added) + " to " + target
	}
	if skipped > 0 && moved > 0 && added > 0 {
		msg += " · " + strconv.Itoa(skipped) + " already there"
	}
	m.setToast(msg, kind, 4*time.Second)
	m.markDirty()
	return nil
}

func trackCount(n int) string {
	if n == 1 {
		return "1 track"
	}
	return strconv.Itoa(n) + " tracks"
}
