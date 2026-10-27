package terminal

import (
	"strings"
	"testing"
)

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
	info := Info{ConHost: true}
	if note := info.StartupNote(true); !strings.Contains(note, "Windows Terminal") {
		t.Fatalf("unexpected note: %q", note)
	}
	if note := info.StartupNote(false); !strings.Contains(note, "icons look wrong") {
		t.Fatalf("unexpected note: %q", note)
	}
	if note := (Info{ConHost: false}).StartupNote(true); note != "" {
		t.Fatalf("note for a normal terminal = %q, want empty", note)
	}
}

func TestSummaryMentionsProgram(t *testing.T) {
	lines := strings.Join((Info{Program: "Windows Terminal", Unicode: true}).Summary(), "\n")
	if !strings.Contains(lines, "Windows Terminal") {
		t.Fatalf("summary does not mention the program: %s", lines)
	}
}
