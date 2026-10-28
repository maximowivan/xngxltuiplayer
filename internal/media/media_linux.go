//go:build linux

package media

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	mprisBusName     = "org.mpris.MediaPlayer2.xngxltuiplayer"
	mprisPath        = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRootIface   = "org.mpris.MediaPlayer2"
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
)

type linuxController struct {
	conn   *dbus.Conn
	events chan Command
	props  *prop.Properties

	mu    sync.Mutex
	state State

	closeOnce sync.Once
}

type rootController struct {
	c *linuxController
}

// Start exposes the player over MPRIS2. Desktop environments forward media keys
// to the MPRIS player, so this also enables keyboard/headset controls on Linux.
func Start() (Controller, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("request MPRIS name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("MPRIS name is already owned")
	}

	c := &linuxController{
		conn:   conn,
		events: make(chan Command, 8),
	}
	props, err := prop.Export(conn, mprisPath, prop.Map{
		mprisRootIface: {
			"Identity":            {Value: "xngxltuiplayer", Emit: prop.EmitConst},
			"DesktopEntry":        {Value: "", Emit: prop.EmitConst},
			"CanQuit":             {Value: false, Emit: prop.EmitConst},
			"CanRaise":            {Value: false, Emit: prop.EmitConst},
			"HasTrackList":        {Value: false, Emit: prop.EmitConst},
			"SupportedUriSchemes": {Value: []string{}, Emit: prop.EmitConst},
			"SupportedMimeTypes":  {Value: []string{}, Emit: prop.EmitConst},
		},
		mprisPlayerIface: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     {Value: "None", Emit: prop.EmitTrue},
			"Rate":           {Value: 1.0, Emit: prop.EmitConst},
			"Shuffle":        {Value: false, Emit: prop.EmitTrue},
			"Metadata":       {Value: map[string]dbus.Variant{}, Emit: prop.EmitTrue},
			"Volume":         {Value: 1.0, Emit: prop.EmitTrue},
			"Position":       {Value: int64(0), Emit: prop.EmitTrue},
			"MinimumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"MaximumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"CanGoNext":      {Value: true, Emit: prop.EmitConst},
			"CanGoPrevious":  {Value: true, Emit: prop.EmitConst},
			"CanPlay":        {Value: true, Emit: prop.EmitConst},
			"CanPause":       {Value: true, Emit: prop.EmitConst},
			"CanSeek":        {Value: false, Emit: prop.EmitTrue},
			"CanControl":     {Value: true, Emit: prop.EmitConst},
		},
	})
	if err != nil {
		_, _ = conn.ReleaseName(mprisBusName)
		_ = conn.Close()
		return nil, fmt.Errorf("export MPRIS properties: %w", err)
	}
	c.props = props
	if err := conn.ExportWithMap(c, map[string]string{"SeekOffset": "Seek"}, mprisPath, mprisPlayerIface); err != nil {
		_, _ = conn.ReleaseName(mprisBusName)
		_ = conn.Close()
		return nil, fmt.Errorf("export MPRIS player: %w", err)
	}
	if err := conn.Export(&rootController{c: c}, mprisPath, mprisRootIface); err != nil {
		_, _ = conn.ReleaseName(mprisBusName)
		_ = conn.Close()
		return nil, fmt.Errorf("export MPRIS root: %w", err)
	}
	return c, nil
}

func (c *linuxController) Events() <-chan Command { return c.events }

