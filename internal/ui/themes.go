package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// themePalette is one complete color scheme. All UI chrome and the visualizer
// derive their colors from these values.
type themePalette struct {
	Name    string
	Label   string
	Text    lipgloss.Color
	Dim     lipgloss.Color
	Faint   lipgloss.Color
	Accent  lipgloss.Color
	Accent2 lipgloss.Color
	Pink    lipgloss.Color
	Green   lipgloss.Color
	Yellow  lipgloss.Color
	Red     lipgloss.Color
	Blue    lipgloss.Color
	SelBg   lipgloss.Color
	SelFg   lipgloss.Color
	Bg      lipgloss.Color
}

var themePalettes = []themePalette{
	{
		Name: "catppuccin", Label: "Catppuccin Mocha",
		Text: "#D9E0EE", Dim: "#7F849C", Faint: "#585B70",
		Accent: "#CBA6F7", Accent2: "#89DCEB",
		Pink: "#F5C2E7", Green: "#A6E3A1", Yellow: "#F9E2AF",
		Red: "#F38BA8", Blue: "#89B4FA",
		SelBg: "#313244", SelFg: "#CDD6F4", Bg: "#1E1E2E",
	},
	{
		Name: "tokyonight", Label: "Tokyo Night",
		Text: "#C0CAF5", Dim: "#737AA2", Faint: "#525C85",
		Accent: "#BB9AF7", Accent2: "#7DCFFF",
		Pink: "#FF75A0", Green: "#9ECE6A", Yellow: "#E0AF68",
		Red: "#F7768E", Blue: "#7AA2F7",
		SelBg: "#292E42", SelFg: "#C0CAF5", Bg: "#1A1B26",
	},
	{
		Name: "gruvbox", Label: "Gruvbox",
		Text: "#EBDBB2", Dim: "#A89984", Faint: "#7C6F64",
		Accent: "#FE8019", Accent2: "#8EC07C",
		Pink: "#D3869B", Green: "#B8BB26", Yellow: "#FABD2F",
		Red: "#FB4934", Blue: "#83A598",
		SelBg: "#3C3836", SelFg: "#FBF1C7", Bg: "#282828",
	},
	{
		Name: "nord", Label: "Nord",
		Text: "#D8DEE9", Dim: "#81A1C1", Faint: "#66738F",
		Accent: "#88C0D0", Accent2: "#8FBCBB",
		Pink: "#B48EAD", Green: "#A3BE8C", Yellow: "#EBCB8B",
		Red: "#BF616A", Blue: "#81A1C1",
		SelBg: "#3B4252", SelFg: "#ECEFF4", Bg: "#2E3440",
	},
	{
		Name: "dracula", Label: "Dracula",
		Text: "#F8F8F2", Dim: "#7C86C4", Faint: "#5F6490",
		Accent: "#BD93F9", Accent2: "#8BE9FD",
		Pink: "#FF79C6", Green: "#50FA7B", Yellow: "#F1FA8C",
		Red: "#FF5555", Blue: "#6E9BFF",
		SelBg: "#44475A", SelFg: "#F8F8F2", Bg: "#282A36",
	},
	{
		Name: "rosepine", Label: "Rosé Pine Moon",
		Text: "#E0DEF4", Dim: "#908CAA", Faint: "#625E7D",
		Accent: "#C4A7E7", Accent2: "#9CCFD8",
		Pink: "#EBBCBA", Green: "#95B1AC", Yellow: "#F6C177",
		Red: "#EB6F92", Blue: "#3E8FB0",
		SelBg: "#393552", SelFg: "#E0DEF4", Bg: "#232136",
	},
	{
		Name: "ember", Label: "Ember",
		Text: "#E8E3DC", Dim: "#9A8F84", Faint: "#6A5C52",
		Accent: "#F5A97F", Accent2: "#8BD5CA",
		Pink: "#F5BDE6", Green: "#A6DA95", Yellow: "#EED49F",
		Red: "#ED8796", Blue: "#8AADF4",
		SelBg: "#3A322E", SelFg: "#F5EFE8", Bg: "#262220",
	},
}

const defaultTheme = "catppuccin"

var currentTheme = defaultTheme

func init() {
	applyTheme(defaultTheme)
}

func themeIndex(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(name)
	for i, p := range themePalettes {
		if p.Name == name {
			return i
		}
	}
	return 0
}

func themeByIndex(index int) themePalette {
	n := len(themePalettes)
	index = ((index % n) + n) % n
	return themePalettes[index]
}

// applyTheme switches the active palette and returns its canonical name.
func applyTheme(name string) string {
	index := themeIndex(name)
	p := themePalettes[index]

	colText, colDim, colFaint = p.Text, p.Dim, p.Faint
	colAccent, colAccent2 = p.Accent, p.Accent2
	colPink, colGreen, colYellow = p.Pink, p.Green, p.Yellow
	colRed, colBlue = p.Red, p.Blue
	colSelBg, colSelFg = p.SelBg, p.SelFg
	colBg = p.Bg

	buildStyles()
	applyArtPalette(p)
	currentTheme = p.Name
	return p.Name
}

func themeLabel(name string) string {
	return themePalettes[themeIndex(name)].Label
}

func (m *Model) cycleTheme(delta int) {
	p := nextTheme(themeIndex(m.theme), delta)
	m.theme = applyTheme(p.Name)
	m.markDirty()
	m.setToast(iconTheme+" Theme: "+p.Label, toastInfo, 1600*time.Millisecond)
}

// nextTheme returns the palette after index+delta (delta may be negative).
func nextTheme(index, delta int) themePalette {
	return themeByIndex(index + delta)
}

// applyArtPalette derives the placeholder artwork and spectrum colors from the
// active theme, so pictures stay in the same color family as the UI.
func applyArtPalette(p themePalette) {
	accent := colorRGB(p.Accent)
	accent2 := colorRGB(p.Accent2)
	bg := colorRGB(p.Bg)

	artBgColor = bg
	artTopColor = [3]float64{
		bg[0]*0.70 + accent[0]*0.30 + 6,
		bg[1]*0.70 + accent[1]*0.30 + 6,
		bg[2]*0.70 + accent[2]*0.30 + 8,
	}
	artBottomColor = [3]float64{
		bg[0]*0.92 + accent2[0]*0.08,
		bg[1]*0.92 + accent2[1]*0.08,
		bg[2]*0.92 + accent2[2]*0.08,
	}
	specLowColor = colorRGB(p.Green)
	specMidColor = colorRGB(p.Yellow)
	specHighColor = colorRGB(p.Pink)
}

func colorRGB(c lipgloss.Color) [3]float64 {
	r, g, b := hexRGB(string(c))
	return [3]float64{float64(r), float64(g), float64(b)}
}
