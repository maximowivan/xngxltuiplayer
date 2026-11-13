package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/download"
)

func sendTestKey(m *Model, msg tea.KeyMsg) {
	m.Update(msg)
}

func TestDownloadPlaylistPickerTogglesAndReturns(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Chill"); err != nil {
		t.Fatal(err)
	}
	m.openDownloadPlaylistPicker(modalAddLink)
	if m.modal != modalDownloadPlaylists {
		t.Fatalf("modal = %v, want picker", m.modal)
	}

	sendTestKey(m, teaKeyRunes(" "))
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyDown})
	sendTestKey(m, teaKeyRunes(" "))
	if len(m.downloadPlaylists) != 2 {
		t.Fatalf("selected = %v, want two playlists", m.downloadPlaylists)
	}
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modal != modalAddLink {
		t.Fatalf("modal = %v, want add link", m.modal)
	}
	if len(m.downloadPlaylists) != 2 {
		t.Fatalf("selection was not applied: %v", m.downloadPlaylists)
	}
}

func TestDownloadPlaylistPickerEscCancels(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	m.openDownloadPlaylistPicker(modalAddLink)
	sendTestKey(m, teaKeyRunes(" "))
	sendTestKey(m, teaKeyRunes("c"))
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.downloadPlaylists) != 0 {
		t.Fatalf("Esc did not restore selection: %v", m.downloadPlaylists)
	}
}

func TestDownloadPlaylistPickerEmptyState(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openDownloadPlaylistPicker(modalAddLink)
	frame := m.renderDownloadPlaylistModal(110, 32)
	for _, want := range []string{"SAVE TO", "No playlists yet", "enter save"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("picker is missing %q:\n%s", want, frame)
		}
	}
}

func TestSaveToLineInAddLinkAndPreview(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.downloadPlaylists = []string{"Mix", "Chill"}

	linkFrame := m.renderAddLinkModal(110, 32)
	if !strings.Contains(linkFrame, "Save to:") || !strings.Contains(linkFrame, "Mix, Chill") {
		t.Fatalf("add-link Save to line is missing:\n%s", linkFrame)
	}

	m.preview = &download.Preview{
		Kind:  download.KindYouTubePlaylist,
		Title: "Night Drive",
		Tracks: []download.PreviewTrack{
			{Title: "Night Drive", Artist: "Aurora", Duration: 3 * 60},
		},
	}
	previewFrame := m.renderPreviewModal(110, 32)
	if !strings.Contains(previewFrame, "Save to:") || !strings.Contains(previewFrame, "Mix, Chill") {
		t.Fatalf("preview Save to line is missing:\n%s", previewFrame)
	}

	m.downloadPlaylists = nil
	if frame := m.renderAddLinkModal(110, 32); !strings.Contains(frame, "Save to:") || !strings.Contains(frame, "none") {
		t.Fatalf("empty Save to line is missing:\n%s", frame)
	}
}

func TestDownloadPlaylistShortcuts(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()

	m.openAddLink()
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyF4})
	if m.modal != modalDownloadPlaylists {
		t.Fatalf("F4 modal = %v, want picker", m.modal)
	}
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	m.preview = &download.Preview{Kind: download.KindYouTubePlaylist, Title: "Mix"}
	m.modal = modalPreview
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.modal != modalDownloadPlaylists {
		t.Fatalf("Ctrl+P modal = %v, want picker", m.modal)
	}
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	m.modal = modalDownloads
	sendTestKey(m, tea.KeyMsg{Type: tea.KeyF4})
	if m.modal != modalDownloadPlaylists {
		t.Fatalf("downloads F4 modal = %v, want picker", m.modal)
	}
}

func TestSingleDownloadAddsImmediately(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.cfg.MusicDir, "single.mp3")
	m.jobPlaylists[1] = []string{"Mix"}
	m.jobSeen[1] = download.StatusQueued

	jobs := []download.Job{{ID: 1, Status: download.StatusQueued}}
	m.processDownloadJobs(jobs)
	jobs[0].Status = download.StatusDone
	jobs[0].Path = path
	m.processDownloadJobs(jobs)

	pl, ok := m.playlists.Get("Mix")
	if !ok || len(pl.Tracks) != 1 || pl.Tracks[0] != filepath.Clean(path) {
		t.Fatalf("playlist = %+v, want single track", pl)
	}
}

