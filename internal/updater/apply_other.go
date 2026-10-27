//go:build !windows

package updater

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// Apply replaces the current binary on Unix-like systems. The running image
// stays mapped, so renaming over it is safe.
func Apply(stageDir, installDir string) error {
	src := filepath.Join(stageDir, "xngxltuiplayer")
	if _, err := os.Stat(src); err != nil {
		return errors.New("staged binary not found")
	}
	target := filepath.Join(installDir, "xngxltuiplayer")
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(src, target); err != nil {
		_ = os.Rename(old, target)
		return err
	}
	_ = os.Remove(old)
	return nil
}

func startHidden(cmd *exec.Cmd) error {
	return cmd.Start()
}
