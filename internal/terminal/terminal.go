// Package terminal detects the terminal host and decides whether the Unicode
// icon set is safe to use.
package terminal

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Info describes the current terminal.
type Info struct {
	Program         string
	WindowsTerminal bool
	ConHost         bool
	Elevated        bool
	FontFace        string
	FontSize        int
	OutputCP        uint32
	ConsoleClass    string
	TrueColor       bool
	Unicode         bool
}

// Detect inspects environment variables and the Windows console.
func Detect() Info {
	info := Info{Program: detectProgram()}
	info.WindowsTerminal = os.Getenv("WT_SESSION") != ""

	info.Elevated, info.FontFace, info.FontSize, info.OutputCP, info.ConsoleClass = platformInfo()
	classifyConsole(&info)
	info.TrueColor = detectTrueColor(info)
	info.Unicode = detectUnicode(info)
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
func (i Info) UseASCII() bool {
	return !i.Unicode
}

// StartupNote returns a short hint for the UI. An empty string means there is
// nothing to warn about.
func (i Info) StartupNote(ascii bool) string {
	if !i.ConHost {
		return ""
	}
	if ascii {
		if i.Elevated {
			return "Classic console (admin): ASCII icons enabled. Use Windows Terminal for the full UI."
		}
		return "Classic console: ASCII icons enabled. Use Windows Terminal for the full UI."
	}
	return "Classic console detected. If icons look wrong, start with -icons ascii or use Windows Terminal."
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
		fmt.Sprintf("  truecolor    %s", yesNo(i.TrueColor)),
		fmt.Sprintf("  unicode      %s", yesNo(i.Unicode)),
	)
	if i.ConHost {
		lines = append(lines, "  note         classic conhost has no font fallback")
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

func detectTrueColor(i Info) bool {
	if i.WindowsTerminal {
		return true
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return true
	}
	switch strings.ToLower(i.Program) {
	case "wezterm", "vscode terminal", "alacritty", "kitty", "iterm2":
		return true
	}
	return false
}

func detectUnicode(i Info) bool {
	if runtime.GOOS != "windows" {
		return true
	}
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

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
