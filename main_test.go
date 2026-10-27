package main

import "testing"

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
