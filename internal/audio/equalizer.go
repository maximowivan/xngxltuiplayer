package audio

import (
	"math"
	"strconv"
	"strings"

	"github.com/gopxl/beep/v2"
)

const (
	// EQBandCount is the number of graphic equalizer bands.
	EQBandCount = 10
	// MinEQGain and MaxEQGain bound each band in decibels.
	MinEQGain = -12.0
	MaxEQGain = 12.0
	// MinEQPreamp and MaxEQPreamp bound the preamp in decibels.
	MinEQPreamp = -12.0
	MaxEQPreamp = 12.0

	eqQ = 1.4
)

// EQBands are the center frequencies of the ten graphic EQ bands.
var EQBands = [EQBandCount]float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// EQSettings is the complete equalizer configuration.
type EQSettings struct {
	Gains  [EQBandCount]float64
	Preamp float64
}

// FlatEQ returns a bypassed equalizer.
func FlatEQ() EQSettings {
	return EQSettings{}
}

// EQFromSlice converts a stored gain slice into a normalized EQ settings
// value. Missing or extra entries are ignored.
func EQFromSlice(gains []float64, preamp float64) EQSettings {
	var out EQSettings
	for i := 0; i < EQBandCount && i < len(gains); i++ {
		out.Gains[i] = gains[i]
	}
	out.Preamp = preamp
	return out.Normalize()
}

// GainsSlice returns the band gains as a copy for JSON/session storage.
func (s EQSettings) GainsSlice() []float64 {
	out := make([]float64, EQBandCount)
	copy(out[:], s.Gains[:])
	return out
}

// Normalize clamps gains and preamp into their supported ranges.
func (s EQSettings) Normalize() EQSettings {
	for i := range s.Gains {
		s.Gains[i] = clampEQ(s.Gains[i], MinEQGain, MaxEQGain)
		if math.Abs(s.Gains[i]) < 0.05 {
			s.Gains[i] = 0
		}
	}
	s.Preamp = clampEQ(s.Preamp, MinEQPreamp, MaxEQPreamp)
	if math.Abs(s.Preamp) < 0.05 {
		s.Preamp = 0
	}
	return s
}

func clampEQ(v, min, max float64) float64 {
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

// EQPreset is a named equalizer configuration.
type EQPreset struct {
	Name     string
	Settings EQSettings
}

// EqualizerPresets are the built-in presets. They are intentionally mild and
// include negative preamp values so boosted presets do not clip immediately.
var EqualizerPresets = []EQPreset{
	{
		Name:     "Flat",
		Settings: FlatEQ(),
	},
	{
		Name: "Bass",
		Settings: EQSettings{
			Gains:  [EQBandCount]float64{6, 5, 3, 1, 0, 0, 0, 0, 0, 0},
			Preamp: -3,
		},
	},
	{
		Name: "Vocal",
		Settings: EQSettings{
			Gains:  [EQBandCount]float64{-2, -1, 0, 1, 3, 4, 4, 2, 0, -1},
			Preamp: -2,
		},
	},
	{
		Name: "Electronic",
		Settings: EQSettings{
			Gains:  [EQBandCount]float64{5, 4, 1, -1, 2, 3, 2, 3, 2, 1},
			Preamp: -4,
		},
	},
}

// MatchPreset returns the preset name matching settings, or "Custom".
func MatchPreset(settings EQSettings) string {
	settings = settings.Normalize()
	for _, preset := range EqualizerPresets {
		candidate := preset.Settings.Normalize()
		match := math.Abs(candidate.Preamp-settings.Preamp) < 0.05
		if !match {
			continue
		}
		for i := 0; i < EQBandCount; i++ {
			if math.Abs(candidate.Gains[i]-settings.Gains[i]) >= 0.05 {
				match = false
				break
			}
		}
		if match {
			return preset.Name
		}
	}
	return "Custom"
}

// equalizer is a ten-band peaking EQ streamer. It keeps per-channel filter
// state and can be reconfigured while streaming.
type equalizer struct {
	beep.Streamer
	filters [EQBandCount]biquad
	preamp  float64
	enabled bool
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	z1, z2     [2]float64
	active     bool
}

func newEqualizer(streamer beep.Streamer, settings EQSettings, sampleRate float64) *equalizer {
	eq := &equalizer{Streamer: streamer}
	eq.set(settings, sampleRate)
	return eq
}

func (e *equalizer) set(settings EQSettings, sampleRate float64) {
	settings = settings.Normalize()
	if sampleRate <= 0 {
		sampleRate = float64(SampleRate)
	}
	e.enabled = math.Abs(settings.Preamp) > 0.001
	e.preamp = math.Pow(10, settings.Preamp/20)
	for i := range e.filters {
		active := math.Abs(settings.Gains[i]) > 0.001
		if active {
			e.enabled = true
		}
		e.filters[i].setPeaking(EQBands[i], settings.Gains[i], sampleRate, active)
	}
}

func (e *equalizer) Stream(samples [][2]float64) (int, bool) {
	n, ok := e.Streamer.Stream(samples)
	if !e.enabled {
		return n, ok
	}
	for i := 0; i < n; i++ {
		for channel := 0; channel < 2; channel++ {
			x := samples[i][channel] * e.preamp
			for b := range e.filters {
				if e.filters[b].active {
					x = e.filters[b].process(channel, x)
				}
			}
			if x > 1 {
				x = 1
			} else if x < -1 {
				x = -1
			}
			samples[i][channel] = x
		}
	}
	return n, ok
}

func (b *biquad) setPeaking(freq, gainDB, sampleRate float64, active bool) {
	if !active || freq <= 0 || freq >= sampleRate/2 {
		b.b0, b.b1, b.b2 = 1, 0, 0
		b.a1, b.a2 = 0, 0
		b.active = false
		return
	}
	A := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * freq / sampleRate
	alpha := math.Sin(w0) / (2 * eqQ)
	cosW := math.Cos(w0)

	b0 := 1 + alpha*A
	b1 := -2 * cosW
	b2 := 1 - alpha*A
	a0 := 1 + alpha/A
	a1 := -2 * cosW
	a2 := 1 - alpha/A

	b.b0 = b0 / a0
	b.b1 = b1 / a0
	b.b2 = b2 / a0
	b.a1 = a1 / a0
	b.a2 = a2 / a0
	b.active = true
}

func (b *biquad) process(channel int, x float64) float64 {
	y := b.b0*x + b.z1[channel]
	b.z1[channel] = b.b1*x - b.a1*y + b.z2[channel]
	b.z2[channel] = b.b2*x - b.a2*y
	return y
}

// Speed constants and normalization. Speed is a tape-style rate: 2.0 plays
// twice as fast and raises pitch, 0.5 plays half speed and lowers it.
const (
	MinSpeed  = 0.5
	MaxSpeed  = 2.0
	SpeedStep = 0.05
)

// NormalizeSpeed clamps and rounds a speed value to the supported step.
func NormalizeSpeed(speed float64) float64 {
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed <= 0 {
		return 1
	}
	speed = math.Round(speed/SpeedStep) * SpeedStep
	if speed < MinSpeed {
		return MinSpeed
	}
	if speed > MaxSpeed {
		return MaxSpeed
	}
	return speed
}

// SpeedLabel formats a speed as "1.25x".
func SpeedLabel(speed float64) string {
	text := strconv.FormatFloat(NormalizeSpeed(speed), 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		text = "1"
	}
	return text + "x"
}
