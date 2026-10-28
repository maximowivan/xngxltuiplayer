package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/library"
	"tuiplay/internal/model"
)

type tagField struct {
	label string
	input textinput.Model
}

type tagEditor struct {
	track       model.Track
	fields      []tagField
	cover       textinput.Model
	removeCover bool
	focus       int
	err         string
}

type tagsSavedMsg struct {
	track   model.Track
	updated model.Track
	err     error
}

func newTagEditor(track model.Track) *tagEditor {
	values := []string{cleanText(track.Title), cleanText(track.Artist), cleanText(track.Album), cleanText(track.Year), cleanText(track.TrackNumber)}
	labels := []string{"Title", "Artist", "Album", "Year", "Track number"}
	limits := []int{512, 512, 512, 16, 16}
	e := &tagEditor{track: track}
	for i, label := range labels {
		in := newInput("", label, limits[i])
		in.SetValue(values[i])
		in.Blur()
		e.fields = append(e.fields, tagField{label: label, input: in})
	}
	e.cover = newInput(currentGlyphs.Prompt, "C:\\path\\cover.jpg (empty keeps the current one)", 4096)
	e.cover.Blur()
	return e
}

func (e *tagEditor) focusField(index int) tea.Cmd {
	if e == nil {
		return nil
	}
	total := len(e.fields) + 1
	if index < 0 {
		index = total - 1
	}
	if index >= total {
		index = 0
	}
	if e.focus < len(e.fields) {
		e.fields[e.focus].input.Blur()
	} else {
		e.cover.Blur()
	}
	e.focus = index
	if e.focus < len(e.fields) {
		return e.fields[e.focus].input.Focus()
	}
	return e.cover.Focus()
}

func (e *tagEditor) blur() {
	if e == nil {
		return
	}
	for i := range e.fields {
		e.fields[i].input.Blur()
	}
	e.cover.Blur()
}

func (m *Model) openTagEditor() tea.Cmd {
	track, ok := m.selectedTrack()
	if !ok {
		m.setToast("Select a track first", toastInfo, 2*time.Second)
		return nil
	}
	ext := strings.ToLower(filepath.Ext(track.Path))
	if ext != ".mp3" && ext != ".flac" {
		m.setToast("Tag editing supports MP3 and FLAC only", toastWarning, 4*time.Second)
		return nil
	}
	m.tags = newTagEditor(track)
	m.modal = modalEditTags
	m.modalErr = ""
	m.resizeTagInputs()
	return m.tags.focusField(0)
}

func (m *Model) resizeTagInputs() {
	if m.tags == nil {
		return
	}
	width := maxInt(12, modalWidth(m.width)-18)
	for i := range m.tags.fields {
		m.tags.fields[i].input.Width = width
	}
	m.tags.cover.Width = width
}

func (m *Model) renderTagModal(w, contentH int) string {
	width := minInt(88, maxInt(54, w-8))
	innerW := width - 4
	height := modalHeight(contentH, 18)
	lines := []string{
		textStyle.Render("Edit tags for " + truncateText(cleanText(filepath.Base(m.tagPath())), innerW)),
		dimStyle.Render("Changes are written to the audio file. The cover can be replaced."),
		"",
	}
	if m.tags == nil {
		lines = append(lines, dimStyle.Render("No track selected."))
	} else {
		for i, field := range m.tags.fields {
			label := fmt.Sprintf("%-13s", field.label)
			if i == m.tags.focus {
				label = accent2Style.Render(label)
			} else {
				label = dimStyle.Render(label)
			}
			inputW := maxInt(8, innerW-14)
			lines = append(lines, label+" "+fitLine(field.input.View(), inputW))
		}
		lines = append(lines, "")
		coverLabel := fmt.Sprintf("%-13s", "Cover")
		if m.tags.focus == len(m.tags.fields) {
			coverLabel = accent2Style.Render(coverLabel)
		} else {
			coverLabel = dimStyle.Render(coverLabel)
		}
		inputW := maxInt(8, innerW-14)
		if m.tags.focus == len(m.tags.fields) {
			lines = append(lines, coverLabel+" "+fitLine(m.tags.cover.View(), inputW))
		} else {
			lines = append(lines, coverLabel+" "+truncateText(m.tags.coverStatus(), inputW))
		}
		if m.tags.err != "" {
			lines = append(lines, "")
			lines = append(lines, errorStyle.Render(truncateText(cleanText(m.tags.err), innerW)))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("ctrl+s save"+currentGlyphs.Separator+"tab next"+currentGlyphs.Separator+"c cover"+currentGlyphs.Separator+"x remove"+currentGlyphs.Separator+"esc cancel"))
	return renderBox("Edit tags", lines, width, height, colAccent, colAccent)
}

func (e *tagEditor) coverStatus() string {
	if e == nil {
		return ""
	}
	if e.removeCover {
		return "will be removed"
	}
	if path := strings.TrimSpace(e.cover.Value()); path != "" {
		return "new: " + cleanText(filepath.Base(path))
	}
	if e.track.HasArt {
		return "embedded"
	}
	return "none"
}

func (m *Model) tagPath() string {
	if m.tags == nil {
		return ""
	}
	return m.tags.track.Path
}

func (m *Model) submitTagEdit() tea.Cmd {
	if m.tags == nil {
		return nil
	}
	m.tags.err = ""
	edit := library.TagEdit{
		Title:       strings.TrimSpace(m.tags.fields[0].input.Value()),
		Artist:      strings.TrimSpace(m.tags.fields[1].input.Value()),
		Album:       strings.TrimSpace(m.tags.fields[2].input.Value()),
		Year:        strings.TrimSpace(m.tags.fields[3].input.Value()),
		TrackNumber: strings.TrimSpace(m.tags.fields[4].input.Value()),
		RemoveCover: m.tags.removeCover,
	}
	if !edit.RemoveCover {
		if path := cleanCoverPath(m.tags.cover.Value()); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				m.tags.err = "Cover: " + err.Error()
				return nil
			}
			if len(data) > library.MaxCoverInput {
				m.tags.err = fmt.Sprintf("Cover: image is too large (%d MB, limit %d MB)", len(data)>>20, library.MaxCoverInput>>20)
				return nil
			}
			edit.CoverData = data
		}
	}
	track := m.tags.track
	if m.current != nil && trackKey(m.current.Path) == trackKey(track.Path) {
		// Windows may keep the decoded file open; stop it before the atomic
		// rename. The user can start playback again after saving.
		m.engine.Stop()
		m.loaded = false
		m.paused = false
		m.ended = false
		m.position = 0
	}
	return func() tea.Msg {
		updated, err := m.lib.UpdateTags(track, edit)
		return tagsSavedMsg{track: track, updated: updated, err: err}
	}
}

func cleanCoverPath(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func (m *Model) handleTagsSaved(msg tagsSavedMsg) tea.Cmd {
	if msg.err != nil {
		if m.tags != nil {
			m.tags.err = msg.err.Error()
		}
		return nil
	}
	m.tags = nil
	m.modal = modalNone
	m.modalErr = ""
	if m.current != nil && trackKey(m.current.Path) == trackKey(msg.track.Path) {
		updated := msg.updated
		m.current = &updated
		m.artFor = ""
		m.artImage = nil
		m.artLoading = false
	}
	for i := range m.tracks {
		if trackKey(m.tracks[i].Path) == trackKey(msg.track.Path) {
			m.tracks[i] = msg.updated
			break
		}
	}
	m.rebuildVisible(msg.track.Path)
	m.markDirty()
	m.setToast("Tags saved", toastSuccess, 2*time.Second)
	return m.startRefresh()
}