func TestDownloadGroupCompletesBeforeFirstPoll(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(m.cfg.MusicDir, "fast-one.mp3")
	path2 := filepath.Join(m.cfg.MusicDir, "fast-two.mp3")
	m.groupPlaylists[15] = []string{"Mix"}
	jobs := []download.Job{
		{ID: 1, GroupID: 15, TrackNumber: 1, Status: download.StatusDone, Path: path1},
		{ID: 2, GroupID: 15, TrackNumber: 2, Status: download.StatusDone, Path: path2},
	}
	m.processDownloadJobs(jobs)

	pl, ok := m.playlists.Get("Mix")
	if !ok || len(pl.Tracks) != 2 {
		t.Fatalf("fast group was not saved: %+v", pl)
	}
	if pl.Tracks[0] != filepath.Clean(path1) || pl.Tracks[1] != filepath.Clean(path2) {
		t.Fatalf("tracks = %v, want source order", pl.Tracks)
	}
}

func TestDownloadGroupWaitsAndKeepsTrackOrder(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(m.cfg.MusicDir, "one.mp3")
	path2 := filepath.Join(m.cfg.MusicDir, "two.mp3")
	path3 := filepath.Join(m.cfg.MusicDir, "three.mp3")
	m.groupPlaylists[7] = []string{"Mix"}

	jobs := []download.Job{
		{ID: 1, GroupID: 7, TrackNumber: 3, Status: download.StatusQueued},
		{ID: 2, GroupID: 7, TrackNumber: 1, Status: download.StatusQueued},
		{ID: 3, GroupID: 7, TrackNumber: 2, Status: download.StatusQueued},
	}
	for _, job := range jobs {
		m.jobSeen[job.ID] = download.StatusQueued
	}
	m.processDownloadJobs(jobs)

	jobs[0].Status = download.StatusDone
	jobs[0].Path = path1
	m.processDownloadJobs(jobs)
	if pl, _ := m.playlists.Get("Mix"); len(pl.Tracks) != 0 {
		t.Fatalf("group was saved too early: %+v", pl.Tracks)
	}

	jobs[1].Status = download.StatusDone
	jobs[1].Path = path2
	jobs[2].Status = download.StatusDone
	jobs[2].Path = path3
	m.processDownloadJobs(jobs)

	pl, ok := m.playlists.Get("Mix")
	if !ok {
		t.Fatal("playlist disappeared")
	}
	want := []string{filepath.Clean(path2), filepath.Clean(path3), filepath.Clean(path1)}
	if len(pl.Tracks) != len(want) {
		t.Fatalf("tracks = %v, want %v", pl.Tracks, want)
	}
	for i := range want {
		if pl.Tracks[i] != want[i] {
			t.Fatalf("tracks = %v, want %v", pl.Tracks, want)
		}
	}
}

func TestDownloadGroupPartialFailureAddsSuccessfulTracks(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.cfg.MusicDir, "kept.mp3")
	m.groupPlaylists[9] = []string{"Mix"}
	jobs := []download.Job{
		{ID: 1, GroupID: 9, TrackNumber: 1, Status: download.StatusQueued},
		{ID: 2, GroupID: 9, TrackNumber: 2, Status: download.StatusQueued},
	}
	for _, job := range jobs {
		m.jobSeen[job.ID] = download.StatusQueued
	}
	m.processDownloadJobs(jobs)
	jobs[0].Status = download.StatusDone
	jobs[0].Path = path
	jobs[1].Status = download.StatusFailed
	m.processDownloadJobs(jobs)

	pl, _ := m.playlists.Get("Mix")
	if len(pl.Tracks) != 1 || pl.Tracks[0] != filepath.Clean(path) {
		t.Fatalf("tracks = %v, want successful track only", pl.Tracks)
	}
}

func TestDownloadGroupRetryAddsLateTrackOnce(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(m.cfg.MusicDir, "first.mp3")
	path2 := filepath.Join(m.cfg.MusicDir, "retry.mp3")
	m.groupPlaylists[11] = []string{"Mix"}
	jobs := []download.Job{
		{ID: 1, GroupID: 11, TrackNumber: 1, Status: download.StatusQueued},
		{ID: 2, GroupID: 11, TrackNumber: 2, Status: download.StatusQueued},
	}
	for _, job := range jobs {
		m.jobSeen[job.ID] = download.StatusQueued
	}
	m.processDownloadJobs(jobs)
	jobs[0].Status = download.StatusDone
	jobs[0].Path = path1
	jobs[1].Status = download.StatusFailed
	m.processDownloadJobs(jobs)

	jobs[1].Status = download.StatusQueued
	m.processDownloadJobs(jobs)
	jobs[1].Status = download.StatusDone
	jobs[1].Path = path2
	m.processDownloadJobs(jobs)

	pl, _ := m.playlists.Get("Mix")
	if len(pl.Tracks) != 2 {
		t.Fatalf("tracks = %v, want two tracks", pl.Tracks)
	}
	if pl.Tracks[1] != filepath.Clean(path2) {
		t.Fatalf("retry path = %q, want %q", pl.Tracks[1], filepath.Clean(path2))
	}
}

