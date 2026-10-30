package ui

import (
	"strings"
	"testing"
	"time"

	"tuiplay/internal/download"
)

func TestDownloadListItemsGroupHeaders(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.jobs = []download.Job{
		{ID: 1, GroupID: 7, GroupTitle: "Night Drive", GroupKind: download.KindYouTubePlaylist, TrackNumber: 1, TrackTotal: 2},
		{ID: 2, GroupID: 7, GroupTitle: "Night Drive", GroupKind: download.KindYouTubePlaylist, TrackNumber: 2, TrackTotal: 2},
		{ID: 3},
	}
	items := m.downloadListItems()
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	if !items[0].header || !strings.Contains(items[0].title, "Night Drive") || !strings.Contains(items[0].title, "2 tracks") {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].header || items[1].jobIndex != 0 {
		t.Fatalf("second item = %+v", items[1])
	}
	if items[2].header || items[2].jobIndex != 1 {
		t.Fatalf("third item = %+v", items[2])
	}
	if items[3].header || items[3].jobIndex != 2 {
		t.Fatalf("fourth item = %+v", items[3])
	}

	m.downloadsCur = 1
	if got := m.selectedDownloadItem(items); got != 2 {
		t.Fatalf("selected item = %d, want 2", got)
	}
}

func TestPreviewModalShowsSummary(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.preview = &download.Preview{
		Kind:     download.KindYouTubePlaylist,
		Title:    "Night Drive",
		Subtitle: "Aurora",
		Tracks: []download.PreviewTrack{
			{Title: "Night Drive", Artist: "Aurora", Duration: 3 * time.Minute},
			{Title: "Sunset", Artist: "Aurora", Duration: 2 * time.Minute},
		},
		TotalDuration: 5 * time.Minute,
		Skipped:       1,
	}
	m.modal = modalPreview

	frame := m.renderPreviewModal(110, 32)
	for _, want := range []string{"PLAYLIST PREVIEW", "Night Drive", "2 tracks", "5:00", "1 skipped", "Sunset"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("preview frame does not contain %q:\n%s", want, frame)
		}
	}
}

func TestPreviewModalTruncatedWarning(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.preview = &download.Preview{
		Kind:      download.KindSoundCloudSet,
		Title:     "Late Night",
		Tracks:    []download.PreviewTrack{{Title: "One", URL: "https://soundcloud.com/a/one"}},
		Truncated: true,
	}
	m.modal = modalPreview
	frame := m.renderPreviewModal(110, 32)
	if !strings.Contains(frame, "Only the first 1 tracks") {
		t.Fatalf("missing truncation warning:\n%s", frame)
	}
}
