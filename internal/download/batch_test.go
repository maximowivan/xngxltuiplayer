package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const youtubePlaylistFixture = `{
  "id": "PL1234567890",
  "title": "Night Drive",
  "uploader": "Aurora",
  "thumbnails": [{"url": "https://img.example/playlist.jpg"}],
  "entries": [
    {
      "id": "aaaaaaaaaaa",
      "title": "Aurora - Night Drive",
      "uploader": "Aurora - Topic",
      "duration": 215,
      "thumbnails": [{"url": "https://img.example/a.jpg"}]
    },
    {"id": "", "title": "[Deleted video]", "uploader": ""},
    {
      "id": "bbbbbbbbbbb",
      "title": "Aurora - Sunset",
      "uploader": "Aurora - Topic",
      "duration": 180
    },
    {
      "id": "bbbbbbbbbbb",
      "title": "Aurora - Sunset",
      "uploader": "Aurora - Topic",
      "duration": 180
    }
  ]
}`

func newBatchTestManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	return &Manager{
		opts: Options{
			MusicDir: filepath.Join(base, "music"),
			DataDir:  filepath.Join(base, "data"),
		},
		tools:  Tools{YtDlp: "yt-dlp", FFmpeg: "ffmpeg"},
		ctx:    context.Background(),
		jobs:   make(map[int]*Job),
		queue:  make(chan int, 1024),
		active: make(map[int]context.CancelFunc),
	}
}

func TestPreviewYouTubePlaylist(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		if !containsArg(args, "--flat-playlist") {
			t.Fatalf("preview did not use --flat-playlist: %v", args)
		}
		return []byte(youtubePlaylistFixture), nil
	}

	preview, err := mgr.Preview(context.Background(), "https://www.youtube.com/playlist?list=PL1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Batch() {
		t.Fatal("playlist preview is not marked as a batch")
	}
	if preview.Title != "Night Drive" || preview.Subtitle != "Aurora" {
		t.Fatalf("title=%q subtitle=%q", preview.Title, preview.Subtitle)
	}
	if preview.CoverURL != "https://img.example/playlist.jpg" {
		t.Fatalf("cover = %q", preview.CoverURL)
	}
	if len(preview.Tracks) != 3 {
		t.Fatalf("tracks = %d, want 3", len(preview.Tracks))
	}
	if preview.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", preview.Skipped)
	}
	if preview.TotalDuration != 215*time.Second+2*180*time.Second {
		t.Fatalf("total duration = %v", preview.TotalDuration)
	}

	first := preview.Tracks[0]
	if first.Title != "Night Drive" || first.Artist != "Aurora" {
		t.Fatalf("first track = %+v", first)
	}
	if first.Album != "Night Drive" {
		t.Fatalf("album = %q", first.Album)
	}
	if first.URL != "https://www.youtube.com/watch?v=aaaaaaaaaaa" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.CoverURL != "https://img.example/a.jpg" {
		t.Fatalf("cover = %q", first.CoverURL)
	}
}

