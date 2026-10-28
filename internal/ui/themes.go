package ui

import (
	"math"
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
	Light   bool
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
		Name: "rosepine", Label: "Rose Pine Moon",
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
	{
		Name: "catppuccinmacchiato", Label: "Catppuccin Macchiato",
		Text: "#CAD3F5", Dim: "#A5ADCB", Faint: "#8087A2",
		Accent: "#C6A0F6", Accent2: "#91D7E3",
		Pink: "#F5BDE6", Green: "#A6DA95", Yellow: "#EED49F",
		Red: "#ED8796", Blue: "#8AADF4",
		SelBg: "#363A4F", SelFg: "#CAD3F5", Bg: "#24273A",
	},
	{
		Name: "catppuccinfrappe", Label: "Catppuccin Frappe",
		Text: "#C6D0F5", Dim: "#A5ADCE", Faint: "#838BA7",
		Accent: "#CA9EE6", Accent2: "#99D1DB",
		Pink: "#F4B8E4", Green: "#A6D189", Yellow: "#E5C890",
		Red: "#E78284", Blue: "#8CAAEE",
		SelBg: "#414559", SelFg: "#C6D0F5", Bg: "#303446",
	},
	{
		Name: "catppuccinlatte", Label: "Catppuccin Latte", Light: true,
		Text: "#4C4F69", Dim: "#6C6F85", Faint: "#8C8FA1",
		Accent: "#8839EF", Accent2: "#04A5E5",
		Pink: "#C42B86", Green: "#358A24", Yellow: "#A86400",
		Red: "#D20F39", Blue: "#1E66F5",
		SelBg: "#CCD0DA", SelFg: "#4C4F69", Bg: "#EFF1F5",
	},
	{
		Name: "tokyonightstorm", Label: "Tokyo Night Storm",
		Text: "#C0CAF5", Dim: "#7982A9", Faint: "#5F6790",
		Accent: "#BB9AF7", Accent2: "#7DCFFF",
		Pink: "#FF75A0", Green: "#9ECE6A", Yellow: "#E0AF68",
		Red: "#F7768E", Blue: "#7AA2F7",
		SelBg: "#2F3B5C", SelFg: "#C0CAF5", Bg: "#24283B",
	},
	{
		Name: "gruvboxlight", Label: "Gruvbox Light", Light: true,
		Text: "#3C3836", Dim: "#665C54", Faint: "#7C6F64",
		Accent: "#AF3A03", Accent2: "#427B58",
		Pink: "#B16286", Green: "#79740E", Yellow: "#B57614",
		Red: "#9D0006", Blue: "#076678",
		SelBg: "#EBDBB2", SelFg: "#3C3836", Bg: "#FBF1C7",
	},
	{
		Name: "solarizeddark", Label: "Solarized Dark",
		Text: "#C5D0D0", Dim: "#93A1A1", Faint: "#657B83",
		Accent: "#4AA6E0", Accent2: "#2AA198",
		Pink: "#D33682", Green: "#859900", Yellow: "#B58900",
		Red: "#DC322F", Blue: "#268BD2",
		SelBg: "#073642", SelFg: "#EEE8D5", Bg: "#002B36",
	},
	{
		Name: "solarizedlight", Label: "Solarized Light", Light: true,
		Text: "#073642", Dim: "#586E75", Faint: "#657B83",
		Accent: "#1E6F9F", Accent2: "#2AA198",
		Pink: "#D33682", Green: "#6B7D00", Yellow: "#946E00",
		Red: "#DC322F", Blue: "#1E6F9F",
		SelBg: "#EEE8D5", SelFg: "#073642", Bg: "#FDF6E3",
	},
	{
		Name: "onedark", Label: "One Dark",
		Text: "#B9C0CB", Dim: "#7F848E", Faint: "#5C6370",
		Accent: "#C678DD", Accent2: "#56B6C2",
		Pink: "#E06C75", Green: "#98C379", Yellow: "#E5C07B",
		Red: "#E06C75", Blue: "#61AFEF",
		SelBg: "#3E4451", SelFg: "#ABB2BF", Bg: "#282C34",
	},
	{
		Name: "monokai", Label: "Monokai",
		Text: "#F8F8F2", Dim: "#A6A69C", Faint: "#75715E",
		Accent: "#AE81FF", Accent2: "#66D9EF",
		Pink: "#F92672", Green: "#A6E22E", Yellow: "#E6DB74",
		Red: "#F92672", Blue: "#66D9EF",
		SelBg: "#49483E", SelFg: "#F8F8F2", Bg: "#272822",
	},
	{
		Name: "kanagawa", Label: "Kanagawa",
		Text: "#DCD7BA", Dim: "#7E7D75", Faint: "#54546D",
		Accent: "#957FB8", Accent2: "#7FB4CA",
		Pink: "#D27E99", Green: "#76946A", Yellow: "#C0A36E",
		Red: "#E82424", Blue: "#7E9CD8",
		SelBg: "#2D4F67", SelFg: "#DCD7BA", Bg: "#1F1F28",
	},
	{
		Name: "everforest", Label: "Everforest",
		Text: "#D3C6AA", Dim: "#859289", Faint: "#5C6A72",
		Accent: "#A7C080", Accent2: "#83C092",
		Pink: "#D699B6", Green: "#A7C080", Yellow: "#DBBC7F",
		Red: "#E67E80", Blue: "#7FBBB3",
		SelBg: "#3D484D", SelFg: "#D3C6AA", Bg: "#2D353B",
	},
	{
		Name: "nightfox", Label: "Nightfox",
		Text: "#CDCECF", Dim: "#738091", Faint: "#4D5B70",
		Accent: "#719CD6", Accent2: "#63CDCF",
		Pink: "#D67AD2", Green: "#8EBAA4", Yellow: "#DBC074",
		Red: "#C94F6D", Blue: "#719CD6",
		SelBg: "#29394F", SelFg: "#CDCECF", Bg: "#192330",
	},
	{
		Name: "rosepinedawn", Label: "Rose Pine Dawn", Light: true,
		Text: "#4C4768", Dim: "#797593", Faint: "#9893A5",
		Accent: "#6E5A8A", Accent2: "#56949F",
		Pink: "#B05A57", Green: "#286983", Yellow: "#A86400",
		Red: "#B4637A", Blue: "#286983",
		SelBg: "#F2E9E1", SelFg: "#4C4768", Bg: "#FAF4ED",
	},
	{
		Name: "githubdark", Label: "GitHub Dark",
		Text: "#C9D1D9", Dim: "#8B949E", Faint: "#6E7681",
		Accent: "#BC8CFF", Accent2: "#79C0FF",
		Pink: "#FF7B72", Green: "#3FB950", Yellow: "#D29922",
		Red: "#F85149", Blue: "#58A6FF",
		SelBg: "#21262D", SelFg: "#C9D1D9", Bg: "#0D1117",
	},
	{
		Name: "githublight", Label: "GitHub Light", Light: true,
		Text: "#24292F", Dim: "#57606A", Faint: "#6E7781",
		Accent: "#8250DF", Accent2: "#0969DA",
		Pink: "#BF3989", Green: "#1A7F37", Yellow: "#9A6700",
		Red: "#CF222E", Blue: "#0969DA",
		SelBg: "#EAEEF2", SelFg: "#24292F", Bg: "#FFFFFF",
	},
}

const defaultTheme = "catppuccin"

var currentTheme = defaultTheme

func init() {
	SetColorProfile(ColorTrueColor)
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

// colorLuminance returns the WCAG relative luminance of a hex colour.
func colorLuminance(c lipgloss.Color) float64 {
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

func contrastBetween(a, b lipgloss.Color) float64 {
	la, lb := colorLuminance(a), colorLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readableOn picks the dark or light toast text with the better contrast.
func readableOn(bg lipgloss.Color) lipgloss.Color {
	dark := lipgloss.Color("#11111B")
	light := lipgloss.Color("#F8F8F2")
	if contrastBetween(dark, bg) >= contrastBetween(light, bg) {
		return dark
	}
	return light
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
	artGlowSign = 1
	if p.Light {
		artGlowSign = -1
	}

	artBgColor = bg
	artTopColor = [3]float64{
		bg[0]*0.70 + accent[0]*0.30 + artGlowSign*6,
		bg[1]*0.70 + accent[1]*0.30 + artGlowSign*6,
		bg[2]*0.70 + accent[2]*0.30 + artGlowSign*8,
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
