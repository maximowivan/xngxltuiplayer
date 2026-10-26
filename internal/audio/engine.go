// Package audio implements decoding, playback and spectrum analysis.
package audio

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/gopxl/beep/v2/wav"
)

// SampleRate is the fixed output rate used by the speaker.
const SampleRate = beep.SampleRate(48000)

// bufferSamples is roughly 100 ms of audio. It keeps playback stable while
// staying responsive enough for seeking.
const bufferSamples = 4800

// EventKind describes an asynchronous player event.
type EventKind int

const (
	// EventDone is emitted when the current track reaches its end.
	EventDone EventKind = iota
	// EventError is reserved for future use.
	EventError
)

// Event is sent from the audio goroutine to the UI.
type Event struct {
	Kind EventKind
	Gen  uint64
	Path string
	Err  error
}

// trackStream is one decoded track with its own fade, volume and pause
// controls. The speaker mixes several of these streams during a crossfade.
type trackStream struct {
	base     beep.StreamSeekCloser
	counter  *countingStreamer
	tap      *spectrumTap
	fade     *fadeStreamer
	ctrl     *beep.Ctrl
	vol      *effects.Volume
	path     string
	gen      uint64
	duration time.Duration

	announce  atomic.Bool
	closeOnce sync.Once
	closed    atomic.Bool
}

func (t *trackStream) close() {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		_ = t.base.Close()
	})
}

// Engine owns the speaker and the currently mixed tracks.
type Engine struct {
	mu       sync.Mutex
	initErr  error
	initOnce bool
	volume   float64

	tracks   []*trackStream
	active   *trackStream
	path     string
	duration time.Duration
	offset   time.Duration
	paused   bool
	gen      uint64
	events   chan Event
}

// New creates a player with the given volume (0..1). The audio device is
// opened lazily on the first Play call so the UI can start without a device.
func New(volume float64) *Engine {
	return &Engine{
		volume: clamp01(volume),
		events: make(chan Event, 32),
	}
}

// Events returns the asynchronous player event channel.
func (e *Engine) Events() <-chan Event { return e.events }

func (e *Engine) ensureInitLocked() error {
	if e.initOnce {
		return e.initErr
	}
	e.initOnce = true
	e.initErr = speaker.Init(SampleRate, bufferSamples)
	return e.initErr
}

// Play starts a track at start, replacing anything currently playing.
func (e *Engine) Play(path string, duration, start time.Duration) error {
	return e.play(path, duration, start, false)
}

func (e *Engine) play(path string, duration, start time.Duration, paused bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureInitLocked(); err != nil {
		return fmt.Errorf("audio device: %w", err)
	}
	if duration > 0 && start >= duration {
		start = 0
	}
	if start < 0 {
		start = 0
	}

	ts, actualStart, err := e.newTrackLocked(path, duration, start, 1)
	if err != nil {
		return err
	}

	e.stopLocked()
	e.gen++
	ts.gen = e.gen
	ts.ctrl.Paused = paused
	ts.announce.Store(true)
	e.tracks = append(e.tracks, ts)
	e.active = ts
	e.path = path
	e.duration = ts.duration
	e.offset = actualStart
	e.paused = paused
	speaker.Play(beep.Seq(ts.ctrl, beep.Callback(func() {
		e.finishTrack(ts)
	})))
	return nil
}

// CrossfadeTo starts path at start and fades the old track out while the new
// one fades in. It is safe to call while the old track is still playing.
func (e *Engine) CrossfadeTo(path string, duration, start, fade time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureInitLocked(); err != nil {
		return fmt.Errorf("audio device: %w", err)
	}
	if e.active == nil {
		return errors.New("nothing is playing")
	}
	if fade <= 0 {
		return errors.New("crossfade duration must be positive")
	}

	remaining := time.Duration(0)
	if e.duration > 0 {
		remaining = e.duration - e.positionLocked()
	}
	if remaining <= 0 {
		return errors.New("track already ended")
	}
	if fade > remaining {
		fade = remaining
	}
	if fade < 100*time.Millisecond {
		return errors.New("too close to track end")
	}

	ts, actualStart, err := e.newTrackLocked(path, duration, start, 0)
	if err != nil {
		return err
	}
	fadeSamples := int64(SampleRate.N(fade))
	if fadeSamples < 1 {
		ts.close()
		return errors.New("crossfade is too short")
	}

	e.gen++
	ts.gen = e.gen
	ts.ctrl.Paused = e.paused
	ts.announce.Store(true)
	ts.fade.fadeTo(1, fadeSamples)

	speaker.Lock()
	old := e.active
	old.announce.Store(false)
	old.fade.fadeTo(0, fadeSamples)
	speaker.Unlock()

	e.pruneLocked()
	e.tracks = append(e.tracks, ts)
	e.active = ts
	e.path = path
	e.duration = ts.duration
	e.offset = actualStart
	speaker.Play(beep.Seq(ts.ctrl, beep.Callback(func() {
		e.finishTrack(ts)
	})))
	return nil
}

