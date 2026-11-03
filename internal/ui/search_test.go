package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/download"
)

func TestLooksLikeLink(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube.com/watch?v=abc": true,
		"youtube.com/watch?v=abc":             true,
		"youtu.be/abc":                        true,
		"music.yandex.ru/album/1/track/2":     true,
		"open.spotify.com/track/abc":          true,
		"spotify:track:abc":                   true,
		"song.mp3":                            true,
		"daft punk":                           false,
		"daftpunk":                            false,
		"AC/DC":                               false,
		"M.O.O.N.":                            false,
		"":                                    false,
	}
	for raw, want := range cases {
		if got := looksLikeLink(raw); got != want {
			t.Errorf("looksLikeLink(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestSearchResultsInAddModal(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkInput.SetValue("daft punk")
	m.linkSource = download.SearchYouTube
	m.linkResults = []download.SearchResult{
		{Title: "Instant Crush", Artist: "Daft Punk", Duration: 5*time.Minute + 40*time.Second, Source: download.SearchYouTube},
		{Title: "Around the World", Artist: "Daft Punk", Duration: 4*time.Minute + 2*time.Second, Source: download.SearchYouTube},
	}
	m.linkQuery = "daft punk"
	m.modalErr = ""

	frame := m.renderAddLinkModal(110, 32)
	for _, want := range []string{"YouTube", "2 results", "Instant Crush", "Daft Punk", "5:40", "Tab"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("search modal does not contain %q:\n%s", want, frame)
		}
	}
}

func TestSearchResultsScrollAndNarrowSizes(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkSource = download.SearchYouTube
	for i := 0; i < 12; i++ {
		m.linkResults = append(m.linkResults, download.SearchResult{
			Title:    "Track",
			Artist:   "Artist",
			Duration: time.Minute,
			Source:   download.SearchYouTube,
		})
	}
	m.linkCursor = 10
	for _, size := range [][2]int{{110, 32}, {84, 24}, {60, 18}} {
		frame := m.renderAddLinkModal(size[0], size[1])
		if !strings.Contains(frame, "FIND MUSIC") {
			t.Fatalf("%dx%d: missing title:\n%s", size[0], size[1], frame)
		}
		if !strings.Contains(frame, "11.") {
			t.Fatalf("%dx%d: selected result is not visible:\n%s", size[0], size[1], frame)
		}
	}
}

func TestSearchLoadedAndEsc(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkSeq = 3
	m.linkLoading = true
	results := []download.SearchResult{{Title: "A", Artist: "B"}}
	m.handleSearchLoaded(searchLoadedMsg{seq: 3, source: download.SearchYouTube, query: "q", results: results})
	if m.linkLoading {
		t.Fatal("search stayed loading")
	}
	if len(m.linkResults) != 1 || m.linkCursor != 0 {
		t.Fatalf("results = %d cursor = %d", len(m.linkResults), m.linkCursor)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.linkCursor != 0 {
		t.Fatalf("cursor = %d after down with one result", m.linkCursor)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.modal != modalAddLink || len(m.linkResults) != 0 {
		t.Fatalf("first esc: modal=%v results=%d", m.modal, len(m.linkResults))
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.modal != modalNone {
		t.Fatalf("second esc: modal=%v", m.modal)
	}
}

func TestToggleSearchSource(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkInput.SetValue("daft punk")
	m.linkResults = []download.SearchResult{{Title: "A"}}
	m.linkSource = download.SearchYouTube
	cmd := m.toggleLinkSource()
	if m.linkSource != download.SearchSoundCloud {
		t.Fatalf("source = %v, want SoundCloud", m.linkSource)
	}
	if !m.linkLoading {
		t.Fatal("source switch with results did not start a new search")
	}
	if cmd == nil {
		t.Fatal("source switch returned no command")
	}
}

func TestSearchSelection(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkResults = []download.SearchResult{
		{URL: "https://a", Title: "A"},
		{URL: "https://b", Title: "B"},
		{URL: "https://c", Title: "C"},
	}
	m.linkCursor = 0
	m.toggleSearchSelection()
	m.linkCursor = 2
	m.toggleSearchSelection()
	if got := m.selectedSearchCount(); got != 2 {
		t.Fatalf("selected = %d, want 2", got)
	}
	selected := m.selectedSearchResults()
	if len(selected) != 2 || selected[0].URL != "https://a" || selected[1].URL != "https://c" {
		t.Fatalf("selected results = %+v", selected)
	}
	frame := m.renderAddLinkModal(110, 32)
	if !strings.Contains(frame, "[x]") || !strings.Contains(frame, "2 selected") {
		t.Fatalf("selection markers missing:\n%s", frame)
	}
	m.selectAllSearch()
	if got := m.selectedSearchCount(); got != 3 {
		t.Fatalf("select all = %d, want 3", got)
	}
	m.clearLinkResults()
	if got := m.selectedSearchCount(); got != 0 {
		t.Fatalf("selection survived clear: %d", got)
	}
}

func TestAuditionStates(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkResults = []download.SearchResult{{URL: "https://a", Title: "A"}}
	m.linkCursor = 0
	cmd := m.toggleAudition()
	if !m.auditionLoading || cmd == nil {
		t.Fatalf("audition did not start: loading=%v cmd=%v", m.auditionLoading, cmd)
	}
	m.auditionSeq = 7
	m.auditionLoading = true
	m.handleAuditionReady(auditionReadyMsg{seq: 7, err: errors.New("no preview")})
	if m.auditionLoading || !strings.Contains(m.auditionErr, "no preview") {
		t.Fatalf("audition error state = %v / %q", m.auditionLoading, m.auditionErr)
	}
	m.auditionActive = true
	m.auditionLength = 20 * time.Second
	m.auditionPos = 7 * time.Second
	frame := m.renderAddLinkModal(110, 32)
	if !strings.Contains(frame, "Preview 0:07 / 0:20") {
		t.Fatalf("preview status missing:\n%s", frame)
	}
	m.stopAudition()
	if m.auditionActive || m.auditionLoading || m.auditionPos != 0 {
		t.Fatalf("audition did not stop: %+v", m)
	}
}

func TestSearchSelectedEnqueueShowsToolError(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkResults = []download.SearchResult{
		{URL: "https://a", Title: "A", Kind: download.KindYouTube},
		{URL: "https://b", Title: "B", Kind: download.KindYouTube},
	}
	m.linkSelected["https://a"] = true
	m.linkSelected["https://b"] = true
	_ = m.enqueueSearchResult()
	if m.linkErr == "" {
		t.Fatal("missing tools did not set linkErr")
	}
	if m.modal != modalAddLink {
		t.Fatalf("modal = %v, want add link", m.modal)
	}
}

func TestSearchEmptyResultsMessage(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkSeq = 1
	m.handleSearchLoaded(searchLoadedMsg{seq: 1, source: download.SearchSoundCloud, query: "x"})
	if !strings.Contains(m.linkErr, "Nothing found on SoundCloud") {
		t.Fatalf("linkErr = %q", m.linkErr)
	}
}

func TestSearchResultEnqueueShowsToolError(t *testing.T) {
	m, cleanup := testModel(t)
	defer cleanup()
	m.openAddLink()
	m.linkResults = []download.SearchResult{{
		URL:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Kind:   download.KindYouTube,
		Title:  "Test",
		Source: download.SearchYouTube,
	}}
	_ = m.enqueueSearchResult()
	if m.linkErr == "" {
		t.Fatal("missing tools did not set linkErr")
	}
	if m.modal != modalAddLink {
		t.Fatalf("modal = %v, want add link", m.modal)
	}
}
