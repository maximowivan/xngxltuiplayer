//go:build windows

package atomicfile

import (
	"golang.org/x/sys/windows"
)

// replace swaps dst for src. MoveFileEx with MOVEFILE_REPLACE_EXISTING is the
// closest Windows equivalent of rename(2): readers see either the old or the
// new file, never a truncated one.
func replace(src, dst string) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
