// Package applog writes a small rolling log next to the user's data.
package applog

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const maxLogSize = 2 << 20 // 2 MiB

var (
	mu     sync.Mutex
	logger *log.Logger
	file   *os.File
	path   string
)

// Init opens data/logs/xngxltuiplayer.log. Failure is not fatal: the caller
// can keep running without a log file.
func Init(dataDir string) error {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(dir, "xngxltuiplayer.log")

	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		_ = os.Remove(logPath + ".1")
		_ = os.Rename(logPath, logPath+".1")
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
	file = f
	path = logPath
	logger = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	return nil
}

// Path returns the active log path, or an empty string when logging is off.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// Printf writes a line to the log. It is safe to call before Init.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Sync()
		_ = file.Close()
		file = nil
	}
	logger = nil
	path = ""
}

// Writer returns the active log writer for redirecting standard log output.
func Writer() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return io.Discard
	}
	return file
}

// Describe returns a short human-readable path for diagnostics.
func Describe() string {
	if p := Path(); p != "" {
		return p
	}
	return "disabled"
}
