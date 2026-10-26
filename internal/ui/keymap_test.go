package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func teaKeyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestNormalizeShortcut(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ы", "s"},
		{"к", "r"},
		{"З", "P"},
		{"Ч", "X"},
		{"В", "D"},
		{"Е", "T"},
		{"о", "j"},
		{"л", "k"},
		{"н", "y"},
		{"т", "n"},
		{"с", "c"},
		{"С", "C"},
		{".", "/"},
		{"enter", "enter"},
		{"ctrl+c", "ctrl+c"},
		{"ыы", "ыы"},
	}
	for _, tt := range tests {
		if got := normalizeShortcut(tt.in); got != tt.want {
			t.Errorf("normalizeShortcut(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRussianLayoutShortcuts(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	press := func(s string) {
		m.Update(teaKeyRunes(s))
	}
	press("ы")
	if !m.shuffle {
		t.Fatal("Cyrillic s did not toggle shuffle")
	}
	press("к")
	if m.repeat != "all" {
		t.Fatalf("repeat = %q, want all", m.repeat)
	}
	press("З")
	if m.modal != modalNewPlaylist {
		t.Fatalf("modal = %v, want new playlist", m.modal)
	}
	m.closeModal()

	press(".")
	if !m.searching {
		t.Fatal("Cyrillic dot did not start search")
	}
	m.exitSearch(true)

	m.startSearch()
	m.Update(teaKeyRunes("ы"))
	if m.search != "ы" {
		t.Fatalf("search = %q, want the typed Cyrillic letter", m.search)
	}
	m.exitSearch(true)
}