func TestDownloadGroupKeepsDoneTracksAfterClear(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(m.cfg.MusicDir, "before-clear.mp3")
	path2 := filepath.Join(m.cfg.MusicDir, "after-clear.mp3")
	m.groupPlaylists[13] = []string{"Mix"}
	jobs := []download.Job{
		{ID: 1, GroupID: 13, TrackNumber: 1, Status: download.StatusQueued},
		{ID: 2, GroupID: 13, TrackNumber: 2, Status: download.StatusQueued},
	}
	for _, job := range jobs {
		m.jobSeen[job.ID] = download.StatusQueued
	}
	m.processDownloadJobs(jobs)
	jobs[0].Status = download.StatusDone
	jobs[0].Path = path1
	m.processDownloadJobs(jobs)

	// Simulate C removing the finished first job while the second is active.
	m.jobs = []download.Job{jobs[1]}
	m.pruneDownloadState()
	jobs[1].Status = download.StatusDone
	jobs[1].Path = path2
	m.processDownloadJobs([]download.Job{jobs[1]})

	pl, _ := m.playlists.Get("Mix")
	if len(pl.Tracks) != 2 {
		t.Fatalf("tracks = %v, want both tracks", pl.Tracks)
	}
	if pl.Tracks[0] != filepath.Clean(path1) || pl.Tracks[1] != filepath.Clean(path2) {
		t.Fatalf("tracks = %v, want source order", pl.Tracks)
	}
}

func TestDownloadDestinationsRenameAndDelete(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.downloadPlaylists = []string{"Mix", "Chill"}
	m.jobPlaylists[1] = []string{"Mix", "Chill"}
	m.groupPlaylists[2] = []string{"Mix"}

	m.renameDownloadDestination("Mix", "Mix 2")
	if m.downloadPlaylists[0] != "Mix 2" || m.jobPlaylists[1][0] != "Mix 2" || m.groupPlaylists[2][0] != "Mix 2" {
		t.Fatalf("rename did not update destinations: %v %v %v", m.downloadPlaylists, m.jobPlaylists, m.groupPlaylists)
	}

	m.dropDownloadDestination("Chill")
	if len(m.downloadPlaylists) != 1 || len(m.jobPlaylists[1]) != 1 {
		t.Fatalf("delete did not clear destinations: %v %v", m.downloadPlaylists, m.jobPlaylists)
	}
}

func TestDownloadDestinationRenameBeforeCompletion(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.playlists.Create("Chill"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.cfg.MusicDir, "renamed.mp3")
	m.downloadPlaylists = []string{"Mix"}
	m.groupPlaylists[17] = []string{"Mix"}
	m.renameDownloadDestination("Mix", "Chill")
	jobs := []download.Job{{
		ID:          1,
		GroupID:     17,
		TrackNumber: 1,
		Status:      download.StatusDone,
		Path:        path,
	}}
	m.processDownloadJobs(jobs)

	if pl, _ := m.playlists.Get("Mix"); len(pl.Tracks) != 0 {
		t.Fatalf("old playlist got tracks: %v", pl.Tracks)
	}
	pl, _ := m.playlists.Get("Chill")
	if len(pl.Tracks) != 1 || pl.Tracks[0] != filepath.Clean(path) {
		t.Fatalf("renamed playlist = %v, want track", pl.Tracks)
	}
}

func TestDownloadDestinationDeleteBeforeCompletion(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	if _, err := m.playlists.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	m.downloadPlaylists = []string{"Mix"}
	m.groupPlaylists[19] = []string{"Mix"}
	m.dropDownloadDestination("Mix")
	jobs := []download.Job{{
		ID:          1,
		GroupID:     19,
		TrackNumber: 1,
		Status:      download.StatusDone,
		Path:        filepath.Join(m.cfg.MusicDir, "deleted-destination.mp3"),
	}}
	m.processDownloadJobs(jobs)

	if pl, _ := m.playlists.Get("Mix"); len(pl.Tracks) != 0 {
		t.Fatalf("deleted destination received tracks: %v", pl.Tracks)
	}
}

func TestDownloadsShowSaveDestination(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.jobPlaylists[1] = []string{"Mix"}
	status := m.downloadStatusText(download.Job{ID: 1, Status: download.StatusDone})
	if !strings.Contains(status, "Mix") || !strings.Contains(status, currentGlyphs.Right) {
		t.Fatalf("status = %q, want destination", status)
	}

	m.groupPlaylists[7] = []string{"Chill"}
	m.jobs = []download.Job{{
		ID:          2,
		GroupID:     7,
		GroupTitle:  "Search: night",
		TrackNumber: 1,
		TrackTotal:  2,
	}}
	items := m.downloadListItems()
	if len(items) == 0 || !items[0].header || !strings.Contains(items[0].title, "Chill") {
		t.Fatalf("group header = %+v, want destination", items)
	}
}
