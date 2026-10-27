package terminal

import (
	"runtime"
	"strings"
	"testing"
)

func TestNoColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR", "")
	if got := detectColorFor(Info{WindowsTerminal: true, Console: true, VT: true}, "windows"); got != ColorNone {
		t.Fatalf("NO_COLOR color = %s, want none", got)
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "0")
	if got := detectColorFor(Info{Console: true, Term: "xterm-256color"}, "linux"); got != ColorNone {
		t.Fatalf("CLICOLOR=0 color = %s, want none", got)
	}
}

func TestDetectProgramWindowsTerminal(t *testing.T) {
	t.Setenv("WT_SESSION", "abc")
	t.Setenv("TERM_PROGRAM", "")
	if got := detectProgram(); got != "Windows Terminal" {
		t.Fatalf("detectProgram = %q, want Windows Terminal", got)
	}
}

func TestDetectProgramVSCode(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "vscode")
	if got := detectProgram(); got != "VS Code terminal" {
		t.Fatalf("detectProgram = %q, want VS Code terminal", got)
	}
}

func TestUseASCIIFollowsUnicode(t *testing.T) {
	if (Info{Unicode: true}).UseASCII() {
		t.Fatal("Unicode terminal should not use ASCII")
	}
	if !(Info{Unicode: false}).UseASCII() {
		t.Fatal("non-Unicode terminal should use ASCII")
	}
}

func TestStartupNoteForConHost(t *testing.T) {
	info := Info{ConHost: true, Console: true, VT: true}
	if note := info.StartupNote(GlyphsASCII); !strings.Contains(strings.ToLower(note), "ascii") {
		t.Fatalf("unexpected note: %q", note)
	}
	if note := info.StartupNote(GlyphsCompat); !strings.Contains(note, "Compatibility") {
		t.Fatalf("unexpected note: %q", note)
	}
	if note := info.StartupNote(GlyphsFull); note != "" {
		t.Fatalf("full-mode note = %q, want empty", note)
	}
	if note := (Info{ConHost: false}).StartupNote(GlyphsASCII); note != "" {
		t.Fatalf("note for a normal terminal = %q, want empty", note)
	}
}

func TestGlyphModeDetection(t *testing.T) {
	cases := []struct {
		name string
		info Info
		goos string
		want GlyphMode
	}{
		{"windows-terminal", Info{WindowsTerminal: true, VT: true, Unicode: true}, "windows", GlyphsFull},
		{"conhost-truetype", Info{ConHost: true, VT: true, FontTrueType: true}, "windows", GlyphsCompat},
		{"conhost-raster", Info{ConHost: true, VT: true, FontTrueType: false}, "windows", GlyphsASCII},
		{"conhost-no-vt", Info{ConHost: true, VT: false, FontTrueType: true}, "windows", GlyphsASCII},
		{"linux-utf8", Info{Console: true, VT: true, Unicode: true, LocaleUTF8: true, Term: "xterm-256color"}, "linux", GlyphsFull},
		{"linux-tty", Info{Console: true, VT: true, Unicode: false, LocaleUTF8: true, Term: "linux"}, "linux", GlyphsASCII},
		{"linux-c-locale", Info{Console: true, VT: true, Unicode: false, LocaleUTF8: false, Term: "xterm-256color"}, "linux", GlyphsASCII},
		{"dumb", Info{Console: true, Term: "dumb"}, "linux", GlyphsASCII},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectGlyphsFor(tc.info, tc.goos); got != tc.want {
				t.Fatalf("detectGlyphsFor = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestColorModeDetection(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	cases := []struct {
		name string
		info Info
		goos string
		want ColorMode
	}{
		{"windows-terminal", Info{WindowsTerminal: true, Console: true, VT: true}, "windows", ColorTrueColor},
		{"conhost", Info{ConHost: true, Console: true, VT: true}, "windows", ColorTrueColor},
		{"conhost-no-vt", Info{ConHost: true, Console: true, VT: false}, "windows", ColorANSI16},
		{"linux-256", Info{Console: true, Term: "xterm-256color"}, "linux", ColorANSI256},
		{"linux-16", Info{Console: true, Term: "xterm"}, "linux", ColorANSI16},
		{"linux-true", Info{Console: true, Term: "xterm-256color", Program: "WezTerm"}, "linux", ColorTrueColor},
		{"linux-tty", Info{Console: true, Term: "linux"}, "linux", ColorANSI16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectColorFor(tc.info, tc.goos); got != tc.want {
				t.Fatalf("detectColorFor = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSummaryMentionsProgram(t *testing.T) {
	lines := strings.Join((Info{Program: "Windows Terminal", Unicode: true}).Summary(), "\n")
	if !strings.Contains(lines, "Windows Terminal") {
		t.Fatalf("summary does not mention the program: %s", lines)
	}
}

func TestClassifyPseudoConsole(t *testing.T) {
	info := Info{Program: "Windows console (conhost)", ConsoleClass: "PseudoConsoleWindow"}
	classifyConsole(&info)
	if info.ConHost {
		t.Fatal("pseudoconsole must not be treated as conhost")
	}
	if !info.WindowsTerminal {
		t.Fatal("pseudoconsole should count as Windows Terminal")
	}
	if info.Program != "Windows Terminal (pseudoconsole)" {
		t.Fatalf("program = %q", info.Program)
	}
	if runtime.GOOS == "windows" && !detectUnicode(info) {
		t.Fatal("pseudoconsole should support Unicode")
	}
}

func TestClassifyConHost(t *testing.T) {
	info := Info{Program: "Windows console (conhost)", ConsoleClass: "ConsoleWindowClass"}
	classifyConsole(&info)
	if !info.ConHost {
		t.Fatal("ConsoleWindowClass should be treated as conhost")
	}
	if runtime.GOOS == "windows" && detectUnicode(info) {
		t.Fatal("conhost should not be Unicode by default")
	}
}

func TestClassifyUnknownConHost(t *testing.T) {
	info := Info{Program: "Windows console (conhost)"}
	classifyConsole(&info)
	if !info.ConHost {
		t.Fatal("unknown windows host should fall back to conhost")
	}
}
