// Package instance prevents two players from writing to the same data folder.
package instance

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning is returned when another process holds the data folder.
var ErrAlreadyRunning = errors.New("another xngxltuiplayer instance is already running")

// Lock represents an acquired advisory lock. Release must be called on exit.
type Lock struct {
	file *os.File
}

// Acquire locks dataDir/player.lock without waiting. The operating system
// releases the lock automatically if the process crashes.
func Acquire(dataDir string) (*Lock, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "player.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLocked) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Release unlocks and closes the lock file.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unlockFile(l.file)
	_ = l.file.Close()
	l.file = nil
}
