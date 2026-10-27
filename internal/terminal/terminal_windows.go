//go:build windows

package terminal

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type consoleFontInfoEx struct {
	CbSize      uint32
	NFont       uint32
	DwFontSizeX int16
	DwFontSizeY int16
	FontFamily  uint32
	FontWeight  uint32
	FaceName    [32]uint16
}

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procGetCurrentConsoleFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")
	procGetConsoleWindow        = kernel32.NewProc("GetConsoleWindow")
	procGetClassNameW           = user32.NewProc("GetClassNameW")
)

func platformInfo() (elevated bool, fontFace string, fontSize int, outputCP uint32, consoleClass string) {
	elevated = windows.GetCurrentProcessToken().IsElevated()
	outputCP, _ = windows.GetConsoleOutputCP()
	fontFace, fontSize = currentConsoleFont()
	consoleClass = currentConsoleWindowClass()
	return
}

// platformVT reports whether the current output console accepts VT escape
// sequences. It tries to enable the flag and restores the original mode so a
// preflight check does not change the user's console.
func platformVT() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return false
	}
	_ = windows.SetConsoleMode(handle, mode)
	return true
}

func currentConsoleWindowClass() string {
	if err := procGetConsoleWindow.Find(); err != nil {
		return ""
	}
	handle, _, _ := procGetConsoleWindow.Call()
	if handle == 0 {
		return ""
	}
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(
		handle,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func currentConsoleFont() (string, int) {
	if err := procGetCurrentConsoleFontEx.Find(); err != nil {
		return "", 0
	}
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return "", 0
	}
	info := consoleFontInfoEx{CbSize: uint32(unsafe.Sizeof(consoleFontInfoEx{}))}
	r, _, _ := procGetCurrentConsoleFontEx.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&info)),
	)
	if r == 0 {
		return "", 0
	}
	return windows.UTF16ToString(info.FaceName[:]), int(info.DwFontSizeY)
}
