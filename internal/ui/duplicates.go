package ui

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
)

type duplicatesLoadedMsg struct {
	groups []library.DuplicateGroup
	err    error
}

type duplicatesMergedMsg struct {
	groupIndex int
	keeper     model.Track
	removed    []string
	err        error
}

func (m *Model) openDuplicates() tea.Cmd {
	m.modal = modalDuplicates
	m.modalErr = ""
	m.dupGroups = nil
	m.dupGroup = 0
	m.dupKeeper = 0
	m.dupOffset = 0
	m.dupLoading = true
	m.dupMerging = false
	return func() tea.Msg {
		groups, err := m.lib.FindDuplicates()
		return duplicatesLoadedMsg{groups: groups, err: err}
	}
}

func (m *Model) handleDuplicatesLoaded(msg duplicatesLoadedMsg) {
	m.dupLoading = false
	if msg.err != nil {
		m.modalErr = msg.err.Error()
		return
	}
	m.dupGroups = msg.groups
	m.dupGroup = 0
	m.dupKeeper = 0
	m.dupOffset = 0
	m.chooseDuplicateKeeper()
}

func (m *Model) currentDuplicateGroup() (library.DuplicateGroup, bool) {
	if m.dupGroup < 0 || m.dupGroup >= len(m.dupGroups) {
		return library.DuplicateGroup{}, false
	}
	return m.dupGroups[m.dupGroup], true
}

func (m *Model) chooseDuplicateKeeper() {
	group, ok := m.currentDuplicateGroup()
	if !ok {
		m.dupKeeper = 0
		return
	}
	best := 0
	for i := 1; i < len(group.Tracks); i++ {
		if duplicateQuality(group.Tracks[i]) > duplicateQuality(group.Tracks[best]) {
			best = i
		}
	}
	m.dupKeeper = best
	m.dupOffset = 0
}

func duplicateQuality(t model.Track) float64 {
	if t.Duration > 0 {
		return float64(t.Size) * 8 / t.Duration.Seconds()
	}
	return float64(t.Size)
}

func duplicateMeta(t model.Track) string {
	size := "?"
	if t.Size > 0 {
		size = humanSize(t.Size)
	}
	if t.Duration > 0 && t.Size > 0 {
		return fmt.Sprintf("%s  ~%d kbps", size, int(float64(t.Size)*8/t.Duration.Seconds()/1000))
	}
	return size
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n >= div*unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func (m *Model) renderDuplicatesModal(w, contentH int) string {
	width := minInt(94, maxInt(54, w-8))
	innerW := width - 4
	height := modalHeight(contentH, 21)
	innerH := height - 2

	lines := []string{}
	if m.dupLoading {
		lines = append(lines, textStyle.Render("Scanning the library for duplicates"+currentGlyphs.Ellipsis))
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return renderBox("Duplicates", lines, width, height, colAccent, colAccent)
	}
	if m.dupMerging {
		lines = append(lines, textStyle.Render("Merging duplicates"+currentGlyphs.Ellipsis))
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return renderBox("Duplicates", lines, width, height, colAccent, colAccent)
	}
	if m.modalErr != "" {
		lines = append(lines, errorStyle.Render(truncateText(cleanText(m.modalErr), innerW)))
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return renderBox("Duplicates", lines, width, height, colAccent, colAccent)
	}
	if len(m.dupGroups) == 0 {
		lines = append(lines,
			textStyle.Render("No duplicates found."),
			"",
			dimStyle.Render("Duplicates are matched by artist, title and duration."),
		)
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return renderBox("Duplicates", lines, width, height, colAccent, colAccent)
	}

	group, _ := m.currentDuplicateGroup()
	mode := "possible duplicate"
	if group.Exact {
		mode = "exact copy"
	}
	lines = append(lines,
		sectionStyle.Render(fmt.Sprintf("%d/%d  %s %s %s", m.dupGroup+1, len(m.dupGroups), cleanText(group.Artist), currentGlyphs.Dash, cleanText(group.Title))),
		dimStyle.Render(mode+currentGlyphs.Separator+"choose the file to keep"),
		"",
	)

	rows := maxInt(1, innerH-6)
	if m.dupKeeper < m.dupOffset {
		m.dupOffset = m.dupKeeper
	}
	if m.dupKeeper >= m.dupOffset+rows {
		m.dupOffset = m.dupKeeper - rows + 1
	}
	if m.dupOffset < 0 {
		m.dupOffset = 0
	}
	if maxOffset := maxInt(0, len(group.Tracks)-rows); m.dupOffset > maxOffset {
		m.dupOffset = maxOffset
	}

	for i := m.dupOffset; i < len(group.Tracks) && i < m.dupOffset+rows; i++ {
		t := group.Tracks[i]
		mark := "[ ]"
		if i == m.dupKeeper {
			mark = "[x]"
		}
		left := mark + " " + cleanText(filepath.Base(t.Path))
		right := duplicateMeta(t)
		row := padBetween(left, right, innerW)
		if i == m.dupKeeper {
			lines = append(lines, selectedStyle.Render(row))
		} else {
			lines = append(lines, textStyle.Render(row))
		}
	}
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}
	lines = append(lines, faintStyle.Render("Duplicate files are moved to data/trash, not deleted."))
	return renderBox("Duplicates", lines, width, height, colAccent, colAccent)
}

