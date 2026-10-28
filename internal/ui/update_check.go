package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/updater"
)

type updateCheckedMsg struct {
	release updater.Release
	err     error
}

type updateProgressMsg struct {
	progress updater.Progress
}

type updateStageDoneMsg struct {
	stage string
	err   error
}

type updateApplyMsg struct {
	stage string
}

// SetProgram lets background commands deliver progress messages while the
// Bubble Tea program is running.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// EnableUpdateCheck turns on the silent startup check. It is only called on
// Windows by main.
func (m *Model) EnableUpdateCheck(version string) {
	m.updateEnabled = true
	m.updateVersion = strings.TrimSpace(version)
}

func (m *Model) checkUpdateCmd() tea.Cmd {
	if !m.updateEnabled || m.updateVersion == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		rel, err := updater.Latest(ctx)
		return updateCheckedMsg{release: rel, err: err}
	}
}

func (m *Model) handleUpdateChecked(msg updateCheckedMsg) {
	if msg.err != nil || msg.release.TagName == "" {
		return
	}
	if !updater.IsNewer(msg.release.TagName, m.updateVersion) {
		return
	}
	m.updateRelease = msg.release
	m.updateAvailable = true
	if m.modal == modalNone {
		m.openUpdatePrompt()
		return
	}
	m.setToast("Update "+cleanText(msg.release.TagName)+" available (U)", toastInfo, 5*time.Second)
}

func (m *Model) openUpdatePrompt() {
	m.modal = modalUpdate
	m.modalErr = ""
	m.updatePhase = ""
	m.updateDone = 0
	m.updateTotal = 0
	m.updatePercent = 0
	m.updateErr = ""
	m.updateStaging = false
}

func (m *Model) startUpdateCmd() tea.Cmd {
	if m.updateStaging || m.updateRelease.TagName == "" {
		return nil
	}
	m.updatePhase = "download"
	m.updateDone = 0
	m.updateTotal = 0
	m.updatePercent = 0
	m.updateErr = ""
	m.updateStaging = true

	rel := m.updateRelease
	dataDir := m.cfg.DataDir
	report := func(p updater.Progress) {
		if m.program != nil {
			m.program.Send(updateProgressMsg{progress: p})
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		stage, err := updater.StageWithProgress(ctx, dataDir, rel, report)
		return updateStageDoneMsg{stage: stage, err: err}
	}
}

func (m *Model) handleUpdateProgress(msg updateProgressMsg) {
	if m.modal != modalUpdate || !m.updateStaging {
		return
	}
	p := msg.progress
	switch p.Phase {
	case "download":
		m.updatePhase = "download"
		m.updateDone = p.Done
		m.updateTotal = p.Total
		m.updatePercent = p.Percent
	case "verify", "extract", "ready":
		m.updatePhase = p.Phase
	}
}

func (m *Model) handleUpdateStageDone(msg updateStageDoneMsg) tea.Cmd {
	m.updateStaging = false
	if msg.err != nil {
		m.updatePhase = "error"
		m.updateErr = msg.err.Error()
		return nil
	}
	m.updatePhase = "ready"
	m.updatePercent = 1
	stage := msg.stage
	return tea.Tick(450*time.Millisecond, func(time.Time) tea.Msg {
		return updateApplyMsg{stage: stage}
	})
}

func (m *Model) handleUpdateApply(msg updateApplyMsg) tea.Cmd {
	if err := updater.Apply(msg.stage, m.cfg.BaseDir); err != nil {
		m.updatePhase = "error"
		m.updateErr = err.Error()
		return nil
	}
	m.saveSession()
	return tea.Quit
}

func (m *Model) renderUpdateModal(w, contentH int) string {
	width := minInt(84, maxInt(54, w-8))
	innerW := width - 4
	desired := 12
	if m.updatePhase == "download" {
		desired = 14
	}
	if m.updatePhase == "error" {
		desired = 13
	}
	height := modalHeight(contentH, desired)
	lines := []string{}
	tag := cleanText(m.updateRelease.TagName)

	switch m.updatePhase {
	case "":
		lines = append(lines,
			textStyle.Render("A new release is available."),
			"",
			accentStyle.Render("current "+cleanText(m.updateVersion))+faintStyle.Render(currentGlyphs.Separator)+
				accent2Style.Render("latest "+tag),
			"",
			dimStyle.Render("Download it, verify SHA256 and restart the player."),
		)
		if url := cleanText(m.updateRelease.HTMLURL); url != "" {
			lines = append(lines, faintStyle.Render(truncateText(url, innerW)))
		}
	case "download":
		lines = append(lines,
			sectionStyle.Render("Updating to "+tag),
			"",
			textStyle.Render("Downloading the release"),
		)
		barW := maxInt(12, innerW-12)
		lines = append(lines, progressBar(m.updatePercent, barW, true)+dimStyle.Render(fmt.Sprintf("  %d%%", int(m.updatePercent*100))))
		if m.updateTotal > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("%s / %s", humanSize(m.updateDone), humanSize(m.updateTotal))))
		} else {
			lines = append(lines, dimStyle.Render(humanSize(m.updateDone)))
		}
	case "verify":
		lines = append(lines,
			sectionStyle.Render("Updating to "+tag),
			"",
			textStyle.Render("Verifying SHA256"+strings.Repeat(".", m.spinner%4)),
			"",
			dimStyle.Render("The archive is checked before anything is replaced."),
		)
	case "extract":
		lines = append(lines,
			sectionStyle.Render("Updating to "+tag),
			"",
			textStyle.Render("Extracting files"+strings.Repeat(".", m.spinner%4)),
			"",
			dimStyle.Render("The player will restart automatically."),
		)
	case "ready":
		lines = append(lines,
			sectionStyle.Render("Update ready"),
			"",
			textStyle.Render("Restarting the player"+strings.Repeat(".", m.spinner%4)),
			"",
			dimStyle.Render("Music, playlists and settings are kept."),
		)
	case "error":
		lines = append(lines,
			errorStyle.Render("Update failed"),
			"",
			errorStyle.Render(truncateText(cleanText(m.updateErr), innerW)),
			"",
			dimStyle.Render("Press Enter to retry or Esc to close."),
		)
	}
	for len(lines) < height-3 {
		lines = append(lines, "")
	}
	return renderBox("Update", lines, width, height, colAccent, colAccent)
}
