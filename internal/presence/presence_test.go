package presence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "discord.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"client_id":" 123456789012345678 ","activity_type":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load(dir)
	if !cfg.Configured() || cfg.ClientID != "123456789012345678" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.ActivityType != 2 {
		t.Fatalf("activity type = %d, want 2", cfg.ActivityType)
	}

	cfg.FallbackImage = "logo"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	again := Load(dir)
	if again.ClientID != cfg.ClientID || again.FallbackImage != "logo" || !again.Configured() {
		t.Fatalf("saved config = %+v", again)
	}
}

func TestDefaultConfigEnablesPresence(t *testing.T) {
	cfg := Load(t.TempDir())
	if !cfg.Enabled || cfg.ClientID != DefaultClientID || !cfg.Configured() {
		t.Fatalf("default config = %+v", cfg)
	}
}

func TestDisabledConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "discord.json"), []byte(`{"enabled":false,"client_id":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load(dir)
	if cfg.Enabled || cfg.Configured() {
		t.Fatalf("disabled config = %+v", cfg)
	}
	if cfg.ClientID != "" {
		t.Fatalf("disabled config should keep an empty client id, got %q", cfg.ClientID)
	}
}

func TestEmptyClientIDUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "discord.json"), []byte(`{"enabled":true,"client_id":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load(dir)
	if cfg.ClientID != DefaultClientID || !cfg.Configured() {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestBuildActivity(t *testing.T) {
	c := &Client{cfg: Config{ActivityType: 2}}
	playing := c.buildActivity(Activity{
		Title:    "Night Drive",
		Artist:   "Aurora",
		Album:    "Neon Skyline",
		CoverURL: "https://example.com/cover.jpg",
		Position: 42 * time.Second,
		Duration: 3*time.Minute + 21*time.Second,
	})
	if playing.Type != 2 || playing.Details != "Night Drive" {
		t.Fatalf("activity = %+v", playing)
	}
	if !strings.Contains(playing.State, "Aurora") || !strings.Contains(playing.State, "Neon Skyline") {
		t.Fatalf("state = %q", playing.State)
	}
	if playing.Timestamps == nil || playing.Timestamps.End-playing.Timestamps.Start != 201 {
		t.Fatalf("timestamps = %+v", playing.Timestamps)
	}
	if playing.Assets == nil || playing.Assets.LargeImage != "https://example.com/cover.jpg" {
		t.Fatalf("assets = %+v", playing.Assets)
	}

	paused := c.buildActivity(Activity{
		Title:    "Night Drive",
		Artist:   "Aurora",
		Position: 42 * time.Second,
		Duration: 3*time.Minute + 21*time.Second,
		Paused:   true,
	})
	if paused.Timestamps != nil {
		t.Fatal("paused activity should not tick")
	}
	if !strings.Contains(paused.State, "Paused 0:42 / 3:21") {
		t.Fatalf("paused state = %q", paused.State)
	}
}

func TestSourceArtwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://img.example/cover.jpg"></head></html>`))
	}))
	defer server.Close()

	got := lookupSourceArtwork(context.Background(), server.Client(), server.URL+"/track")
	if got != "https://img.example/cover.jpg" {
		t.Fatalf("artwork = %q", got)
	}
}

func TestYouTubeThumbnail(t *testing.T) {
	got := lookupSourceArtwork(context.Background(), http.DefaultClient, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if got != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Fatalf("thumbnail = %q", got)
	}
}

func TestDescribeError(t *testing.T) {
	if got := DescribeError(fmt.Errorf("open pipe: %w", os.ErrPermission)); !strings.Contains(got, "administrator") {
		t.Fatalf("permission error = %q", got)
	}
	if got := DescribeError(fmt.Errorf("open pipe: %w", os.ErrNotExist)); got != "Discord is not running" {
		t.Fatalf("missing pipe error = %q", got)
	}
	if got := DescribeError(nil); got != "" {
		t.Fatalf("nil error = %q", got)
	}
}

func TestRPCErrorMessage(t *testing.T) {
	got := rpcErrorMessage(json.RawMessage(`{"code":4000,"message":"Invalid activity"}`))
	if got != "Invalid activity (code 4000)" {
		t.Fatalf("message = %q", got)
	}
	if got := rpcErrorMessage(json.RawMessage(`null`)); got != "unknown error" {
		t.Fatalf("null message = %q", got)
	}
}

func TestArtworkSelection(t *testing.T) {
	songs := []itunesSong{
		{TrackName: "Night Drive (Karaoke)", ArtistName: "Karaoke Hits", ArtworkURL100: "https://x/100x100bb.jpg"},
		{TrackName: "Night Drive", ArtistName: "Aurora", ArtworkURL100: "https://x/100x100bb.jpg"},
	}
	got := chooseArtwork(songs, "Night Drive", "Aurora")
	if got != "https://x/600x600bb.jpg" {
		t.Fatalf("artwork = %q", got)
	}
}
