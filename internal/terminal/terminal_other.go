//go:build !windows

package terminal

func platformInfo() (elevated bool, fontFace string, fontSize int, outputCP uint32, consoleClass string) {
	return false, "", 0, 0, ""
}

func platformVT() bool {
	return true
}
