package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/media"
)

type mediaCommandMsg struct {
	command media.Command
}

// SetMediaController installs the platform media-key/MPRIS integration.
func (m *Model) SetMediaController(c media.Controller) {
	m.mediaController = c
	m.lastMediaUpdate = time.Time{}
	m.syncMedia()
}

func (m *Model) waitMediaCommand() tea.Cmd {
	c := m.mediaController
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		cmd, ok := <-c.Events()
		if !ok {
			return nil
		}
		return mediaCommandMsg{command: cmd}
	}
}

func (m *Model) handleMediaCommand(cmd media.Command) tea.Cmd {
	switch cmd.Kind {
	case media.PlayPause:
		return m.togglePlay()
	case media.Play:
		if m.current == nil || !m.loaded || m.paused || m.ended {
			return m.togglePlay()
		}
		return nil
	case media.Pause:
		if m.current != nil && m.loaded && !m.paused && !m.ended {
			return m.togglePlay()
		}
		return nil
	case media.Next:
		return m.nextTrack()
	case media.Previous:
		return m.prevTrack()
	case media.Stop:
		m.stopPlayback()
		return nil
	case media.Seek:
		m.seek(cmd.Offset)
		return nil
	case media.SeekAbsolute:
		m.seekTo(cmd.Offset)
		return nil
	default:
		return nil
	}
}

func (m *Model) stopPlayback() {
	m.engine.Stop()
	m.loaded = false
	m.paused = false
	m.ended = false
	m.position = 0
	m.markDirty()
	m.syncPresence()
}

func (m *Model) mediaState() media.State {
	state := media.State{
		Volume:  m.volume,
		Shuffle: m.shuffle,
		Repeat:  m.repeat,
	}
	if m.current == nil {
		return state
	}
	state.Loaded = m.loaded
	state.Playing = m.loaded && !m.paused && !m.ended
	state.Paused = m.loaded && m.paused
	state.Title = cleanText(m.current.DisplayTitle())
	state.Artist = cleanText(m.current.DisplayArtist())
	state.Album = cleanText(m.current.DisplayAlbum())
	state.Path = m.current.Path
	state.Duration = m.duration
	state.Position = m.currentPosition()
	return state
}

// syncMedia publishes state changes at most once per second; metadata and
// paused/playing transitions are sent immediately.
func (m *Model) syncMedia() {
	if m.mediaController == nil {
		return
	}
	state := m.mediaState()
	changed := state
	changed.Position = m.lastMediaState.Position
	if changed != m.lastMediaState || time.Since(m.lastMediaUpdate) >= time.Second {
		m.mediaController.Update(state)
		m.lastMediaState = state
		m.lastMediaUpdate = time.Now()
	}
}
