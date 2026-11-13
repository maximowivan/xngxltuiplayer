package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
		"PLAYER",
		"Night Drive",
		"Aurora",
		"vol 80%",
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

func TestCoverModeNarrowHidesUpNextPanel(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 60, 24

	frame := m.renderCover(60, 24)
	if strings.Contains(frame, "UP NEXT") {
		t.Fatalf("narrow cover should not show the UP NEXT panel:\n%s", frame)
	}
	if !strings.Contains(frame, "PLAYER") || !strings.Contains(frame, "Night Drive") {
		t.Fatalf("narrow cover is missing the player card:\n%s", frame)
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
	if !strings.Contains(strings.ToLower(frame), "queue empty") {
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
	m.rebuildShuffleOrder(m.queueIndex)
	m.repeat = "all"
	m.width, m.height = 130, 36

	frame := m.renderCover(130, 36)
	if !strings.Contains(frame, "shuffle 1/3") || !strings.Contains(frame, "vol 80%") {
		t.Fatalf("shuffle/volume badge missing:\n%s", frame)
	}
}

func TestCoverPauseButtonClick(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.coverMode = true
	m.width, m.height = 130, 36
	m.ended = true

	cardLeft, cardTop, cardW, _ := m.coverCardGeometry(130, 36)
	layout := m.coverControlsLine(cardW - 4)
	x := cardLeft + 2 + layout.playStart
	y := cardTop + 4
	m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.ended {
		t.Fatal("clicking the pause button did not reach togglePlay")
	}
	if m.coverMode != true {
		t.Fatal("clicking the player card should not close cover mode")
	}
}

func TestCoverUpNextFollowsShuffleOrder(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.shuffle = true
	m.rebuildShuffleOrder(0)
	order := append([]int(nil), m.shuffleOrder...)
	if len(order) != len(m.queue) {
		t.Fatalf("shuffle order = %v", order)
	}

	if next := m.nextIndex(); next != order[1] {
		t.Fatalf("nextIndex = %d, want shuffle order index %d", next, order[1])
	}

	second := m.tracks[order[1]]
	m.current = &second
	m.queueIndex = order[1]
	m.shufflePos = 1
	up := m.upNextTracks(1)
	if len(up) != 1 || up[0].Title != m.tracks[order[2]].Title {
		t.Fatalf("up next after shuffle = %+v, want %q", up, m.tracks[order[2]].Title)
	}

	last := m.tracks[order[len(order)-1]]
	m.current = &last
	m.queueIndex = order[len(order)-1]
	m.shufflePos = len(order) - 1
	m.repeat = "off"
	if next := m.nextIndex(); next != -1 {
		t.Fatalf("nextIndex at the end = %d, want -1", next)
	}
	if prev := m.prevIndex(); prev != order[len(order)-2] {
		t.Fatalf("prevIndex = %d, want %d", prev, order[len(order)-2])
	}
	m.repeat = "all"
	if next := m.nextIndex(); next != order[0] {
		t.Fatalf("wrapped nextIndex = %d, want %d", next, order[0])
	}
}
