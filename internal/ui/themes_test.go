package ui

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func relativeLuminance(c lipgloss.Color) float64 {
	r, g, b := hexRGB(string(c))
	lin := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrastRatio(a, b lipgloss.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestThemeBackgroundPainted(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.width, m.height = 110, 32

	applyTheme("nord")
	defer applyTheme(defaultTheme)
	view := m.View()
	// Nord's base color #2E3440.
	seq := "\x1b[48;2;46;52;64m"
	lines := strings.Split(view, "\n")
	if len(lines) != 32 {
		t.Fatalf("got %d lines, want 32", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, seq) {
			t.Fatalf("line %d is not filled with the theme background: %q", i, line)
		}
	}
}

// TestThemeContrast keeps every palette readable on its own background.
func TestThemeContrast(t *testing.T) {
	for _, p := range themePalettes {
		t.Run(p.Name, func(t *testing.T) {
			checks := []struct {
				name string
				fg   lipgloss.Color
				bg   lipgloss.Color
				min  float64
			}{
				{"text", p.Text, p.Bg, 7.0},
				{"dim", p.Dim, p.Bg, 3.5},
				{"faint", p.Faint, p.Bg, 2.2},
				{"accent", p.Accent, p.Bg, 4.5},
				{"selection", p.SelFg, p.SelBg, 4.5},
				{"pink", p.Pink, p.Bg, 3.0},
				{"green", p.Green, p.Bg, 3.0},
				{"yellow", p.Yellow, p.Bg, 3.0},
				{"red", p.Red, p.Bg, 3.0},
				{"blue", p.Blue, p.Bg, 3.0},
				{"toast/info", readableOn(p.Blue), p.Blue, 3.5},
				{"toast/success", readableOn(p.Green), p.Green, 3.5},
				{"toast/warning", readableOn(p.Yellow), p.Yellow, 3.5},
				{"toast/error", readableOn(p.Red), p.Red, 3.5},
			}
			for _, c := range checks {
				if got := contrastRatio(c.fg, c.bg); got < c.min {
					t.Errorf("%s contrast %.2f, want at least %.1f", c.name, got, c.min)
				}
			}
		})
	}
}

func TestThemePickerPreviewAndCancel(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	applyTheme(defaultTheme)
	defer applyTheme(defaultTheme)
	m.theme = defaultTheme
	m.openThemePicker()
	if m.modal != modalThemes {
		t.Fatalf("modal = %v, want themes", m.modal)
	}
	original := m.theme
	m.previewTheme(1)
	if m.theme == original {
		t.Fatal("preview did not change the theme")
	}
	m.cancelThemePicker()
	if m.theme != original {
		t.Fatalf("theme = %q after cancel, want %q", m.theme, original)
	}
	if m.modal != modalNone {
		t.Fatalf("modal = %v after cancel", m.modal)
	}
}

func TestThemePickerCommit(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	applyTheme(defaultTheme)
	defer applyTheme(defaultTheme)
	m.theme = defaultTheme
	m.openThemePicker()
	m.previewTheme(2)
	want := m.theme
	m.commitThemePicker()
	if m.theme != want {
		t.Fatalf("theme = %q, want %q", m.theme, want)
	}
	if !m.saveDirty {
		t.Fatal("committing a theme should mark the session dirty")
	}
	if m.modal != modalNone {
		t.Fatalf("modal = %v after commit", m.modal)
	}
}

func TestThemeCount(t *testing.T) {
	if len(themePalettes) < 20 {
		t.Fatalf("theme count = %d, want at least 20", len(themePalettes))
	}
	seen := make(map[string]bool, len(themePalettes))
	for _, p := range themePalettes {
		if seen[p.Name] {
			t.Fatalf("duplicate theme name %q", p.Name)
		}
		seen[p.Name] = true
		if got := applyTheme(p.Name); got != p.Name {
			t.Fatalf("applyTheme(%q) = %q", p.Name, got)
		}
	}
	applyTheme(defaultTheme)
}
