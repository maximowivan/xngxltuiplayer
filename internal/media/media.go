// Package media exposes platform media keys and MPRIS integration.
package media

import "time"

// CommandKind is an action requested by an OS media key or MPRIS client.
type CommandKind int

const (
	PlayPause CommandKind = iota
	Play
	Pause
	Next
	Previous
	Stop
	Seek
	SeekAbsolute
)

// Command is a media control request.
type Command struct {
	Kind   CommandKind
	Offset time.Duration
}

// State is the playback state exported to the operating system.
type State struct {
	Loaded   bool
	Playing  bool
	Paused   bool
	Title    string
	Artist   string
	Album    string
	Path     string
	Duration time.Duration
	Position time.Duration
	Volume   int
	Shuffle  bool
	Repeat   string
}

// Controller receives media commands and exposes playback state.
type Controller interface {
	Events() <-chan Command
	Update(State)
	Close() error
}

type noopController struct {
	events chan Command
}

func (n *noopController) Events() <-chan Command { return n.events }
func (n *noopController) Update(State)           {}
func (n *noopController) Close() error {
	close(n.events)
	return nil
}

func newNoop() Controller {
	return &noopController{events: make(chan Command)}
}
