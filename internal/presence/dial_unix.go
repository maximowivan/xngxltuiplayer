//go:build !windows

package presence

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func dialDiscordIPC() (net.Conn, error) {
	dirs := make([]string, 0, 8)
	for _, key := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if dir := os.Getenv(key); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	dirs = append(dirs, "/tmp")
	if uid := os.Getuid(); uid >= 0 {
		dirs = append(dirs,
			fmt.Sprintf("/run/user/%d", uid),
			fmt.Sprintf("/run/user/%d/snap.discord", uid),
			fmt.Sprintf("/run/user/%d/.flatpak/com.discordapp.Discord/xdg-run", uid),
		)
	}

	seen := make(map[string]bool)
	var lastErr error
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		for i := 0; i < 10; i++ {
			path := filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", i))
			conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("discord IPC socket not found")
	}
	return nil, lastErr
}
