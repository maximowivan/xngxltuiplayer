package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/presence"
)

// PresenceSink receives playback updates for Discord Rich Presence.
type PresenceSink interface {
	Update(presence.Activity)
	Clear()
	Status() <-chan presence.Status
}

type presenceStatusMsg struct {
	status presence.Status
}

func (m *Model) SetPresence(sink PresenceSink) {
	m.presence = sink
	if sink != nil {
		m.presenceStatus = sink.Status()
	}
	m.syncPresence()
}

func (m *Model) waitPresenceStatus() tea.Cmd {
	ch := m.presenceStatus
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return nil
		}
		return presenceStatusMsg{status: status}
	}
}

func (m *Model) handlePresenceStatus(msg presenceStatusMsg) tea.Cmd {
	switch {
	case msg.status.Err != nil:
		m.presenceWarned = true
		m.setToast("Discord: "+presence.DescribeError(msg.status.Err), toastWarning, 7*time.Second)
	case msg.status.Connected && m.presenceWarned:
		m.presenceWarned = false
		m.setToast("Discord connected", toastSuccess, 3*time.Second)
	}
	return m.waitPresenceStatus()
}

func (m *Model) syncPresence() {
	if m.presence == nil {
		return
	}
	if m.current == nil {
		m.presence.Clear()
		return
	}
	// A selected but never played track (for example after a restart) must not
	// appear in Discord as a paused track.
	if !m.loaded && !m.ended {
		m.presence.Clear()
		return
	}
	duration := m.duration
	if duration <= 0 {
		duration = m.current.Duration
	}
	m.presence.Update(presence.Activity{
		TrackKey:  trackKey(m.current.Path),
		Title:     m.current.DisplayTitle(),
		Artist:    strings.TrimSpace(m.current.Artist),
		Album:     strings.TrimSpace(m.current.Album),
		CoverURL:  m.current.CoverURL,
		SourceURL: m.current.Source,
		Position:  m.currentPosition(),
		Duration:  duration,
		Paused:    m.paused || !m.loaded || m.ended,
	})
}
