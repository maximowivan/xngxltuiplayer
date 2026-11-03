package download

import (
	"context"
	"strings"
	"testing"
	"time"
)

const youtubeSearchFixture = `{
  "entries": [
    {"id": "aaaaaaaaaaa", "title": "Daft Punk - Instant Crush (Official Video)",
     "uploader": "Daft Punk", "duration": 340,
     "thumbnails": [{"url": "https://img.example/instant.jpg"}]},
    {"id": "bbbbbbbbbbb", "title": "Daft Punk - Around the World",
     "uploader": "Daft Punk", "duration": 242},
    {"id": "ccccccccccc", "title": "Daft Punk - Live",
     "uploader": "Daft Punk", "duration": 300, "is_live": true},
    {"id": "ddddddddddd", "title": "Daft Punk - Upcoming",
     "uploader": "Daft Punk", "duration": 300, "live_status": "is_upcoming"},
    {"id": "aaaaaaaaaaa", "title": "Daft Punk - Instant Crush",
     "uploader": "Daft Punk", "duration": 340}
  ]
}`

func TestSearchYouTube(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		if !hasArgPrefix(args, "ytsearch5:") {
			t.Fatalf("search did not use ytsearch: %v", args)
		}
		return []byte(youtubeSearchFixture), nil
	}

	results, err := mgr.Search(context.Background(), "daft punk", SearchYouTube, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	first := results[0]
	if first.Title != "Instant Crush" || first.Artist != "Daft Punk" {
		t.Fatalf("first = %+v", first)
	}
	if first.URL != "https://www.youtube.com/watch?v=aaaaaaaaaaa" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.Kind != KindYouTube || first.Source != SearchYouTube {
		t.Fatalf("kind/source = %v/%v", first.Kind, first.Source)
	}
	if first.Duration != 340*time.Second {
		t.Fatalf("duration = %v", first.Duration)
	}
	if first.CoverURL != "https://img.example/instant.jpg" {
		t.Fatalf("cover = %q", first.CoverURL)
	}
}

func TestSearchSoundCloud(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		if !hasArgPrefix(args, "scsearch3:") {
			t.Fatalf("search did not use scsearch: %v", args)
		}
		return []byte(`{
			"entries": [
				{"title": "Around the World", "uploader": "Daft Punk", "duration": 429,
				 "webpage_url": "https://soundcloud.com/daftpunkofficialmusic/around-the-world",
				 "thumbnails": [{"url": "https://img.example/sc.jpg"}]},
				{"title": "Get Lucky", "uploader": "Daft Punk", "duration": 246,
				 "webpage_url": "https://soundcloud.com/example/get-lucky"}
			]
		}`), nil
	}

	results, err := mgr.Search(context.Background(), "daft punk", SearchSoundCloud, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Kind != KindSoundCloud || results[0].Source != SearchSoundCloud {
		t.Fatalf("kind/source = %v/%v", results[0].Kind, results[0].Source)
	}
	if results[0].URL != "https://soundcloud.com/daftpunkofficialmusic/around-the-world" {
		t.Fatalf("url = %q", results[0].URL)
	}
}

func TestSearchLimitAndEmptyQuery(t *testing.T) {
	mgr := newBatchTestManager(t)
	mgr.runJSON = func(ctx context.Context, args []string) ([]byte, error) {
		return []byte(youtubeSearchFixture), nil
	}
	results, err := mgr.Search(context.Background(), "daft punk", SearchYouTube, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("limited results = %d, want 1", len(results))
	}
	if _, err := mgr.Search(context.Background(), "   ", SearchYouTube, 5); err == nil {
		t.Fatal("empty query should fail")
	}
}

func TestEnqueueSearchResultUsesSeed(t *testing.T) {
	mgr := newBatchTestManager(t)
	result := SearchResult{
		URL:      "https://www.youtube.com/watch?v=aaaaaaaaaaa",
		Kind:     KindYouTube,
		Title:    "Instant Crush",
		Artist:   "Daft Punk",
		Duration: 340 * time.Second,
		CoverURL: "https://img.example/instant.jpg",
		Source:   SearchYouTube,
	}
	job, err := mgr.EnqueueResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if job.Title != "Instant Crush" || job.Artist != "Daft Punk" {
		t.Fatalf("job = %+v", job)
	}
	if len(mgr.queue) != 1 {
		t.Fatalf("queue = %d, want 1", len(mgr.queue))
	}
	if job.link.Seed == nil {
		t.Fatal("search result did not seed metadata")
	}
	if job.link.Seed.DownloadURL != result.URL || job.link.Seed.Duration != result.Duration {
		t.Fatalf("seed = %+v", job.link.Seed)
	}
}

func TestEnqueueResultsGroupsAndDeduplicates(t *testing.T) {
	mgr := newBatchTestManager(t)
	results := []SearchResult{
		{URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Kind: KindYouTube, Title: "One", Artist: "A"},
		{URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Kind: KindYouTube, Title: "One duplicate", Artist: "A"},
		{URL: "https://soundcloud.com/a/two", Kind: KindSoundCloud, Title: "Two", Artist: "B"},
	}
	batch, err := mgr.EnqueueResults(results, "Search: test")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Queued != 2 || batch.Duplicates != 1 || batch.GroupID == 0 {
		t.Fatalf("batch = %+v", batch)
	}
	jobs := mgr.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	for i, job := range jobs {
		if job.GroupID != batch.GroupID || job.GroupTitle != "Search: test" {
			t.Fatalf("job %d group = %d %q", i, job.GroupID, job.GroupTitle)
		}
		if job.TrackNumber != i+1 || job.TrackTotal != 2 {
			t.Fatalf("job %d track = %d/%d", i, job.TrackNumber, job.TrackTotal)
		}
		if job.link.Seed == nil {
			t.Fatalf("job %d has no seed", i)
		}
	}
}

func TestCleanSearchTitle(t *testing.T) {
	cases := map[string]string{
		"Instant Crush (Official Video) ft. Julian Casablancas": "Instant Crush ft. Julian Casablancas",
		"Song (Official Audio)":                                 "Song",
		"Song [Lyric Video]":                                    "Song",
		"Song (Official Music Video Remastered)":                "Song",
		"Song (Live)":                                           "Song (Live)",
	}
	for in, want := range cases {
		if got := cleanSearchTitle(in); got != want {
			t.Errorf("cleanSearchTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchSourceNext(t *testing.T) {
	if SearchYouTube.Next() != SearchSoundCloud {
		t.Fatal("YouTube should cycle to SoundCloud")
	}
	if SearchSoundCloud.Next() != SearchYouTube {
		t.Fatal("SoundCloud should cycle to YouTube")
	}
	if SearchYouTube.String() != "YouTube" || SearchSoundCloud.String() != "SoundCloud" {
		t.Fatal("unexpected source labels")
	}
}

func hasArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
