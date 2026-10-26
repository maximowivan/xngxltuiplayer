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
	toastText := lipgloss.Color("#11111B")
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
				{"toast/info", toastText, p.Blue, 3.5},
				{"toast/success", toastText, p.Green, 3.5},
				{"toast/warning", toastText, p.Yellow, 3.5},
				{"toast/error", toastText, p.Red, 3.5},
			}
			for _, c := range checks {
				if got := contrastRatio(c.fg, c.bg); got < c.min {
					t.Errorf("%s contrast %.2f, want at least %.1f", c.name, got, c.min)
				}
			}
		})
	}
}