func (m *Model) moveDuplicateKeeper(delta int) {
	group, ok := m.currentDuplicateGroup()
	if !ok {
		return
	}
	m.dupKeeper += delta
	if m.dupKeeper < 0 {
		m.dupKeeper = 0
	}
	if m.dupKeeper >= len(group.Tracks) {
		m.dupKeeper = len(group.Tracks) - 1
	}
}

func (m *Model) moveDuplicateGroup(delta int) {
	if len(m.dupGroups) == 0 {
		return
	}
	m.dupGroup += delta
	if m.dupGroup < 0 {
		m.dupGroup = 0
	}
	if m.dupGroup >= len(m.dupGroups) {
		m.dupGroup = len(m.dupGroups) - 1
	}
	m.chooseDuplicateKeeper()
}

func (m *Model) mergeDuplicateGroup() tea.Cmd {
	if m.dupMerging {
		return nil
	}
	group, ok := m.currentDuplicateGroup()
	if !ok || m.dupKeeper < 0 || m.dupKeeper >= len(group.Tracks) {
		return nil
	}
	keeper := group.Tracks[m.dupKeeper]
	removed := make([]model.Track, 0, len(group.Tracks)-1)
	for i, t := range group.Tracks {
		if i != m.dupKeeper {
			removed = append(removed, t)
		}
	}
	if len(removed) == 0 {
		return nil
	}

	// Stop playback if any file involved in the merge is open.
	if m.current != nil && groupContainsPath(group, m.current.Path) {
		m.engine.Stop()
		m.loaded = false
		m.paused = false
		m.ended = false
		m.position = 0
	}
	for _, path := range m.queue {
		if groupContainsPath(group, path) {
			m.engine.Stop()
			m.loaded = false
			m.paused = false
			m.ended = false
			m.position = 0
			break
		}
	}

	index := m.dupGroup
	m.dupMerging = true
	return func() tea.Msg {
		removedPaths := make([]string, 0, len(removed))
		for _, dup := range removed {
			if _, err := m.lib.MoveToTrash(dup); err != nil {
				return duplicatesMergedMsg{groupIndex: index, keeper: keeper, removed: removedPaths, err: err}
			}
			if _, err := m.playlists.ReplacePathEverywhere(dup.Path, keeper.Path); err != nil {
				return duplicatesMergedMsg{groupIndex: index, keeper: keeper, removed: removedPaths, err: err}
			}
			if err := m.stats.Merge(dup.Path, keeper.Path); err != nil {
				return duplicatesMergedMsg{groupIndex: index, keeper: keeper, removed: removedPaths, err: err}
			}
			removedPaths = append(removedPaths, dup.Path)
		}
		return duplicatesMergedMsg{groupIndex: index, keeper: keeper, removed: removedPaths}
	}
}

func groupContainsPath(group library.DuplicateGroup, path string) bool {
	for _, t := range group.Tracks {
		if trackKey(t.Path) == trackKey(path) {
			return true
		}
	}
	return false
}

func (m *Model) handleDuplicatesMerged(msg duplicatesMergedMsg) tea.Cmd {
	m.dupMerging = false
	if msg.err != nil {
		m.closeModal()
		m.setToast("Merge failed: "+cleanText(msg.err.Error()), toastError, 6*time.Second)
		return m.startRefresh()
	}
	removed := make(map[string]bool, len(msg.removed))
	for _, path := range msg.removed {
		removed[trackKey(path)] = true
	}
	keeperKey := trackKey(msg.keeper.Path)
	if m.current != nil && removed[trackKey(m.current.Path)] {
		updated := msg.keeper
		m.current = &updated
		m.loaded = false
		m.paused = false
		m.ended = false
		m.position = 0
		m.duration = msg.keeper.Duration
	}
	if len(m.queue) > 0 {
		queue := make([]string, 0, len(m.queue))
		seen := make(map[string]bool)
		for _, path := range m.queue {
			if removed[trackKey(path)] {
				path = msg.keeper.Path
			}
			key := trackKey(path)
			if seen[key] {
				continue
			}
			seen[key] = true
			queue = append(queue, path)
		}
		m.queue = queue
		if m.current != nil {
			m.queueIndex = 0
			for i, path := range queue {
				if trackKey(path) == keeperKey {
					m.queueIndex = i
					break
				}
			}
		}
		if m.queueIndex >= len(queue) {
			m.queueIndex = maxInt(0, len(queue)-1)
		}
	}
	tracks := m.tracks[:0]
	for _, t := range m.tracks {
		if removed[trackKey(t.Path)] {
			continue
		}
		tracks = append(tracks, t)
	}
	m.tracks = tracks
	groups := m.dupGroups[:0]
	for _, group := range m.dupGroups {
		kept := group.Tracks[:0]
		for _, t := range group.Tracks {
			if !removed[trackKey(t.Path)] {
				kept = append(kept, t)
			}
		}
		if len(kept) < 2 {
			continue
		}
		group.Tracks = kept
		groups = append(groups, group)
	}
	m.dupGroups = groups
	if m.dupGroup >= len(m.dupGroups) {
		m.dupGroup = maxInt(0, len(m.dupGroups)-1)
	}
	m.chooseDuplicateKeeper()
	m.rebuildVisible(msg.keeper.Path)
	m.markDirty()
	m.setToast(fmt.Sprintf("Merged %d duplicate(s)", len(msg.removed)), toastSuccess, 3*time.Second)
	if len(m.dupGroups) == 0 {
		m.closeModal()
	}
	return m.startRefresh()
}
