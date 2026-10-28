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
	// `go run` builds the binary under .../go-buildXXXX/b001/exe/main.exe.
	// Require the bNNN/exe tail so a portable folder that happens to be named
	// "go-build" keeps its own music/data next to the executable.
	lower := strings.ToLower(dir)
	if !strings.Contains(lower, "go-build") {
		return false
	}
	if strings.ToLower(filepath.Base(dir)) != "exe" {
		return false
	}
	build := strings.ToLower(filepath.Base(filepath.Dir(dir)))
	if len(build) < 2 || build[0] != 'b' {
		return false
	}
	for _, r := range build[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
