package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// ColorProfile is the UI's colour capability. It mirrors terminal.ColorMode
// but lives here so the renderer does not depend on the terminal package.
type ColorProfile int

const (
	ColorNone ColorProfile = iota
	ColorANSI16
	ColorANSI256
	ColorTrueColor
)

var activeColorProfile = ColorTrueColor

func (c ColorProfile) String() string {
	switch c {
	case ColorNone:
		return "none"
	case ColorANSI16:
		return "16"
	case ColorANSI256:
		return "256"
	case ColorTrueColor:
		return "truecolor"
	default:
		return "unknown"
	}
}

// SetColorProfile configures lipgloss and the direct colour sequences used by
// the background, artwork and spectrum.
func SetColorProfile(p ColorProfile) {
	activeColorProfile = p
	switch p {
	case ColorNone:
		lipgloss.SetColorProfile(termenv.Ascii)
	case ColorANSI16:
		lipgloss.SetColorProfile(termenv.ANSI)
	case ColorANSI256:
		lipgloss.SetColorProfile(termenv.ANSI256)
	default:
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
	buildStyles()
}

func termProfile() termenv.Profile {
	switch activeColorProfile {
	case ColorNone:
		return termenv.Ascii
	case ColorANSI16:
		return termenv.ANSI
	case ColorANSI256:
		return termenv.ANSI256
	default:
		return termenv.TrueColor
	}
}

// colorSequence converts a lipgloss colour to the active terminal profile.
func colorSequence(c lipgloss.Color, background bool) string {
	if activeColorProfile == ColorNone || c == "" {
		return ""
	}
	colour := termProfile().Color(string(c))
	if colour == nil {
		return ""
	}
	seq := colour.Sequence(background)
	if seq == "" {
		return ""
	}
	return "\x1b[" + seq + "m"
}

var (
	colText    lipgloss.Color
	colDim     lipgloss.Color
	colFaint   lipgloss.Color
	colAccent  lipgloss.Color
	colAccent2 lipgloss.Color
	colPink    lipgloss.Color
	colGreen   lipgloss.Color
	colYellow  lipgloss.Color
	colRed     lipgloss.Color
	colBlue    lipgloss.Color
	colSelBg   lipgloss.Color
	colSelFg   lipgloss.Color
	colBg      lipgloss.Color
)

var (
	textStyle      lipgloss.Style
	dimStyle       lipgloss.Style
	faintStyle     lipgloss.Style
	accentStyle    lipgloss.Style
	accent2Style   lipgloss.Style
	titleStyle     lipgloss.Style
	selectedStyle  lipgloss.Style
	dropStyle      lipgloss.Style
	playingStyle   lipgloss.Style
	errorStyle     lipgloss.Style
	successStyle   lipgloss.Style
	warningStyle   lipgloss.Style
	keyStyle       lipgloss.Style
	sectionStyle   lipgloss.Style
	borderDimStyle lipgloss.Style
)

// buildStyles recreates every style from the active palette. It is called
// whenever the user switches themes.
func buildStyles() {
	textStyle = lipgloss.NewStyle().Foreground(colText)
	dimStyle = lipgloss.NewStyle().Foreground(colDim)
	faintStyle = lipgloss.NewStyle().Foreground(colFaint)
	accentStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	accent2Style = lipgloss.NewStyle().Foreground(colAccent2).Bold(true)
	titleStyle = lipgloss.NewStyle().Foreground(colText).Bold(true)
	selectedStyle = lipgloss.NewStyle().Background(colSelBg).Foreground(colSelFg)
	dropStyle = lipgloss.NewStyle().Background(colAccent).Foreground(colBg).Bold(true)
	playingStyle = lipgloss.NewStyle().Foreground(colAccent2).Bold(true)
	errorStyle = lipgloss.NewStyle().Foreground(colRed)
	successStyle = lipgloss.NewStyle().Foreground(colGreen)
	warningStyle = lipgloss.NewStyle().Foreground(colYellow)
	keyStyle = lipgloss.NewStyle().Foreground(colAccent2).Bold(true)
	sectionStyle = lipgloss.NewStyle().Foreground(colDim).Bold(true)
	borderDimStyle = lipgloss.NewStyle().Foreground(colFaint)
	if activeColorProfile == ColorNone {
		// Keep selection and emphasis visible without relying on colour.
		selectedStyle = lipgloss.NewStyle().Reverse(true)
		dropStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
		playingStyle = lipgloss.NewStyle().Bold(true)
		dimStyle = lipgloss.NewStyle().Faint(true)
		faintStyle = lipgloss.NewStyle().Faint(true)
	}
}

// fitLine truncates a possibly styled line to width and pads it with spaces.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, ellipsisFor(width))
	}
	if gap := width - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

func truncateText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, ellipsisFor(width))
}

// cleanText makes dynamic text safe for a single terminal line: strips ANSI
// escapes, replaces invalid UTF-8, converts tabs/newlines to spaces and drops
// other control characters.
func cleanText(s string) string {
	if s == "" {
		return ""
	}
	s = ansi.Strip(s)
	s = strings.ToValidUTF8(s, "?")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
	return strings.TrimSpace(s)
}

func centerLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w >= width {
		return truncateText(s, width)
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

func wrapText(s string, width, maxLines int) []string {
	if width <= 0 || maxLines <= 0 {
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	wrapped := ansi.Wordwrap(s, width, " -/")
	raw := strings.Split(wrapped, "\n")
	lines := make([]string, 0, maxLines)
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if ansi.StringWidth(line) > width {
			line = ansi.Truncate(line, width, ellipsisFor(width))
		}
		lines = append(lines, line)
		if len(lines) == maxLines {
			break
		}
	}
	if len(raw) > maxLines && len(lines) == maxLines {
		last := lines[len(lines)-1]
		tail := ellipsisFor(width)
		if ansi.StringWidth(last)+ansi.StringWidth(tail) <= width {
			lines[len(lines)-1] = last + tail
		}
	}
	return lines
}

// renderBox draws a rounded panel with an optional title and exact dimensions.
func renderBox(title string, lines []string, width, height int, borderColor, titleColor lipgloss.Color) string {
	if width < 4 || height < 2 {
		return fitLine(strings.Join(lines, " "), width)
	}
	border := lipgloss.NewStyle().Foreground(borderColor)
	titleSt := lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	innerW := width - 4
	innerH := height - 2

	var top string
	title = strings.ToUpper(strings.TrimSpace(title))
	if title == "" {
		top = border.Render(currentGlyphs.BorderTopLeft) +
			border.Render(strings.Repeat(currentGlyphs.BorderHorizontal, width-2)) +
			border.Render(currentGlyphs.BorderTopRight)
	} else {
		if ansi.StringWidth(title) > width-6 {
			title = truncateText(title, maxInt(width-6, 1))
		}
		fill := width - 5 - ansi.StringWidth(title)
		if fill < 0 {
			fill = 0
		}
		top = border.Render(currentGlyphs.BorderTopLeft) + border.Render(currentGlyphs.BorderHorizontal) + " " +
			titleSt.Render(title) + " " +
			border.Render(strings.Repeat(currentGlyphs.BorderHorizontal, fill)) + border.Render(currentGlyphs.BorderTopRight)
	}

	bottom := border.Render(currentGlyphs.BorderBottomLeft) +
		border.Render(strings.Repeat(currentGlyphs.BorderHorizontal, width-2)) +
		border.Render(currentGlyphs.BorderBottomRight)
	rows := make([]string, 0, height)
	rows = append(rows, top)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		rows = append(rows, border.Render(currentGlyphs.BorderVertical)+" "+fitLine(line, innerW)+" "+border.Render(currentGlyphs.BorderVertical))
	}
	rows = append(rows, bottom)
	return strings.Join(rows, "\n")
}

func renderToast(message string, kind toastKind, width int) string {
	if width <= 0 {
		return ""
	}
	message = cleanText(message)
	icon := currentGlyphs.Bullet
	bg := colBlue
	switch kind {
	case toastSuccess:
		icon, bg = currentGlyphs.Check, colGreen
	case toastError:
		icon, bg = currentGlyphs.Cross, colRed
	case toastWarning:
		icon, bg = currentGlyphs.Warning, colYellow
	}
	content := " " + icon + "  " + message
	style := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#11111B")).Bold(true)
	return style.Render(fitLine(content, width))
}

func keyHint(key, description string) string {
	return keyStyle.Render(key) + " " + dimStyle.Render(description)
}

func hintLine(parts ...string) string {
	sep := faintStyle.Render(currentGlyphs.Separator)
	return strings.Join(parts, sep)
}

func gradientText(s string, from, to lipgloss.Color) string {
	if activeColorProfile != ColorTrueColor {
		return accentStyle.Render(s)
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range runes {
		t := 0.0
		if len(runes) > 1 {
			t = float64(i) / float64(len(runes)-1)
		}
		r1, g1, b1 := hexRGB(string(from))
		r2, g2, b2 := hexRGB(string(to))
		color := lipgloss.Color(fmt.Sprintf("#%02X%02X%02X",
			int(float64(r1)+(float64(r2-r1)*t)),
			int(float64(g1)+(float64(g2-g1)*t)),
			int(float64(b1)+(float64(b2-b1)*t)),
		))
		b.WriteString(lipgloss.NewStyle().Foreground(color).Bold(true).Render(string(r)))
	}
	return b.String()
}

func hexRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 32)
	g, _ := strconv.ParseInt(hex[2:4], 16, 32)
	b, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return int(r), int(g), int(b)
}

// paintBackground fills every terminal cell with the theme background. Every
// style reset is followed by the background again, so truncated or styled
// segments do not leave black holes in the middle of a line.
func paintBackground(view string, bg lipgloss.Color) string {
	if bg == "" || activeColorProfile == ColorNone {
		return view
	}
	seq := colorSequence(bg, true)
	if seq == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq)
		line = strings.ReplaceAll(line, "\x1b[49m", "\x1b[49m"+seq)
		line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+seq)
		lines[i] = seq + line + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
