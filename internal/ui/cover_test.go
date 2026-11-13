package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCoverModeShowsUpNextPanel(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36

	frame := m.renderCover(130, 36)
	for _, want := range []string{
		"UP NEXT",
		"Paper Planes",
		"Slow Motion",
		"Neon Skyline",
		"2024",
		"queue 1/3",
		"1x",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("cover frame is missing %q:\n%s", want, frame)
		}
	}
	for i, line := range strings.Split(frame, "\n") {
		if got := ansi.StringWidth(line); got != 130 {
			t.Fatalf("line %d width = %d, want 130\n%s", i, got, line)
		}
	}
}

func TestCoverModeNarrowFallsBackToSingleUpNextLine(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 60, 24

	frame := m.renderCover(60, 24)
	if strings.Contains(frame, "UP NEXT") {
		t.Fatalf("narrow cover should not show the UP NEXT panel:\n%s", frame)
	}
	if !strings.Contains(frame, "Up next:") || !strings.Contains(frame, "Paper Planes") {
		t.Fatalf("narrow cover is missing the next track line:\n%s", frame)
	}
	for i, line := range strings.Split(frame, "\n") {
		if got := ansi.StringWidth(line); got != 60 {
			t.Fatalf("line %d width = %d, want 60\n%s", i, got, line)
		}
	}
}

func TestCoverModeEmptyQueue(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.queue = nil
	m.queueIndex = 0
	m.width, m.height = 120, 32

	frame := m.renderCover(120, 32)
	if !strings.Contains(frame, "Queue is empty") {
		t.Fatalf("empty queue message missing:\n%s", frame)
	}
}

func TestCoverUpNextWrapsOnlyWithRepeatAll(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	last := m.tracks[len(m.tracks)-1]
	m.current = &last
	m.queueIndex = len(m.queue) - 1
	m.repeat = "off"

	if got := m.upNextTracks(5); len(got) != 0 {
		t.Fatalf("up next without repeat = %d tracks, want 0", len(got))
	}
	m.repeat = "all"
	got := m.upNextTracks(5)
	if len(got) != len(m.queue)-1 {
		t.Fatalf("up next with repeat all = %d tracks, want %d", len(got), len(m.queue)-1)
	}
	if got[0].Title != "Night Drive" {
		t.Fatalf("up next wrapped to %q, want Night Drive", got[0].Title)
	}
}

func TestCoverModeShowsShuffleBadge(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.shuffle = true
	m.repeat = "all"
	m.width, m.height = 130, 36

	frame := m.renderCover(130, 36)
	if !strings.Contains(frame, "shuffle") || !strings.Contains(frame, "repeat all") {
		t.Fatalf("shuffle/repeat badge missing:\n%s", frame)
	}
}