func TestPreviewSoundCloudSetHydration(t *testing.T) {
	const page = `<!doctype html><script>window.__sc_hydration = [
		{"hydratable":"anonymousId","data":"1-2-3"},
		{"hydratable":"playlist","data":{
			"title":"Late Night",
			"user":{"username":"DJ Example"},
			"artwork_url":"https://i1.sndcdn.com/artworks-set-large.jpg",
			"tracks":[
				{"title":"DJ Example - Track One","duration":60000,
				 "permalink_url":"https://soundcloud.com/dj-example/track-one",
				 "user":{"username":"DJ Example"},"release_date":"2024-05-01T00:00:00Z",
				 "artwork_url":"https://i1.sndcdn.com/artworks-one-large.jpg"},
				{"title":"Track Two","duration":90000,
				 "permalink_url":"https://soundcloud.com/dj-example/track-two",
				 "user":{"username":"DJ Example"}}
			]
		}}
	];</script>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	mgr := newBatchTestManager(t)
	mgr.client = server.Client()
	preview, err := mgr.previewSoundCloud(context.Background(), Link{
		Raw:  "https://soundcloud.com/dj-example/sets/late-night",
		URL:  server.URL,
		Kind: KindSoundCloudSet,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(preview.Tracks))
	}
	first := preview.Tracks[0]
	if first.Title != "Track One" || first.Artist != "DJ Example" {
		t.Fatalf("first track = %+v", first)
	}
	if first.Album != "Late Night" || first.Year != "2024" {
		t.Fatalf("album/year = %q/%q", first.Album, first.Year)
	}
	if first.Duration != time.Minute {
		t.Fatalf("duration = %v", first.Duration)
	}
	if first.URL != "https://soundcloud.com/dj-example/track-one" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.CoverURL != "https://i1.sndcdn.com/artworks-one-t500x500.jpg" {
		t.Fatalf("cover = %q", first.CoverURL)
	}
	if preview.CoverURL != "https://i1.sndcdn.com/artworks-set-t500x500.jpg" {
		t.Fatalf("set cover = %q", preview.CoverURL)
	}
	if preview.Tracks[1].Title != "Track Two" || preview.Tracks[1].Artist != "DJ Example" {
		t.Fatalf("second track = %+v", preview.Tracks[1])
	}
}

func TestParseSoundCloudHydrationRejectsBrokenPage(t *testing.T) {
	if _, err := parseSoundCloudHydration("<html>no data</html>"); err == nil {
		t.Fatal("expected a parse error")
	}
	if _, err := parseSoundCloudHydration(`__sc_hydration = [{"hydratable":"playlist","data":{}}];`); err == nil {
		t.Fatal("expected an empty playlist error")
	}
}

func TestPreviewRejectsEmptyPlaylist(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		return []byte(`{"title":"Empty","entries":[{"title":"[Private video]"}]}`), nil
	}
	_, err := mgr.Preview(context.Background(), "https://www.youtube.com/playlist?list=PL1234567890")
	if err == nil || !strings.Contains(err.Error(), "no playable tracks") {
		t.Fatalf("err = %v, want no playable tracks", err)
	}
}

func TestPreviewTruncatesLargePlaylist(t *testing.T) {
	entries := make([]map[string]any, 0, maxBatchTracks+5)
	for i := 0; i < maxBatchTracks+5; i++ {
		entries = append(entries, map[string]any{
			"id":       "aaaaaaaaaaa",
			"title":    "Track",
			"duration": 60,
		})
	}
	payload, err := json.Marshal(map[string]any{"title": "Huge", "entries": entries})
	if err != nil {
		t.Fatal(err)
	}

	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		return payload, nil
	}
	preview, err := mgr.Preview(context.Background(), "https://www.youtube.com/playlist?list=PL1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Tracks) != maxBatchTracks {
		t.Fatalf("tracks = %d, want %d", len(preview.Tracks), maxBatchTracks)
	}
	if !preview.Truncated {
		t.Fatal("large playlist was not marked as truncated")
	}
}

func TestEnqueuePreviewGroupsAndDeduplicates(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		return []byte(youtubePlaylistFixture), nil
	}
	preview, err := mgr.Preview(context.Background(), "https://www.youtube.com/playlist?list=PL1234567890")
	if err != nil {
		t.Fatal(err)
	}

	result, err := mgr.EnqueuePreview(preview)
	if err != nil {
		t.Fatal(err)
	}
	if result.Queued != 2 || result.Duplicates != 1 {
		t.Fatalf("result = %+v, want queued=2 duplicates=1", result)
	}
	if result.GroupID == 0 {
		t.Fatal("group id was not assigned")
	}
	if len(mgr.queue) != 2 {
		t.Fatalf("queue = %d, want 2", len(mgr.queue))
	}

	jobs := mgr.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	for i, job := range jobs {
		if job.GroupID != result.GroupID || job.GroupTitle != "Night Drive" {
			t.Fatalf("job %d group = %d %q", i, job.GroupID, job.GroupTitle)
		}
		if job.TrackNumber != i+1 || job.TrackTotal != 2 {
			t.Fatalf("job %d track = %d/%d", i, job.TrackNumber, job.TrackTotal)
		}
		if job.link.Seed == nil {
			t.Fatalf("job %d has no seed metadata", i)
		}
	}
	if jobs[0].link.Seed.Album != "Night Drive" || jobs[0].link.Seed.TrackNumber != 1 {
		t.Fatalf("seed = %+v", jobs[0].link.Seed)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
