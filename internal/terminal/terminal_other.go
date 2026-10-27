//go:build !windows

package terminal

func platformInfo() (elevated bool, fontFace string, fontSize int, outputCP uint32) {
	return false, "", 0, 0
}
