package main

import (
	"testing"

	"tuiplay/internal/terminal"
	"tuiplay/internal/ui"
)

func TestParseIconMode(t *testing.T) {
	cases := []struct {
		value string
		ascii bool
		want  string
	}{
		{"", false, "auto"},
		{"auto", false, "auto"},
		{"unicode", false, "unicode"},
		{"ascii", false, "ascii"},
		{"ASCII", false, "ascii"},
		{"unicode", true, "ascii"},
	}
	for _, tc := range cases {
		got, err := parseIconMode(tc.value, tc.ascii)
		if err != nil {
			t.Fatalf("parseIconMode(%q, %v): %v", tc.value, tc.ascii, err)
		}
		if got != tc.want {
			t.Fatalf("parseIconMode(%q, %v) = %q, want %q", tc.value, tc.ascii, got, tc.want)
		}
	}
	if _, err := parseIconMode("emoji", false); err == nil {
		t.Fatal("expected an error for an unknown icon mode")
	}
}

func TestParseUIMode(t *testing.T) {
	cases := map[string]string{
		"":       "auto",
		"auto":   "auto",
		"full":   "full",
		"compat": "compat",
		"ascii":  "ascii",
		"ASCII":  "ascii",
	}
	for value, want := range cases {
		got, err := parseUIMode(value, false)
		if err != nil {
			t.Fatalf("parseUIMode(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("parseUIMode(%q) = %q, want %q", value, got, want)
		}
	}
	if got, err := parseUIMode("full", true); err != nil || got != "ascii" {
		t.Fatalf("parseUIMode(full, ascii) = %q, %v; want ascii", got, err)
	}
	if _, err := parseUIMode("pretty", false); err == nil {
		t.Fatal("expected an error for an unknown UI mode")
	}
}

func TestParseColorMode(t *testing.T) {
	for _, value := range []string{"", "auto", "truecolor", "256", "16", "none", "NONE"} {
		if _, err := parseColorMode(value); err != nil {
			t.Fatalf("parseColorMode(%q): %v", value, err)
		}
	}
	if _, err := parseColorMode("16m"); err == nil {
		t.Fatal("expected an error for an unknown colour mode")
	}
}

func TestParseWatchMode(t *testing.T) {
	for _, value := range []string{"", "auto", "on", "off", "OFF"} {
		if _, err := parseWatchMode(value); err != nil {
			t.Fatalf("parseWatchMode(%q): %v", value, err)
		}
	}
	if _, err := parseWatchMode("sometimes"); err == nil {
		t.Fatal("expected an error for an unknown watch mode")
	}
}

func TestParseSampleRate(t *testing.T) {
	for value, want := range map[string]int{"": 48000, "48000": 48000, "44100": 44100, " 44100 ": 44100} {
		got, err := parseSampleRate(value)
		if err != nil {
			t.Fatalf("parseSampleRate(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("parseSampleRate(%q) = %d, want %d", value, got, want)
		}
	}
	if _, err := parseSampleRate("96000"); err == nil {
		t.Fatal("expected an error for an unknown sample rate")
	}
}

func TestParseDownloadWorkers(t *testing.T) {
	for _, value := range []int{1, 2, 6} {
		if got, err := parseDownloadWorkers(value); err != nil || got != value {
			t.Fatalf("parseDownloadWorkers(%d) = %d, %v", value, got, err)
		}
	}
	for _, value := range []int{0, -1, 7} {
		if _, err := parseDownloadWorkers(value); err == nil {
			t.Fatalf("parseDownloadWorkers(%d) should fail", value)
		}
	}
}

func TestResolveGlyphAndIconModes(t *testing.T) {
	info := terminal.Info{Glyphs: terminal.GlyphsCompat, Unicode: false}
	if got := resolveGlyphMode("auto", info); got != ui.GlyphsCompat {
		t.Fatalf("auto glyph mode = %s, want compat", got)
	}
	if got := resolveGlyphMode("full", info); got != ui.GlyphsFull {
		t.Fatalf("forced full glyph mode = %s", got)
	}
	if !resolveIconASCII("auto", ui.GlyphsCompat) {
		t.Fatal("compat mode should select ASCII icons")
	}
	if resolveIconASCII("unicode", ui.GlyphsCompat) {
		t.Fatal("forced Unicode icons should override compat")
	}
	if resolveIconASCII("auto", ui.GlyphsFull) {
		t.Fatal("full mode should select Unicode icons")
	}
}

func TestResolveColorProfile(t *testing.T) {
	info := terminal.Info{Color: terminal.ColorANSI256}
	if got := resolveColorProfile("auto", info); got != ui.ColorANSI256 {
		t.Fatalf("auto color = %s, want 256", got)
	}
	if got := resolveColorProfile("none", info); got != ui.ColorNone {
		t.Fatalf("forced color = %s, want none", got)
	}
	if got := mapColorProfile(terminal.ColorTrueColor); got != ui.ColorTrueColor {
		t.Fatalf("mapped color = %s", got)
	}
}
