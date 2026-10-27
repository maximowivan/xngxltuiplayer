package ui

import "github.com/charmbracelet/x/ansi"

// GlyphMode controls how much Unicode decoration the UI may use.
type GlyphMode int

const (
	GlyphsFull GlyphMode = iota
	GlyphsCompat
	GlyphsASCII
)

func (m GlyphMode) String() string {
	switch m {
	case GlyphsFull:
		return "full"
	case GlyphsCompat:
		return "compat"
	case GlyphsASCII:
		return "ascii"
	default:
		return "unknown"
	}
}

// glyphSet contains every decorative symbol that is not one of the transport
// icons. Keeping them in one place makes the fallback complete instead of
// changing only the play/pause/shuffle labels.
type glyphSet struct {
	BorderTopLeft     string
	BorderTopRight    string
	BorderBottomLeft  string
	BorderBottomRight string
	BorderHorizontal  string
	BorderVertical    string

	Separator string
	Ellipsis  string
	Dash      string
	Prompt    string

	Cursor   string
	Checked  string
	Active   string
	Drop     string
	Check    string
	Cross    string
	Partial  string
	Warning  string
	Bullet   string
	Stop     string
	Playlist string

	UpDown         string
	LeftRight      string
	UpDownWords    string
	LeftRightWords string
	Up             string
	Down           string
	Left           string
	Right          string

	ProgressFull  string
	ProgressEmpty string
	VolumeFull    string
	VolumeEmpty   string
	VolumePartial []rune

	SpectrumBlocks bool
	Spectrum       []rune
	SpectrumBase   string
	Spinner        []rune
	ArtHalfBlock   bool
}

var currentGlyphMode = GlyphsFull
var currentGlyphs = fullGlyphs()
var mouseAvailable = true

// SetMouseAvailable controls whether the UI advertises mouse actions.
func SetMouseAvailable(on bool) {
	mouseAvailable = on
}

func mouseHintAvailable() bool {
	return mouseAvailable
}

func fullGlyphs() glyphSet {
	return glyphSet{
		BorderTopLeft:     "╭",
		BorderTopRight:    "╮",
		BorderBottomLeft:  "╰",
		BorderBottomRight: "╯",
		BorderHorizontal:  "─",
		BorderVertical:    "│",
		Separator:         " · ",
		Ellipsis:          "…",
		Dash:              "—",
		Prompt:            "› ",
		Cursor:            "▸",
		Checked:           "✓",
		Active:            "●",
		Drop:              "→",
		Check:             "✓",
		Cross:             "✕",
		Partial:           "◐",
		Warning:           "!",
		Bullet:            "•",
		Stop:              "■",
		Playlist:          "≡",
		UpDown:            "↑↓",
		LeftRight:         "← →",
		UpDownWords:       "↑ ↓",
		LeftRightWords:    "← →",
		Up:                "↑",
		Down:              "↓",
		Left:              "←",
		Right:             "→",
		ProgressFull:      "━",
		ProgressEmpty:     "─",
		VolumeFull:        "█",
		VolumeEmpty:       "░",
		VolumePartial:     []rune("▏▎▍▌▋▊▉"),
		SpectrumBlocks:    true,
		Spectrum:          []rune("▁▂▃▄▅▆▇█"),
		SpectrumBase:      "· ",
		Spinner:           []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"),
		ArtHalfBlock:      true,
	}
}

func compatGlyphs() glyphSet {
	return glyphSet{
		BorderTopLeft:     "╭",
		BorderTopRight:    "╮",
		BorderBottomLeft:  "╰",
		BorderBottomRight: "╯",
		BorderHorizontal:  "─",
		BorderVertical:    "│",
		Separator:         " | ",
		Ellipsis:          "...",
		Dash:              "-",
		Prompt:            "> ",
		Cursor:            ">",
		Checked:           "x",
		Active:            "*",
		Drop:              ">",
		Check:             "+",
		Cross:             "x",
		Partial:           "o",
		Warning:           "!",
		Bullet:            "-",
		Stop:              "#",
		Playlist:          "=",
		UpDown:            "^v",
		LeftRight:         "<>",
		UpDownWords:       "up/down",
		LeftRightWords:    "left/right",
		Up:                "^",
		Down:              "v",
		Left:              "<",
		Right:             ">",
		ProgressFull:      "=",
		ProgressEmpty:     "-",
		VolumeFull:        "#",
		VolumeEmpty:       "-",
		SpectrumBlocks:    false,
		Spectrum:          []rune("#"),
		SpectrumBase:      "- ",
		Spinner:           []rune("|/-\\"),
		ArtHalfBlock:      true,
	}
}

func asciiGlyphs() glyphSet {
	g := compatGlyphs()
	g.BorderTopLeft = "+"
	g.BorderTopRight = "+"
	g.BorderBottomLeft = "+"
	g.BorderBottomRight = "+"
	g.BorderHorizontal = "-"
	g.BorderVertical = "|"
	g.ArtHalfBlock = false
	return g
}

// SetGlyphMode switches the complete UI glyph set. It also selects the icon
// subset; SetASCIIIcons can still be called afterwards to force icons
// independently of the chrome.
func SetGlyphMode(mode GlyphMode) {
	switch mode {
	case GlyphsFull:
		currentGlyphMode = GlyphsFull
		currentGlyphs = fullGlyphs()
		SetASCIIIcons(false)
	case GlyphsCompat:
		currentGlyphMode = GlyphsCompat
		currentGlyphs = compatGlyphs()
		SetASCIIIcons(true)
	default:
		currentGlyphMode = GlyphsASCII
		currentGlyphs = asciiGlyphs()
		SetASCIIIcons(true)
	}
}

// GlyphModeName returns the active glyph mode.
func GlyphModeName() string {
	return currentGlyphMode.String()
}

// ellipsisFor returns a truncation tail that fits into width cells.
func ellipsisFor(width int) string {
	if width <= 0 {
		return ""
	}
	tail := currentGlyphs.Ellipsis
	if ansi.StringWidth(tail) <= width {
		return tail
	}
	switch width {
	case 1:
		return "."
	case 2:
		return ".."
	default:
		return "..."
	}
}