func (e *Engine) newTrackLocked(path string, duration, start time.Duration, initialGain float64) (*trackStream, time.Duration, error) {
	if duration > 0 && start >= duration {
		start = 0
	}
	if start < 0 {
		start = 0
	}

	base, format, err := decodeFile(path)
	if err != nil {
		return nil, start, err
	}
	if duration <= 0 && format.SampleRate > 0 {
		duration = format.SampleRate.D(base.Len())
		if duration > 0 && start >= duration {
			// A stale session position from a shorter file should not start at
			// the very end.
			start = 0
		}
	}

	startSample := int(float64(start) / float64(time.Second) * float64(format.SampleRate))
	if startSample > 0 {
		if startSample >= base.Len() {
			startSample = base.Len() - 1
		}
		if startSample < 0 {
			startSample = 0
		}
		if err := base.Seek(startSample); err != nil {
			_ = base.Close()
			return nil, start, fmt.Errorf("seek audio: %w", err)
		}
		start = format.SampleRate.D(startSample)
	}

	var streamer beep.Streamer = base
	if format.SampleRate != SampleRate {
		streamer = beep.Resample(4, format.SampleRate, SampleRate, base)
	}

	counter := &countingStreamer{Streamer: streamer}
	tap := newSpectrumTap(counter, 4096)
	fade := &fadeStreamer{Streamer: tap, gain: initialGain}
	vol := &effects.Volume{
		Streamer: fade,
		Base:     2,
		Volume:   gainToVolume(e.volume),
		Silent:   e.volume <= 0.001,
	}
	ctrl := &beep.Ctrl{Streamer: vol}

	return &trackStream{
		base:     base,
		counter:  counter,
		tap:      tap,
		fade:     fade,
		ctrl:     ctrl,
		vol:      vol,
		path:     path,
		duration: duration,
	}, start, nil
}

func (e *Engine) finishTrack(ts *trackStream) {
	ts.close()
	if !ts.announce.Load() {
		return
	}
	select {
	case e.events <- Event{Kind: EventDone, Gen: ts.gen, Path: ts.path}:
	default:
	}
}

func (e *Engine) pruneLocked() {
	kept := e.tracks[:0]
	for _, ts := range e.tracks {
		if ts.closed.Load() && ts != e.active {
			continue
		}
		kept = append(kept, ts)
	}
	e.tracks = kept
}

func (e *Engine) stopLocked() {
	e.gen++
	speaker.Clear()
	for _, ts := range e.tracks {
		ts.close()
	}
	e.tracks = nil
	e.active = nil
	e.path = ""
	e.duration = 0
	e.offset = 0
	e.paused = false
}

// Stop stops playback and releases every active file.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
}

// Close stops playback and closes the speaker.
func (e *Engine) Close() {
	e.Stop()
	speaker.Close()
}

// TogglePause flips the paused state. It returns the new paused state.
func (e *Engine) TogglePause() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active == nil {
		return e.paused
	}
	e.paused = !e.paused
	speaker.Lock()
	for _, ts := range e.tracks {
		ts.ctrl.Paused = e.paused
	}
	speaker.Unlock()
	return e.paused
}

// SetPaused explicitly sets the paused state.
func (e *Engine) SetPaused(paused bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.paused = paused
	speaker.Lock()
	for _, ts := range e.tracks {
		ts.ctrl.Paused = paused
	}
	speaker.Unlock()
}

// Paused reports whether playback is paused.
func (e *Engine) Paused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

// Active reports whether a track is loaded.
func (e *Engine) Active() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active != nil
}

// Path returns the loaded track path.
func (e *Engine) Path() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.path
}

// Gen returns the current playback generation.
func (e *Engine) Gen() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gen
}

// Duration returns the duration passed to Play.
func (e *Engine) Duration() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.duration
}

// Position returns the current playback position.
func (e *Engine) Position() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.positionLocked()
}

func (e *Engine) positionLocked() time.Duration {
	if e.active == nil {
		return e.offset
	}
	pos := e.offset + SampleRate.D(int(e.active.counter.n.Load()))
	if e.duration > 0 && pos > e.duration {
		pos = e.duration
	}
	return pos
}

// SeekTo restarts the current track at pos, preserving the paused state.
func (e *Engine) SeekTo(pos time.Duration) error {
	e.mu.Lock()
	path, duration, paused := e.path, e.duration, e.paused
	e.mu.Unlock()
	if path == "" {
		return errors.New("nothing is loaded")
	}
	return e.play(path, duration, pos, paused)
}

// SeekRelative seeks by delta within the current track.
func (e *Engine) SeekRelative(delta time.Duration) error {
	pos := e.Position() + delta
	if pos < 0 {
		pos = 0
	}
	dur := e.Duration()
	if dur > 0 && pos > dur {
		pos = dur
	}
	return e.SeekTo(pos)
}

// SetVolume sets output volume, where 0 is mute and 1 is unity gain.
func (e *Engine) SetVolume(volume float64) {
	volume = clamp01(volume)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.volume = volume
	speaker.Lock()
	for _, ts := range e.tracks {
		ts.vol.Volume = gainToVolume(volume)
		ts.vol.Silent = volume <= 0.001
	}
	speaker.Unlock()
}

