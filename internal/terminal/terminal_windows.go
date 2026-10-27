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
	procGetCurrentConsoleFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")
)

func platformInfo() (elevated bool, fontFace string, fontSize int, outputCP uint32) {
	elevated = windows.GetCurrentProcessToken().IsElevated()
	outputCP, _ = windows.GetConsoleOutputCP()
	fontFace, fontSize = currentConsoleFont()
	return
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
