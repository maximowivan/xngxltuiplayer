package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// State is the small amount of playback state persisted between runs.
type State struct {
	Volume     int    `json:"volume"`
	Repeat     string `json:"repeat"`
	Shuffle    bool   `json:"shuffle"`
	View       string `json:"view"`
	Theme      string `json:"theme"`
	Track      string `json:"track"`
	PositionMS int64  `json:"position_ms"`
}

// Default returns the initial session state.
func Default() State {
	return State{Volume: 80, Repeat: "off", View: "all"}
}

// Load reads the session file. A missing or invalid file yields defaults.
func Load(dataDir string) State {
	state := Default()
	path := filepath.Join(dataDir, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return Default()
	}
	normalize(&state)
	return state
}

// Save writes the session atomically.
func Save(dataDir string, state State) error {
	normalize(&state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "session.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return errors.Join(err, err2)
		}
	}
	return nil
}

func normalize(s *State) {
	if s.Volume < 0 {
		s.Volume = 0
	}
	if s.Volume > 100 {
		s.Volume = 100
	}
	if s.Repeat != "off" && s.Repeat != "all" && s.Repeat != "one" {
		s.Repeat = "off"
	}
	s.View = strings.TrimSpace(s.View)
	if s.View == "" {
		s.View = "all"
	}
	s.Theme = strings.TrimSpace(s.Theme)
	if s.PositionMS < 0 {
		s.PositionMS = 0
	}
}