func (c *linuxController) Update(state State) {
	c.mu.Lock()
	old := c.state
	c.state = state
	c.mu.Unlock()

	if !state.Loaded {
		c.props.SetMust(mprisPlayerIface, "PlaybackStatus", "Stopped")
		c.props.SetMust(mprisPlayerIface, "Metadata", map[string]dbus.Variant{})
		c.props.SetMust(mprisPlayerIface, "CanSeek", false)
		return
	}
	status := "Stopped"
	if state.Playing {
		status = "Playing"
	} else if state.Paused {
		status = "Paused"
	}
	if old.Loaded != state.Loaded || old.Paused != state.Paused || old.Playing != state.Playing {
		c.props.SetMust(mprisPlayerIface, "PlaybackStatus", status)
	}
	if old.Path != state.Path || old.Title != state.Title || old.Artist != state.Artist ||
		old.Album != state.Album || old.Duration != state.Duration {
		c.props.SetMust(mprisPlayerIface, "Metadata", metadataFor(state))
	}
	if state.Duration > 0 {
		c.props.SetMust(mprisPlayerIface, "CanSeek", true)
	}
	if state.Position > 0 && (old.Position-state.Position > time.Second || state.Position-old.Position > time.Second) {
		c.props.SetMust(mprisPlayerIface, "Position", state.Position.Microseconds())
	}
	if old.Volume != state.Volume {
		c.props.SetMust(mprisPlayerIface, "Volume", float64(state.Volume)/100)
	}
	if old.Shuffle != state.Shuffle {
		c.props.SetMust(mprisPlayerIface, "Shuffle", state.Shuffle)
	}
	if loop := loopStatus(state.Repeat); loop != loopStatus(old.Repeat) {
		c.props.SetMust(mprisPlayerIface, "LoopStatus", loop)
	}
}

func metadataFor(state State) map[string]dbus.Variant {
	if state.Path == "" {
		return map[string]dbus.Variant{}
	}
	sum := sha1.Sum([]byte(filepath.Clean(state.Path)))
	id := dbus.ObjectPath("/org/mpris/MediaPlayer2/Track/" + hex.EncodeToString(sum[:]))
	out := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(id),
		"xesam:title":   dbus.MakeVariant(state.Title),
		"mpris:length":  dbus.MakeVariant(state.Duration.Microseconds()),
	}
	if state.Artist != "" {
		out["xesam:artist"] = dbus.MakeVariant([]string{state.Artist})
	}
	if state.Album != "" {
		out["xesam:album"] = dbus.MakeVariant(state.Album)
	}
	return out
}

func loopStatus(repeat string) string {
	switch repeat {
	case "one":
		return "Track"
	case "all":
		return "Playlist"
	default:
		return "None"
	}
}

func (c *linuxController) send(cmd Command) {
	select {
	case c.events <- cmd:
	default:
	}
}

func (c *linuxController) Next() *dbus.Error {
	c.send(Command{Kind: Next})
	return nil
}

func (c *linuxController) Previous() *dbus.Error {
	c.send(Command{Kind: Previous})
	return nil
}

func (c *linuxController) Play() *dbus.Error {
	c.send(Command{Kind: Play})
	return nil
}

func (c *linuxController) Pause() *dbus.Error {
	c.send(Command{Kind: Pause})
	return nil
}

func (c *linuxController) PlayPause() *dbus.Error {
	c.send(Command{Kind: PlayPause})
	return nil
}

func (c *linuxController) Stop() *dbus.Error {
	c.send(Command{Kind: Stop})
	return nil
}

func (c *linuxController) SeekOffset(offset int64) *dbus.Error {
	c.send(Command{Kind: Seek, Offset: time.Duration(offset) * time.Microsecond})
	return nil
}

func (c *linuxController) SetPosition(_ dbus.ObjectPath, position int64) *dbus.Error {
	c.send(Command{Kind: SeekAbsolute, Offset: time.Duration(position) * time.Microsecond})
	return nil
}

func (c *linuxController) OpenUri(_ string) *dbus.Error {
	return nil
}

func (c *linuxController) Close() error {
	c.closeOnce.Do(func() {
		_, _ = c.conn.ReleaseName(mprisBusName)
		close(c.events)
		_ = c.conn.Close()
	})
	return nil
}

func (r *rootController) Raise() *dbus.Error { return nil }
func (r *rootController) Quit() *dbus.Error {
	r.c.send(Command{Kind: Stop})
	return nil
}

func Available() string {
	return "MPRIS2 (Linux desktop)"
}
