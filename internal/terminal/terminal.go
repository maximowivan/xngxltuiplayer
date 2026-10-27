// Package terminal detects the terminal host, its colour capabilities and
// whether the Unicode UI or a simpler compatibility mode should be used.
package terminal

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/term"
)

// ColorMode describes the colour depth the terminal can display.
type ColorMode int

const (
	// ColorNone disables colour completely (NO_COLOR, dumb terminals).
	ColorNone ColorMode = iota
	// ColorANSI16 is the classic 16-colour palette.
	ColorANSI16
	// ColorANSI256 is the xterm 256-colour palette.
	ColorANSI256
	// ColorTrueColor is 24-bit RGB.
	ColorTrueColor
)

func (c ColorMode) String() string {
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

// GlyphMode describes how much Unicode decoration is safe to draw.
type GlyphMode int

const (
	// GlyphsFull is the original Unicode UI.
	GlyphsFull GlyphMode = iota
	// GlyphsCompat keeps Unicode box drawing but uses ASCII for symbols,
	// cursors, bars and the spectrum.
	GlyphsCompat
	// GlyphsASCII uses printable ASCII everywhere, including the cover art.
	GlyphsASCII
)

func (g GlyphMode) String() string {
	switch g {
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

// Info describes the current terminal.
type Info struct {
	Program         string
	Term            string
	WindowsTerminal bool
	ConHost         bool
	Elevated        bool
	FontFace        string
	FontSize        int
	FontTrueType    bool
	OutputCP        uint32
	ConsoleClass    string
	Console         bool
	VT              bool
	TrueColor       bool
	Unicode         bool
	LocaleUTF8      bool
	Mouse           bool
	Color           ColorMode
	Glyphs          GlyphMode
}

// Detect inspects environment variables and the platform console.
func Detect() Info {
	info := Info{
		Program: detectProgram(),
		Term:    strings.TrimSpace(os.Getenv("TERM")),
		Console: stdoutIsConsole(),
	}
	info.WindowsTerminal = os.Getenv("WT_SESSION") != ""

	info.Elevated, info.FontFace, info.FontSize, info.OutputCP, info.ConsoleClass = platformInfo()
	classifyConsole(&info)
	info.FontTrueType = !isRasterFont(info.FontFace)
	info.LocaleUTF8 = detectLocaleUTF8()
	info.Unicode = detectUnicode(info)
	if runtime.GOOS == "windows" {
		info.VT = info.Console && platformVT()
	} else {
		info.VT = info.Console && !strings.EqualFold(info.Term, "dumb")
	}
	info.Color = detectColorFor(info, runtime.GOOS)
	info.TrueColor = info.Color == ColorTrueColor
	info.Glyphs = detectGlyphsFor(info, runtime.GOOS)
	info.Mouse = detectMouseFor(info, runtime.GOOS)
	return info
}

// classifyConsole distinguishes a real conhost window from the hidden
// pseudoconsole window used when Windows Terminal is the default terminal.
func classifyConsole(info *Info) {
	if info.ConsoleClass != "" {
		lower := strings.ToLower(info.ConsoleClass)
		switch {
		case strings.Contains(lower, "pseudo"):
			info.WindowsTerminal = true
			if info.Program == "Windows console (conhost)" {
				info.Program = "Windows Terminal (pseudoconsole)"
			}
		case strings.Contains(lower, "consolewindow"):
			info.ConHost = true
		}
	}
	if !info.ConHost && info.Program == "Windows console (conhost)" && !info.WindowsTerminal {
		info.ConHost = true
	}
}

// UseASCII reports whether the player should fall back to plain-text icons.
// It is kept for compatibility with older callers; new code should use Glyphs.
func (i Info) UseASCII() bool {
	return !i.Unicode || i.Glyphs == GlyphsCompat || i.Glyphs == GlyphsASCII
}

// StartupNote returns a short hint for the UI. An empty string means there is
// nothing to warn about.
func (i Info) StartupNote(mode GlyphMode) string {
	if !i.Console {
		return ""
	}
	if !i.VT {
		return "This console has no ANSI/VT support. Disable legacy console mode or use Windows Terminal."
	}
	switch mode {
	case GlyphsASCII:
		if runtime.GOOS == "windows" {
			return "Basic console mode: ASCII UI. Install Windows Terminal for the full view (optional)."
		}
		return "Basic terminal mode: ASCII UI. Use a modern terminal for the full view."
	case GlyphsCompat:
		return "Compatibility mode: simplified UI. Windows Terminal gives the full view (optional)."
	default:
		return ""
	}
}

// Recommendation returns a short action for --check.
func (i Info) Recommendation() string {
	if !i.Console {
		return "stdout is redirected; run --check in a terminal for TUI capabilities."
	}
	if !i.VT {
		return "Disable legacy console mode or install Windows Terminal; the TUI needs ANSI/VT support."
	}
	if runtime.GOOS == "windows" && !i.WindowsTerminal {
		if i.Glyphs == GlyphsFull {
			return "Windows Terminal is installed or available; the full UI is active."
		}
		return "Windows Terminal is optional; install it for the full UI. The compatibility mode works without it."
	}
	if i.Glyphs != GlyphsFull {
		return "Use a modern UTF-8 terminal for the full UI; the compatibility mode works without it."
	}
	return ""
}

// Summary returns human-readable lines for --check.
func (i Info) Summary() []string {
	lines := []string{
		fmt.Sprintf("  program      %s", i.Program),
		fmt.Sprintf("  elevated     %s", yesNo(i.Elevated)),
	}
	if i.FontFace != "" {
		lines = append(lines, fmt.Sprintf("  font         %s %d", i.FontFace, i.FontSize))
	}
	if i.OutputCP != 0 {
		lines = append(lines, fmt.Sprintf("  output CP    %d", i.OutputCP))
	}
	if i.ConsoleClass != "" {
		lines = append(lines, fmt.Sprintf("  console      %s", i.ConsoleClass))
	}
	lines = append(lines,
		fmt.Sprintf("  tty          %s", yesNo(i.Console)),
		fmt.Sprintf("  vt           %s", yesNo(i.VT)),
		fmt.Sprintf("  truecolor    %s", yesNo(i.TrueColor)),
		fmt.Sprintf("  color        %s", i.Color),
		fmt.Sprintf("  unicode      %s", yesNo(i.Unicode)),
		fmt.Sprintf("  glyphs       %s", i.Glyphs),
		fmt.Sprintf("  mouse        %s", yesNo(i.Mouse)),
	)
	if runtime.GOOS != "windows" {
		lines = append(lines, fmt.Sprintf("  locale UTF-8 %s", yesNo(i.LocaleUTF8)))
	}
	if i.ConHost {
		lines = append(lines, "  note         classic conhost has no font fallback")
	}
	if !i.Console {
		lines = append(lines, "  note         stdout is not a terminal; TUI capabilities are detected on launch")
	}
	if rec := i.Recommendation(); rec != "" {
		lines = append(lines, "  hint         "+rec)
	}
	return lines
}

func detectProgram() string {
	if os.Getenv("WT_SESSION") != "" {
		return "Windows Terminal"
	}
	if v := strings.TrimSpace(os.Getenv("TERM_PROGRAM")); v != "" {
		switch strings.ToLower(v) {
		case "vscode":
			return "VS Code terminal"
		case "wezterm":
			return "WezTerm"
		case "apple_terminal":
			return "Apple Terminal"
		case "iterm.app":
			return "iTerm2"
		case "mintty":
			return "mintty"
		default:
			return v
		}
	}
	if os.Getenv("WEZTERM_EXECUTABLE") != "" || os.Getenv("WEZTERM_PANE") != "" {
		return "WezTerm"
	}
	if os.Getenv("ALACRITTY_WINDOW_ID") != "" {
		return "Alacritty"
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return "kitty"
	}
	if strings.EqualFold(os.Getenv("ConEmuANSI"), "ON") {
		return "ConEmu"
	}
	if runtime.GOOS == "windows" {
		return "Windows console (conhost)"
	}
	if term := strings.TrimSpace(os.Getenv("TERM")); term != "" {
		return term
	}
	return "unknown terminal"
}

func detectColorFor(i Info, goos string) ColorMode {
	if noColorRequested() || strings.EqualFold(i.Term, "dumb") {
		return ColorNone
	}
	if i.WindowsTerminal {
		return ColorTrueColor
	}
	if goos == "windows" {
		if i.Console && i.VT {
			// conhost has supported 24-bit colour since Windows 10 1607.
			return ColorTrueColor
		}
		if i.Console {
			return ColorANSI16
		}
		return ColorNone
	}
	if !i.Console {
		return ColorNone
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return ColorTrueColor
	}
	term := strings.ToLower(i.Term)
	if strings.Contains(term, "direct") || strings.Contains(term, "truecolor") {
		return ColorTrueColor
	}
	switch strings.ToLower(i.Program) {
	case "wezterm", "vscode terminal", "alacritty", "kitty", "iterm2":
		return ColorTrueColor
	}
	if strings.Contains(term, "256color") {
		return ColorANSI256
	}
	if strings.EqualFold(i.Program, "Apple Terminal") {
		return ColorANSI256
	}
	if term == "" {
		return ColorNone
	}
	return ColorANSI16
}

func detectGlyphsFor(i Info, goos string) GlyphMode {
	if strings.EqualFold(i.Term, "dumb") {
		return GlyphsASCII
	}
	if goos == "windows" {
		if i.WindowsTerminal {
			return GlyphsFull
		}
		if i.ConHost {
			if i.FontTrueType && i.VT {
				return GlyphsCompat
			}
			return GlyphsASCII
		}
		if i.VT && i.Unicode {
			return GlyphsCompat
		}
		return GlyphsASCII
	}
	if !i.Console || !i.LocaleUTF8 || !i.Unicode || strings.EqualFold(i.Term, "linux") {
		return GlyphsASCII
	}
	if strings.EqualFold(i.Program, "Apple Terminal") {
		return GlyphsCompat
	}
	return GlyphsFull
}

func detectMouseFor(i Info, goos string) bool {
	if goos == "windows" {
		return i.WindowsTerminal
	}
	if !i.Console {
		return false
	}
	switch strings.ToLower(i.Term) {
	case "linux", "dumb", "":
		return false
	}
	return true
}

func detectUnicode(i Info) bool {
	if runtime.GOOS == "windows" {
		if i.WindowsTerminal {
			return true
		}
		if i.ConHost {
			return false
		}
		switch strings.ToLower(i.Program) {
		case "windows terminal", "vs code terminal", "wezterm", "alacritty", "kitty", "conemu", "mintty":
			return true
		}
		// Unknown Windows terminal: be conservative and use ASCII icons.
		return false
	}
	if strings.EqualFold(i.Term, "dumb") || strings.EqualFold(i.Term, "linux") {
		return false
	}
	return i.LocaleUTF8
}

func detectLocaleUTF8() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if lower == "c" || lower == "posix" {
			return false
		}
		return strings.Contains(lower, "utf-8") || strings.Contains(lower, "utf8")
	}
	// Modern Linux distributions default to UTF-8 when the locale is unset.
	return true
}

func noColorRequested() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("CLICOLOR")) == "0"
}

func isRasterFont(face string) bool {
	f := strings.ToLower(strings.TrimSpace(face))
	if f == "" {
		return true
	}
	return f == "terminal" || f == "raster" || strings.Contains(f, "raster") ||
		strings.Contains(f, "fixedsys") || strings.Contains(f, "oem")
}

func stdoutIsConsole() bool {
	return term.IsTerminal(os.Stdout.Fd())
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
