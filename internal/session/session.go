package session

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"tuiplay/internal/atomicfile"
)

// State is the small amount of playback state persisted between runs.
const (
	eqBandCount = 10
	minSpeed    = 0.5
	maxSpeed    = 2.0
	speedStep   = 0.05
	minEQGain   = -12.0
	maxEQGain   = 12.0
)

type State struct {
	Volume     int       `json:"volume"`
	Repeat     string    `json:"repeat"`
	Shuffle    bool      `json:"shuffle"`
	View       string    `json:"view"`
	Theme      string    `json:"theme"`
	Sort       string    `json:"sort"`
	Track      string    `json:"track"`
	PositionMS int64     `json:"position_ms"`
	Speed      float64   `json:"speed"`
	EQ         []float64 `json:"eq"`
	EQPreamp   float64   `json:"eq_preamp"`
}

// Default returns the initial session state.
func Default() State {
	return State{
		Volume: 80,
		Repeat: "off",
		View:   "all",
		Sort:   "artist",
		Speed:  1,
		EQ:     make([]float64, eqBandCount),
	}
}

// Load reads the session file. A missing or invalid file yields defaults.
func Load(dataDir string) State {
	state := Default()
	path := filepath.Join(dataDir, "session.json")
	if err := atomicfile.ReadJSON(path, &state); err != nil {
		return Default()
	}
	normalize(&state)
	return state
}

// Save writes the session atomically.
func Save(dataDir string, state State) error {
	normalize(&state)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return atomicfile.WriteJSON(filepath.Join(dataDir, "session.json"), state)
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
	switch s.Sort {
	case "artist", "title", "album", "year", "recent":
	default:
		s.Sort = "artist"
	}
	if s.PositionMS < 0 {
		s.PositionMS = 0
	}

	if math.IsNaN(s.Speed) || math.IsInf(s.Speed, 0) || s.Speed <= 0 {
		s.Speed = 1
	}
	s.Speed = math.Round(clampFloat(s.Speed, minSpeed, maxSpeed)/speedStep) * speedStep
	s.Speed = math.Round(s.Speed*100) / 100

	eq := make([]float64, eqBandCount)
	for i := 0; i < eqBandCount && i < len(s.EQ); i++ {
		eq[i] = clampFloat(s.EQ[i], minEQGain, maxEQGain)
	}
	s.EQ = eq
	s.EQPreamp = math.Round(clampFloat(s.EQPreamp, minEQGain, maxEQGain))
}

func clampFloat(v, min, max float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
