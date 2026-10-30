package download

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCleanOEmbedTitle(t *testing.T) {
	tests := []struct {
		title, author, want string
	}{
		{"Virtual Riot - Believe What You Want by Monstercat", "Monstercat", "Virtual Riot - Believe What You Want"},
		{"Night Drive by Aurora", "Aurora", "Night Drive"},
		{"Night Drive", "Aurora", "Night Drive"},
		{"Song by Someone Else", "Aurora", "Song by Someone Else"},
	}
	for _, tt := range tests {
		if got := cleanOEmbedTitle(tt.title, tt.author); got != tt.want {
			t.Errorf("cleanOEmbedTitle(%q, %q) = %q, want %q", tt.title, tt.author, got, tt.want)
		}
	}
}

func TestSoundCloudTrackResolvesMinimalEntry(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/oembed") {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		body := `{"title":"DJ Example - Track Two by DJ Example","author_name":"DJ Example","thumbnail_url":"https://i1.sndcdn.com/artworks-two-large.jpg"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	track, ok := mgr.soundCloudTrack(context.Background(), scTrack{ID: 42, Duration: 0})
	if !ok {
		t.Fatal("minimal SoundCloud entry was not resolved")
	}
	if track.Title != "Track Two" || track.Artist != "DJ Example" {
		t.Fatalf("track = %+v", track)
	}
	if track.URL != "https://api.soundcloud.com/tracks/42" {
		t.Fatalf("url = %q", track.URL)
	}
	if track.CoverURL != "https://i1.sndcdn.com/artworks-two-t500x500.jpg" {
		t.Fatalf("cover = %q", track.CoverURL)
	}
	if track.Duration != 0 {
		t.Fatalf("duration = %v, want unknown", track.Duration)
	}
}

func TestExpandSoundCloudShortLink(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://soundcloud.com/dj-example/sets/late-night"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	link, err := mgr.expandSoundCloudShort(context.Background(), "https://on.soundcloud.com/abc")
	if err != nil {
		t.Fatal(err)
	}
	if link.Kind != KindSoundCloudSet || link.URL != "https://soundcloud.com/dj-example/sets/late-night" {
		t.Fatalf("link = %+v", link)
	}
}

func TestSoundCloudPlaylistFallsBackToYtDlp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>no hydration here</html>"))
	}))
	defer server.Close()

	mgr := newBatchTestManager(t)
	mgr.client = server.Client()
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		if !containsArg(args, "--flat-playlist") {
			t.Fatalf("fallback did not use --flat-playlist: %v", args)
		}
		return []byte(`{"title":"Late Night","uploader":"DJ Example","entries":[{"id":"42","title":""}]}`), nil
	}

	playlist, err := mgr.fetchSoundCloudPlaylist(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlist.Tracks) != 1 || playlist.Tracks[0].ID != 42 {
		t.Fatalf("playlist = %+v", playlist)
	}
}

func TestSoundCloudTrackKeepsFullEntry(t *testing.T) {
	mgr := newBatchTestManager(t)
	track, ok := mgr.soundCloudTrack(context.Background(), scTrack{
		ID:           7,
		Title:        "Artist - Song",
		Duration:     125000,
		PermalinkURL: "https://soundcloud.com/artist/song",
		User:         scUser{Username: "Artist"},
		ReleaseDate:  "2023-04-01T00:00:00Z",
	})
	if !ok {
		t.Fatal("full SoundCloud entry was skipped")
	}
	if track.Title != "Song" || track.Artist != "Artist" {
		t.Fatalf("track = %+v", track)
	}
	if track.URL != "https://soundcloud.com/artist/song" {
		t.Fatalf("url = %q", track.URL)
	}
	if track.Duration != 125*time.Second || track.Year != "2023" {
		t.Fatalf("duration/year = %v/%q", track.Duration, track.Year)
	}
}
