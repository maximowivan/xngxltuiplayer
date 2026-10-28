package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// openThemePicker shows the palette list without changing the current theme.
func (m *Model) openThemePicker() {
	m.modal = modalThemes
	m.modalErr = ""
	m.themeBefore = m.theme
	m.themeCursor = themeIndex(m.theme)
}

func (m *Model) previewTheme(delta int) {
	if len(themePalettes) == 0 {
		return
	}
	m.themeCursor = ((m.themeCursor+delta)%len(themePalettes) + len(themePalettes)) % len(themePalettes)
	m.theme = applyTheme(themePalettes[m.themeCursor].Name)
}

func (m *Model) previewThemeAt(index int) {
	if len(themePalettes) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(themePalettes) {
		index = len(themePalettes) - 1
	}
	m.themeCursor = index
	m.theme = applyTheme(themePalettes[index].Name)
}

func (m *Model) commitThemePicker() {
	if len(themePalettes) == 0 {
		m.closeModal()
		return
	}
	p := themePalettes[m.themeCursor]
	m.theme = applyTheme(p.Name)
	m.themeBefore = m.theme
	m.markDirty()
	m.closeModal()
	m.setToast(iconTheme+" Theme: "+p.Label, toastInfo, 1600*time.Millisecond)
}

func (m *Model) cancelThemePicker() {
	if m.themeBefore != "" {
		m.theme = applyTheme(m.themeBefore)
	}
	m.themeCursor = themeIndex(m.theme)
	m.closeModal()
}

func (m *Model) renderThemePicker(w, contentH int) string {
	width := minInt(64, maxInt(42, w-8))
	innerW := width - 4
	height := modalHeight(contentH, minInt(len(themePalettes)+8, 26))
	innerH := height - 2

	lines := []string{
		textStyle.Render("Choose a palette. Arrows preview it live."),
		dimStyle.Render(fmt.Sprintf("%d themes", len(themePalettes))),
		"",
	}
	rows := maxInt(1, innerH-6)
	offset := 0
	if m.themeCursor >= rows {
		offset = m.themeCursor - rows + 1
	}
	if offset > len(themePalettes)-rows {
		offset = maxInt(0, len(themePalettes)-rows)
	}
	end := minInt(len(themePalettes), offset+rows)
	for i := offset; i < end; i++ {
		p := themePalettes[i]
		marker := "  "
		if i == m.themeCursor {
			marker = currentGlyphs.Prompt
		}
		label := p.Label
		if p.Light {
			label += "  light"
		}
		row := padBetween(marker+" "+label, "", innerW)
		if i == m.themeCursor {
			lines = append(lines, selectedStyle.Render(row))
		} else {
			lines = append(lines, textStyle.Render(row))
		}
	}
	for len(lines) < innerH-2 {
		lines = append(lines, "")
	}
	lines = append(lines, dimStyle.Render("enter apply"+currentGlyphs.Separator+"esc cancel"+currentGlyphs.Separator+"arrows preview"))
	return renderBox("Theme", lines, width, height, colAccent, colAccent)
}

func (m *Model) handleThemePickerKey(key string) tea.Cmd {
	switch key {
	case "esc", "q":
		m.cancelThemePicker()
	case "up", "k":
		m.previewTheme(-1)
	case "down", "j":
		m.previewTheme(1)
	case "pgup":
		m.previewTheme(-5)
	case "pgdown":
		m.previewTheme(5)
	case "home":
		m.previewThemeAt(0)
	case "end":
		m.previewThemeAt(len(themePalettes) - 1)
	case "enter":
		m.commitThemePicker()
	}
	return nil
}
