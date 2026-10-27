//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

func startHidden(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd.Start()
}
