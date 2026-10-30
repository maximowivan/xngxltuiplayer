package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/audio"
	"tuiplay/internal/session"
)

func TestSpeedControlsClampAndReset(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	if m.speed != 1 {
		t.Fatalf("initial speed = %v, want 1", m.speed)
	}
	m.changeSpeed(audio.SpeedStep)
	if m.speed != 1.05 || m.engine.Speed() != 1.05 {
		t.Fatalf("speed = %v engine = %v, want 1.05", m.speed, m.engine.Speed())
	}
	for i := 0; i < 40; i++ {
		m.changeSpeed(audio.SpeedStep)
	}
	if m.speed != audio.MaxSpeed {
		t.Fatalf("max speed = %v, want %v", m.speed, audio.MaxSpeed)
	}
	for i := 0; i < 80; i++ {
		m.changeSpeed(-audio.SpeedStep)
	}
	if m.speed != audio.MinSpeed {
		t.Fatalf("min speed = %v, want %v", m.speed, audio.MinSpeed)
	}
	m.resetSpeed()
	if m.speed != 1 || m.engine.Speed() != 1 {
		t.Fatalf("reset speed = %v engine = %v, want 1", m.speed, m.engine.Speed())
	}
}

func TestEffectShortcuts(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.modal = modalNone
	m.searching = false

	send := func(text string) {
		t.Helper()
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	}
	send("]")
	if m.speed != 1.05 {
		t.Fatalf("] speed = %v, want 1.05", m.speed)
	}
	send("[")
	if m.speed != 1 {
		t.Fatalf("[ speed = %v, want 1", m.speed)
	}
	send("]")
	send("]")
	send("\\")
	if m.speed != 1 {
		t.Fatalf("\\ speed = %v, want 1", m.speed)
	}
	send("Q")
	if m.modal != modalEQ {
		t.Fatalf("Q modal = %v, want modalEQ", m.modal)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.modal != modalNone {
		t.Fatalf("Esc left modal %v open", m.modal)
	}
}

func TestEQModalAdjustments(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.openEQ()
	if m.modal != modalEQ {
		t.Fatalf("modal = %v, want modalEQ", m.modal)
	}
	m.eqCursor = 0
	m.adjustEQBand(6)
	if m.eq.Gains[0] != 6 {
		t.Fatalf("band 0 = %v, want 6", m.eq.Gains[0])
	}
	if got := m.engine.EQ(); got.Gains[0] != 6 {
		t.Fatalf("engine band 0 = %v, want 6", got.Gains[0])
	}
	m.eqCursor = 5
	m.adjustEQBand(-3)
	if m.eq.Gains[5] != -3 {
		t.Fatalf("band 5 = %v, want -3", m.eq.Gains[5])
	}
	m.resetEQBand()
	if m.eq.Gains[5] != 0 {
		t.Fatalf("band 5 after reset = %v", m.eq.Gains[5])
	}
	m.adjustEQPreamp(-2)
	if m.eq.Preamp != -2 {
		t.Fatalf("preamp = %v, want -2", m.eq.Preamp)
	}

	if !m.setEQPreset(1) {
		t.Fatal("Bass preset was rejected")
	}
	if audio.MatchPreset(m.eq) != "Bass" {
		t.Fatalf("preset = %q, want Bass", audio.MatchPreset(m.eq))
	}
	m.adjustEQBand(1)
	if audio.MatchPreset(m.eq) != "Custom" {
		t.Fatalf("manual edit kept preset %q", audio.MatchPreset(m.eq))
	}
}

func TestEQModalRenders(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.modal = modalEQ
	m.eq = audio.EQSettings{}
	m.eq.Gains[0] = 6
	m.eq.Gains[5] = -3
	m.eq.Preamp = -2

	for _, size := range [][2]int{{110, 32}, {84, 24}, {60, 18}} {
		frame := m.renderEQModal(size[0], size[1])
		if !strings.Contains(frame, "EQUALIZER") {
			t.Fatalf("%dx%d: missing title:\n%s", size[0], size[1], frame)
		}
		if !strings.Contains(frame, "31") {
			t.Fatalf("%dx%d: missing first band:\n%s", size[0], size[1], frame)
		}
		if !strings.Contains(frame, "Esc close") {
			t.Fatalf("%dx%d: missing footer:\n%s", size[0], size[1], frame)
		}
	}
}

func TestSaveSessionPersistsSpeedAndEQ(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.speed = 1.25
	m.eq.Gains[2] = 4
	m.eq.Preamp = -2
	m.saveSession()

	got := session.Load(m.cfg.DataDir)
	if got.Speed != 1.25 {
		t.Fatalf("saved speed = %v", got.Speed)
	}
	if len(got.EQ) != audio.EQBandCount || got.EQ[2] != 4 {
		t.Fatalf("saved EQ = %+v", got.EQ)
	}
	if got.EQPreamp != -2 {
		t.Fatalf("saved preamp = %v", got.EQPreamp)
	}
}

func TestPlayerShowsSpeedBadge(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.speed = 1.25
	m.width, m.height = 120, 32
	frame := m.renderPlayer(120)
	if !strings.Contains(frame, "1.25x") {
		t.Fatalf("player does not show speed badge:\n%s", frame)
	}
}
