//go:build !windows && !linux

package media

// Start returns a no-op controller on platforms without a media-key backend.
func Start() (Controller, error) {
	return newNoop(), nil
}

func Available() string {
	return "not available on this platform"
}
