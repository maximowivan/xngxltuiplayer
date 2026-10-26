package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeWAV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	if err := os.WriteFile(path, testWAV(0.25, 440), 0o644); err != nil {
		t.Fatal(err)
	}
	streamer, format, err := decodeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer streamer.Close()
	if format.SampleRate != SampleRate {
		t.Fatalf("sample rate = %v, want %v", format.SampleRate, SampleRate)
	}
	if format.NumChannels != 2 {
		t.Fatalf("channels = %d, want 2", format.NumChannels)
	}
	if got := format.SampleRate.D(streamer.Len()); got < 240*time.Millisecond || got > 260*time.Millisecond {
		t.Fatalf("duration = %v", got)
	}
}

func TestSpectrumPeaksAtSineFrequency(t *testing.T) {
	const n = fftSize
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 1000 * float64(i) / float64(SampleRate))
	}
	levels := spectrumFromSamples(samples, 32)
	if len(levels) != 32 {
		t.Fatalf("got %d bars", len(levels))
	}
	best := 0
	for i, v := range levels {
		if v > levels[best] {
			best = i
		}
	}
	if best < 15 || best > 18 {
		t.Fatalf("peak bar = %d, want 15..18 (levels: %v)", best, levels)
	}
	if levels[best] < 0.4 {
		t.Fatalf("peak too quiet: %f", levels[best])
	}
}

func TestSpectrumSilence(t *testing.T) {
	levels := spectrumFromSamples(make([]float64, fftSize), 16)
	for i, v := range levels {
		if v != 0 {
			t.Fatalf("bar %d = %f, want 0", i, v)
		}
	}
}

func testWAV(seconds, freq float64) []byte {
	const sampleRate = 48000
	samples := int(seconds * sampleRate)
	dataSize := samples * 2 * 2

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for i := 0; i < samples; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/sampleRate) * 12000)
		_ = binary.Write(&buf, binary.LittleEndian, v)
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}
