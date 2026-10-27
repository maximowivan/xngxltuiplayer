package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.2.0", "0.2.0", false},
		{"v0.2.0", "0.2.0-dev", true},
		{"v0.2.1", "0.2.0", true},
		{"v0.2.0", "0.3.0", false},
		{"v0.3.0", "0.2.9", true},
		{"v1.0.0", "0.9.9", true},
		{"v0.2.0-beta", "0.2.0", false},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestVersionParts(t *testing.T) {
	got := versionParts("v1.2.3-beta+meta")
	if got != [3]int{1, 2, 3} {
		t.Fatalf("versionParts = %v", got)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("v0.2.0 beta"); got != "v0.2.0-beta" {
		t.Fatalf("sanitize = %q", got)
	}
}
