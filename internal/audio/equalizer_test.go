package audio

import (
	"math"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
)

func testSine(rate, freq, amp float64, total int) beep.Streamer {
	pos := 0
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n := 0
		for ; n < len(samples) && pos < total; n++ {
			v := amp * math.Sin(2*math.Pi*freq*float64(pos)/rate)
			samples[n][0], samples[n][1] = v, v
			pos++
		}
		return n, n > 0
	})
}

func testConstant(value float64, total int) beep.Streamer {
	pos := 0
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n := 0
		for ; n < len(samples) && pos < total; n++ {
			samples[n][0], samples[n][1] = value, value
			pos++
		}
		return n, n > 0
	})
}

func streamAll(t *testing.T, streamer beep.Streamer, skip int) []float64 {
	t.Helper()
	buf := make([][2]float64, 512)
	var out []float64
	seen := 0
	for {
		n, ok := streamer.Stream(buf)
		for i := 0; i < n; i++ {
			seen++
			if seen > skip {
				out = append(out, buf[i][0])
			}
		}
		if !ok || n == 0 {
			break
		}
	}
	return out
}

func rms(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range samples {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func TestEqualizerFlatIsBypass(t *testing.T) {
	source := testConstant(0.25, 2048)
	eq := newEqualizer(source, FlatEQ(), 48000)
	out := streamAll(t, eq, 0)
	if len(out) != 2048 {
		t.Fatalf("samples = %d, want 2048", len(out))
	}
	for i, v := range out {
		if v != 0.25 {
			t.Fatalf("sample %d = %v, want untouched 0.25", i, v)
		}
	}
	if eq.enabled {
		t.Fatal("flat equalizer should be bypassed")
	}
}

func TestEqualizerPreamp(t *testing.T) {
	eq := newEqualizer(testConstant(0.1, 2048), EQSettings{Preamp: 6}, 48000)
	out := streamAll(t, eq, 1024)
	if len(out) == 0 {
		t.Fatal("no output")
	}
	for i, v := range out {
		if math.Abs(v-0.2) > 0.001 {
			t.Fatalf("sample %d = %v, want 0.2", i, v)
		}
	}
}

func TestEqualizerBoostAtCenter(t *testing.T) {
	rate := 48000.0
	flat := newEqualizer(testSine(rate, 1000, 0.08, 48000), FlatEQ(), rate)
	boost := EQSettings{}
	boost.Gains[5] = 6 // 1 kHz
	boosted := newEqualizer(testSine(rate, 1000, 0.08, 48000), boost, rate)
	flatRMS := rms(streamAll(t, flat, 4800))
	boostRMS := rms(streamAll(t, boosted, 4800))
	if flatRMS == 0 {
		t.Fatal("flat RMS is zero")
	}
	ratio := boostRMS / flatRMS
	if ratio < 1.2 || ratio > 2.5 {
		t.Fatalf("boost ratio = %.3f, want roughly +6 dB", ratio)
	}
}

func TestEqualizerClampsOutput(t *testing.T) {
	settings := EQSettings{Preamp: 12}
	settings.Gains[3] = 12
	eq := newEqualizer(testConstant(0.5, 2048), settings, 48000)
	for _, v := range streamAll(t, eq, 512) {
		if v > 1.0001 || v < -1.0001 {
			t.Fatalf("sample out of range: %v", v)
		}
	}
}

func TestEqualizerHighBandAt44100(t *testing.T) {
	settings := EQSettings{}
	settings.Gains[9] = 6
	eq := newEqualizer(testConstant(0.1, 1024), settings, 44100)
	if !eq.filters[9].active {
		t.Fatal("16 kHz band is inactive at 44.1 kHz")
	}
	for _, v := range streamAll(t, eq, 512) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("invalid sample: %v", v)
		}
	}
}

func TestEQNormalizeAndPresets(t *testing.T) {
	settings := EQFromSlice([]float64{99, -99, 1, 2}, 40)
	if settings.Gains[0] != MaxEQGain || settings.Gains[1] != MinEQGain {
		t.Fatalf("gains were not clamped: %+v", settings.Gains)
	}
	if settings.Gains[2] != 1 || settings.Gains[3] != 2 || settings.Gains[4] != 0 {
		t.Fatalf("gains were not copied: %+v", settings.Gains)
	}
	if settings.Preamp != MaxEQPreamp {
		t.Fatalf("preamp = %v", settings.Preamp)
	}
	if MatchPreset(FlatEQ()) != "Flat" {
		t.Fatalf("Flat preset was not matched: %q", MatchPreset(FlatEQ()))
	}
	if MatchPreset(EqualizerPresets[1].Settings) != "Bass" {
		t.Fatalf("Bass preset was not matched: %q", MatchPreset(EqualizerPresets[1].Settings))
	}
	custom := EqualizerPresets[1].Settings
	custom.Gains[0] += 1
	if MatchPreset(custom) != "Custom" {
		t.Fatalf("custom settings matched a preset: %q", MatchPreset(custom))
	}
}

func TestNormalizeSpeed(t *testing.T) {
	cases := map[float64]float64{
		0:    1,
		-1:   1,
		0.49: MinSpeed,
		0.51: 0.5,
		1.24: 1.25,
		2:    2,
		3:    MaxSpeed,
	}
	for in, want := range cases {
		if got := NormalizeSpeed(in); got != want {
			t.Errorf("NormalizeSpeed(%v) = %v, want %v", in, got, want)
		}
	}
	if got := NormalizeSpeed(math.NaN()); got != 1 {
		t.Fatalf("NaN speed = %v, want 1", got)
	}
	if SpeedLabel(1) != "1x" || SpeedLabel(1.25) != "1.25x" || SpeedLabel(2) != "2x" {
		t.Fatalf("unexpected speed labels: %q %q %q", SpeedLabel(1), SpeedLabel(1.25), SpeedLabel(2))
	}
}

func TestResampleRatioSpeedsStream(t *testing.T) {
	const total = 48000
	count := func(ratio float64) int {
		source := testConstant(0.1, total)
		resampled := beep.ResampleRatio(4, ratio, source)
		buf := make([][2]float64, 512)
		got := 0
		for {
			n, ok := resampled.Stream(buf)
			got += n
			if !ok || n == 0 {
				break
			}
		}
		return got
	}
	double := count(2)
	half := count(0.5)
	if double < 23500 || double > 24500 {
		t.Fatalf("2x produced %d samples, want about 24000", double)
	}
	if half < 95500 || half > 96500 {
		t.Fatalf("0.5x produced %d samples, want about 96000", half)
	}
}

func TestWallRemaining(t *testing.T) {
	if got := wallRemaining(10*time.Second, 2); got != 5*time.Second {
		t.Fatalf("2x remaining = %v, want 5s", got)
	}
	if got := wallRemaining(10*time.Second, 0.5); got != 20*time.Second {
		t.Fatalf("0.5x remaining = %v, want 20s", got)
	}
	if got := wallRemaining(-1*time.Second, 1); got != 0 {
		t.Fatalf("negative remaining = %v, want 0", got)
	}
}

func TestEngineSpeedAndEQBeforeInit(t *testing.T) {
	engine := New(0.5)
	engine.SetSpeed(1.3)
	if got := engine.Speed(); got != 1.3 {
		t.Fatalf("speed = %v, want 1.3", got)
	}
	settings := EQSettings{}
	settings.Gains[0] = 6
	settings.Preamp = -3
	engine.SetEQ(settings)
	got := engine.EQ()
	if got.Gains[0] != 6 || got.Preamp != -3 {
		t.Fatalf("EQ = %+v", got)
	}
}
