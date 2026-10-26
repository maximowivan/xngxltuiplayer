package ui

import "tuiplay/internal/model"

// isSelected reports whether a track is part of the multi-selection.
func (m *Model) isSelected(path string) bool {
	if len(m.selection) == 0 {
		return false
	}
	return m.selection[trackKey(path)]
}

func (m *Model) selectionCount() int {
	return len(m.selection)
}

// toggleSelected flips a track in the multi-selection and returns its new
// state.
func (m *Model) toggleSelected(path string) bool {
	key := trackKey(path)
	if m.selection == nil {
		m.selection = make(map[string]bool)
	}
	if m.selection[key] {
		delete(m.selection, key)
		if len(m.selection) == 0 {
			m.selection = nil
		}
		return false
	}
	m.selection[key] = true
	return true
}

func (m *Model) clearSelection() {
	m.selection = nil
}

// selectionPaths returns the selected paths in display order.
func (m *Model) selectionPaths() []string {
	if len(m.selection) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.selection))
	for _, t := range m.visible {
		if m.selection[trackKey(t.Path)] {
			out = append(out, t.Path)
		}
	}
	return out
}

func (m *Model) selectionTracks() []model.Track {
	if len(m.selection) == 0 {
		return nil
	}
	out := make([]model.Track, 0, len(m.selection))
	for _, t := range m.visible {
		if m.selection[trackKey(t.Path)] {
			out = append(out, t)
		}
	}
	return out
}

func (m *Model) selectAllVisible() {
	if len(m.visible) == 0 {
		return
	}
	m.selection = make(map[string]bool, len(m.visible))
	for _, t := range m.visible {
		m.selection[trackKey(t.Path)] = true
	}
}
