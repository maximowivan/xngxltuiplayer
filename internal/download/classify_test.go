package download

import (
	"os"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		kind    Kind
		url     string
		wantErr string
	}{
		{
			name: "spotify track",
			raw:  "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC?si=abc",
			kind: KindSpotifyTrack,
			url:  "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC",
		},
		{
			name: "spotify intl",
			raw:  "https://open.spotify.com/intl-ru/track/4uLU6hMCjMI75M1A2tKUQC",
			kind: KindSpotifyTrack,
			url:  "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC",
		},
		{
			name: "spotify uri",
			raw:  "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
			kind: KindSpotifyTrack,
			url:  "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC",
		},
		{
			name:    "spotify album rejected",
			raw:     "https://open.spotify.com/album/4uLU6hMCjMI75M1A2tKUQC",
			wantErr: "not supported yet",
		},
		{
			name: "soundcloud track",
			raw:  "https://soundcloud.com/artist/night-drive?in=artist/sets/album#t=10",
			kind: KindSoundCloud,
			url:  "https://soundcloud.com/artist/night-drive",
		},
		{
			name: "soundcloud set",
			raw:  "https://soundcloud.com/artist/sets/night-drive?si=abc",
			kind: KindSoundCloudSet,
			url:  "https://soundcloud.com/artist/sets/night-drive",
		},
		{
			name:    "soundcloud profile rejected",
			raw:     "https://soundcloud.com/artist",
			wantErr: "specific SoundCloud track",
		},
		{
			name:    "soundcloud profile tab rejected",
			raw:     "https://soundcloud.com/artist/tracks",
			wantErr: "specific SoundCloud track or set",
		},
		{
			name:    "soundcloud extra path rejected",
			raw:     "https://soundcloud.com/artist/night-drive/extra",
			wantErr: "specific SoundCloud track",
		},
		{
			name: "youtube short",
			raw:  "https://youtu.be/dQw4w9WgXcQ?t=42",
			kind: KindYouTube,
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name: "youtube shorts",
			raw:  "https://www.youtube.com/shorts/dQw4w9WgXcQ",
			kind: KindYouTube,
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name: "direct audio",
			raw:  "https://example.com/files/song.flac?token=1",
			kind: KindDirect,
			url:  "https://example.com/files/song.flac?token=1",
		},
		{
			name: "generic page",
			raw:  "https://example.com/song",
			kind: KindGeneric,
			url:  "https://example.com/song",
		},
		{
			name:    "spotify playlist rejected",
			raw:     "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
			wantErr: "not supported",
		},
		{
			name: "youtube playlist",
			raw:  "https://www.youtube.com/playlist?list=PL1234567890&si=abc",
			kind: KindYouTubePlaylist,
			url:  "https://www.youtube.com/playlist?list=PL1234567890",
		},
		{
			name: "youtube music playlist",
			raw:  "https://music.youtube.com/playlist?list=OLAK5uy_abc12345",
			kind: KindYouTubePlaylist,
			url:  "https://www.youtube.com/playlist?list=OLAK5uy_abc12345",
		},
		{
			name: "youtube embed playlist",
			raw:  "https://www.youtube.com/embed/videoseries?list=PL1234567890",
			kind: KindYouTubePlaylist,
			url:  "https://www.youtube.com/playlist?list=PL1234567890",
		},
		{
			name:    "empty",
			raw:     "   ",
			wantErr: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, err := Classify(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if link.Kind != tt.kind {
				t.Fatalf("kind = %v, want %v", link.Kind, tt.kind)
			}
			if link.URL != tt.url {
				t.Fatalf("url = %q, want %q", link.URL, tt.url)
			}
		})
	}
}

func TestTrackFilename(t *testing.T) {
	tests := []struct {
		meta Meta
		want string
	}{
		{Meta{Title: "Night Drive", Artist: "Aurora"}, "Aurora - Night Drive.mp3"},
		{Meta{Title: "Aurora - Night Drive", Artist: "Aurora"}, "Aurora - Night Drive.mp3"},
		{Meta{Title: "Aurora – Night Drive", Artist: "Aurora"}, "Aurora – Night Drive.mp3"},
		{Meta{Title: `Bad<>:"/\|?* Name`, Artist: "AC/DC"}, "AC DC - Bad Name.mp3"},
		{Meta{Title: "CON", Artist: ""}, "_CON.mp3"},
		{Meta{Title: "  ", Artist: ""}, "Untitled.mp3"},
		{Meta{Title: "Song: Live", Artist: "Band"}, "Band - Song Live.mp3"},
	}
	for _, tt := range tests {
		if got := trackFilename(tt.meta); got != tt.want {
			t.Errorf("trackFilename(%+v) = %q, want %q", tt.meta, got, tt.want)
		}
	}
}

func TestUniqueFilePath(t *testing.T) {
	dir := t.TempDir()
	first := uniqueFilePath(dir, "song.mp3")
	if first == "" || !strings.HasSuffix(first, "song.mp3") {
		t.Fatalf("unexpected first path: %q", first)
	}
	if err := writeTestFile(first, "one"); err != nil {
		t.Fatal(err)
	}
	second := uniqueFilePath(dir, "song.mp3")
	if second == first {
		t.Fatal("second path should differ")
	}
	if !strings.HasSuffix(second, "song (2).mp3") {
		t.Fatalf("unexpected second path: %q", second)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