// Volume returns the current volume.
func (e *Engine) Volume() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.volume
}

// Err returns any streaming error from the current track.
func (e *Engine) Err() error {
	e.mu.Lock()
	tap := (*spectrumTap)(nil)
	if e.active != nil {
		tap = e.active.tap
	}
	e.mu.Unlock()
	if tap == nil {
		return nil
	}
	return tap.Err()
}

// Spectrum returns normalized FFT levels for the requested number of bars.
func (e *Engine) Spectrum(bars int) []float64 {
	e.mu.Lock()
	tap := (*spectrumTap)(nil)
	if e.active != nil {
		tap = e.active.tap
	}
	e.mu.Unlock()
	if tap == nil {
		return make([]float64, maxInt(bars, 0))
	}
	samples := tap.Snapshot(fftSize)
	return spectrumFromSamples(samples, bars)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func gainToVolume(volume float64) float64 {
	if volume <= 0.001 {
		return -60
	}
	// gain = volume^2, expressed in base-2 logarithmic units.
	return 2 * math.Log2(volume)
}

func decodeFile(path string) (beep.StreamSeekCloser, beep.Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("open audio: %w", err)
	}
	var (
		streamer beep.StreamSeekCloser
		format   beep.Format
	)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		streamer, format, err = mp3.Decode(f)
	case ".flac":
		streamer, format, err = flac.Decode(f)
	case ".wav":
		streamer, format, err = wav.Decode(f)
	default:
		_ = f.Close()
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format %q", filepath.Ext(path))
	}
	if err != nil {
		_ = f.Close()
		return nil, beep.Format{}, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return streamer, format, nil
}

type fadeShape int

const (
	fadeLinear fadeShape = iota
	fadeOutEqualPower
	fadeInEqualPower
)

// fadeStreamer applies an equal-power gain ramp to a track. A gain ramp is
// only active during a crossfade; normal tracks keep gain 1.
type fadeStreamer struct {
	beep.Streamer
	gain      float64
	from      float64
	to        float64
	remaining int64
	total     int64
	shape     fadeShape
}

func (f *fadeStreamer) fadeTo(target float64, samples int64) {
	if samples <= 0 {
		f.gain = target
		f.from = target
		f.to = target
		f.remaining = 0
		f.total = 0
		f.shape = fadeLinear
		return
	}
	f.from = f.gain
	f.to = target
	f.remaining = samples
	f.total = samples
	switch {
	case target < f.gain:
		f.shape = fadeOutEqualPower
	case target > f.gain:
		f.shape = fadeInEqualPower
	default:
		f.shape = fadeLinear
	}
}

func (f *fadeStreamer) Stream(samples [][2]float64) (int, bool) {
	n, ok := f.Streamer.Stream(samples)
	for i := 0; i < n; i++ {
		if f.remaining > 0 {
			f.remaining--
			progress := 1.0
			if f.total > 0 {
				progress = 1 - float64(f.remaining)/float64(f.total)
			}
			switch f.shape {
			case fadeOutEqualPower:
				f.gain = f.from * math.Cos(progress*math.Pi/2)
			case fadeInEqualPower:
				f.gain = f.to * math.Sin(progress*math.Pi/2)
			default:
				f.gain = f.from + (f.to-f.from)*progress
			}
			if f.remaining == 0 {
				f.gain = f.to
			}
		}
		samples[i][0] *= f.gain
		samples[i][1] *= f.gain
	}
	return n, ok
}

type countingStreamer struct {
	beep.Streamer
	n atomic.Uint64
}

func (c *countingStreamer) Stream(samples [][2]float64) (int, bool) {
	n, ok := c.Streamer.Stream(samples)
	if n > 0 {
		c.n.Add(uint64(n))
	}
	return n, ok
}

type spectrumTap struct {
	beep.Streamer
	mu     sync.Mutex
	buf    []float64
	pos    int
	filled int
}

func newSpectrumTap(streamer beep.Streamer, size int) *spectrumTap {
	return &spectrumTap{Streamer: streamer, buf: make([]float64, size)}
}

func (t *spectrumTap) Stream(samples [][2]float64) (int, bool) {
	n, ok := t.Streamer.Stream(samples)
	if n > 0 {
		t.mu.Lock()
		for i := 0; i < n; i++ {
			t.buf[t.pos] = (samples[i][0] + samples[i][1]) * 0.5
			t.pos = (t.pos + 1) % len(t.buf)
			if t.filled < len(t.buf) {
				t.filled++
			}
		}
		t.mu.Unlock()
	}
	return n, ok
}

func (t *spectrumTap) Snapshot(n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	t.mu.Lock()
	defer t.mu.Unlock()
	count := t.filled
	if count > n {
		count = n
	}
	start := t.pos - count
	for i := 0; i < count; i++ {
		idx := start + i
		if idx < 0 {
			idx += len(t.buf)
		}
		out[n-count+i] = t.buf[idx]
	}
	return out
}

// Check opens and closes the audio device. It is used by --check and must only
// be called in a process that does not create an Engine.
func Check() error {
	if err := speaker.Init(SampleRate, bufferSamples); err != nil {
		return err
	}
	speaker.Close()
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
