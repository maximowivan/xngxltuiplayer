package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all filesystem locations used by the application.
type Config struct {
	BaseDir   string
	MusicDir  string
	DataDir   string
	CoversDir string
	TempDir   string
}

// Load resolves the music and data directories and makes sure they exist.
// Empty flags select a portable default next to the executable. When running
// through `go run` the executable lives in a temporary build directory, so the
// current working directory is used instead.
func Load(musicFlag, dataFlag string) (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	base := executableDir()
	if base == "" || isTemporaryBuild(base) {
		base = cwd
	}

	musicDir := musicFlag
	if musicDir == "" {
		musicDir = filepath.Join(base, "music")
	} else if !filepath.IsAbs(musicDir) {
		musicDir = filepath.Join(cwd, musicDir)
	}

	dataDir := dataFlag
	if dataDir == "" {
		dataDir = filepath.Join(base, "data")
	} else if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(cwd, dataDir)
	}

	cfg := Config{
		BaseDir:   base,
		MusicDir:  filepath.Clean(musicDir),
		DataDir:   filepath.Clean(dataDir),
		CoversDir: filepath.Join(filepath.Clean(dataDir), "covers"),
		TempDir:   filepath.Join(filepath.Clean(dataDir), "tmp"),
	}

	for _, dir := range []string{cfg.MusicDir, cfg.DataDir, cfg.CoversDir, cfg.TempDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Config{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return cfg, nil
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	return filepath.Dir(exe)
}

func isTemporaryBuild(dir string) bool {
	// `go run` builds the binary under a temporary go-build directory. Do not
	// treat every executable inside %TEMP% as a go-run build: portable copies
	// may legitimately live there and must keep their own music/data folders.
	return strings.Contains(strings.ToLower(dir), "go-build")
}

// Writable reports whether a file can be created inside dir.
func Writable(dir string) error {
	f, err := os.CreateTemp(dir, ".tuiplay-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}
