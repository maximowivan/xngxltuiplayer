// Package presence publishes the current track to Discord Rich Presence over
// the local Discord IPC socket.
package presence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config controls the Discord integration.
type Config struct {
	Enabled       bool   `json:"enabled"`
	ClientID      string `json:"client_id"`
	FallbackImage string `json:"fallback_image"`
	ActivityType  int    `json:"activity_type"`

	dataDir string
}

// Load reads data/discord.json. A missing or invalid file disables presence.
func Load(dataDir string) Config {
	cfg := Config{dataDir: dataDir}
	data, err := os.ReadFile(filepath.Join(dataDir, "discord.json"))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	cfg.dataDir = dataDir
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.FallbackImage = strings.TrimSpace(cfg.FallbackImage)
	if cfg.ActivityType <= 0 || cfg.ActivityType > 5 {
		cfg.ActivityType = 2
	}
	return cfg
}

// Save writes data/discord.json atomically.
func Save(dataDir string, cfg Config) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.FallbackImage = strings.TrimSpace(cfg.FallbackImage)
	if cfg.ActivityType <= 0 || cfg.ActivityType > 5 {
		cfg.ActivityType = 2
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, "discord.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

// Configured reports whether the integration can connect.
func (c Config) Configured() bool {
	return c.Enabled && validClientID(c.ClientID)
}

func validClientID(id string) bool {
	if len(id) < 15 || len(id) > 21 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c Config) activityType() int {
	if c.ActivityType <= 0 || c.ActivityType > 5 {
		return 2
	}
	return c.ActivityType
}
