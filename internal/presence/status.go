package presence

import (
	"errors"
	"os"
)

// Status is emitted when the Discord connection state changes.
type Status struct {
	Connected bool
	Err       error
}

// DescribeError turns low-level IPC errors into actionable text.
func DescribeError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, os.ErrPermission):
		return "access denied — Discord runs as administrator; restart Discord normally or run the player as administrator"
	case errors.Is(err, os.ErrNotExist):
		return "Discord is not running"
	default:
		return err.Error()
	}
}

// Probe checks that Discord is running and accepts an IPC handshake. It does
// not publish an activity.
func Probe(cfg Config) error {
	if !cfg.Configured() {
		return errors.New("Discord Rich Presence is not configured")
	}
	conn, err := connectIPC(cfg.ClientID)
	if err != nil {
		return err
	}
	conn.close()
	return nil
}
