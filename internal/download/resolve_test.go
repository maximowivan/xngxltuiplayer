package download

import (
	"testing"
	"time"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Night Drive", "night drive"},
		{"Night Drive (Official Audio)", "night drive"},
		{"Song [Remastered]", "song"},
		{"Track (feat. Someone)", "track"},
		{"Track feat. Someone", "track"},
		{"A.B.C!", "a b c"},
	}
	for _, tt := range tests {
		if got := normalizeTitle(tt.in); got != tt.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanVideoSuffix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Night Drive (Official Video)", "Night Drive"},
		{"Night Drive [Official Audio]", "Night Drive"},
		{"Night Drive (Lyric Video)", "Night Drive"},
		{"Night Drive (Official Music Video) (4K)", "Night Drive"},
		{"Night Drive (Video Oficial)", "Night Drive"},
		{"Night Drive (Audio Oficial)", "Night Drive"},
		{"Night Drive (Performance Video)", "Night Drive"},
		{"Night Drive ( Video Lyric )", "Night Drive"},
		{"Night Drive (Live)", "Night Drive (Live)"},
	}
	for _, tt := range tests {
		if got := cleanVideoSuffix(tt.in); got != tt.want {
			t.Errorf("cleanVideoSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripArtistPrefix(t *testing.T) {
	tests := []struct {
		title, artist, want string
	}{
		{"Aurora - Night Drive", "Aurora", "Night Drive"},
		{"Aurora – Night Drive", "Aurora", "Night Drive"},
		{"Aurora: Night Drive", "Aurora", "Night Drive"},
		{"Night Drive", "Aurora", "Night Drive"},
		{"Aurora - Aurora", "Aurora", "Aurora"},
		{"Arcángel - FN8 (Video Lyric)", "Arcangel", "FN8 (Video Lyric)"},
		{"BELLAKEO (Video Oficial) - Peso Pluma, Anitta", "Peso Pluma", "BELLAKEO (Video Oficial)"},
		{"Aurora - ", "Aurora", "Aurora - "},
	}
	for _, tt := range tests {
		if got := stripArtistPrefix(tt.title, tt.artist); got != tt.want {
			t.Errorf("stripArtistPrefix(%q, %q) = %q, want %q", tt.title, tt.artist, got, tt.want)
		}
	}
}

func TestCleanArtist(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Aurora", "Aurora"},
		{"Aurora - Topic", "Aurora"},
		{"  Aurora   -   Topic  ", "Aurora"},
		{"Aurora - Live", "Aurora - Live"},
		{"- Topic", "- Topic"},
	}
	for _, tt := range tests {
		if got := cleanArtist(tt.in); got != tt.want {
			t.Errorf("cleanArtist(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMetaFromInfo(t *testing.T) {
	meta, err := metaFromInfo(&ytInfo{
		Title:      "Aurora - Night Drive (Official Audio)",
		Uploader:   "Aurora - Topic",
		Duration:   201,
		WebpageURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Night Drive" {
		t.Fatalf("title = %q", meta.Title)
	}
	if meta.Artist != "Aurora" {
		t.Fatalf("artist = %q", meta.Artist)
	}
	if meta.Duration != 201*time.Second {
		t.Fatalf("duration = %v", meta.Duration)
	}
}

func TestChooseSearchEntry(t *testing.T) {
	entries := []ytInfo{
		{Title: "Night Drive (Karaoke Version)", Uploader: "Karaoke Hits", Duration: 200},
		{Title: "Aurora - Night Drive (Official Audio)", Uploader: "Aurora", Duration: 210},
		{Title: "Night Drive", Uploader: "Some Uploader", Duration: 20},
	}
	got := chooseSearchEntry(entries, "Aurora Night Drive audio")
	if got.Title != "Aurora - Night Drive (Official Audio)" {
		t.Fatalf("picked %q", got.Title)
	}
}

func TestParseProgressLine(t *testing.T) {
	tests := []struct {
		line     string
		progress float64
		ok       bool
	}{
		{"TUI  42.5%|3.2MiB/s|00:12", 0.425, true},
		{"[download]  10.0% of 4.00MiB at 1.00MiB/s ETA 00:03", 0.1, true},
		{"[download] Destination: song.webm", 0, false},
		{"some unrelated output", 0, false},
	}
	for _, tt := range tests {
		p, detail, ok := parseProgressLine(tt.line)
		if ok != tt.ok {
			t.Errorf("parseProgressLine(%q) ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if p != tt.progress {
			t.Errorf("parseProgressLine(%q) progress = %v, want %v", tt.line, p, tt.progress)
		}
		if detail == "" {
			t.Errorf("parseProgressLine(%q) returned empty detail", tt.line)
		}
	}
}

func TestUpgradeArtwork(t *testing.T) {
	in := "https://is1-ssl.mzstatic.com/image/thumb/x/100x100bb.jpg"
	want := "https://is1-ssl.mzstatic.com/image/thumb/x/900x900bb.jpg"
	if got := upgradeArtwork(in); got != want {
		t.Fatalf("upgradeArtwork = %q, want %q", got, want)
	}
	if got := upgradeArtwork(""); got != "" {
		t.Fatalf("empty artwork = %q", got)
	}
}
