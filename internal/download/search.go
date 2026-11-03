package download

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SearchResultLimit is the default number of results fetched per query.
const SearchResultLimit = 15

// SearchSource identifies a searchable platform.
type SearchSource int

const (
	SearchYouTube SearchSource = iota
	SearchSoundCloud
)

func (s SearchSource) String() string {
	if s == SearchSoundCloud {
		return "SoundCloud"
	}
	return "YouTube"
}

// Next cycles between the supported search sources.
func (s SearchSource) Next() SearchSource {
	if s == SearchYouTube {
		return SearchSoundCloud
	}
	return SearchYouTube
}

// SearchResult is one track found by an in-app search.
type SearchResult struct {
	URL      string
	Kind     Kind
	Title    string
	Artist   string
	Duration time.Duration
	CoverURL string
	Source   SearchSource
}

// Search queries YouTube or SoundCloud through yt-dlp. No API keys are
// required; cookies are reused when data/cookies.txt exists.
func (m *Manager) Search(ctx context.Context, query string, source SearchSource, limit int) ([]SearchResult, error) {
	query = cleanText(query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	if limit <= 0 {
		limit = SearchResultLimit
	}
	if limit > 25 {
		limit = 25
	}
	prefix := "ytsearch"
	kind := KindYouTube
	if source == SearchSoundCloud {
		prefix = "scsearch"
		kind = KindSoundCloud
	}
	target := prefix + strconv.Itoa(limit) + ":" + query
	info, err := m.fetchInfo(ctx, target, true)
	if err != nil {
		return nil, withNetworkHint(Link{Kind: kind, URL: target}, err)
	}
	if info == nil {
		return nil, errors.New("yt-dlp returned no search metadata")
	}

	results := make([]SearchResult, 0, limit)
	seen := make(map[string]bool, len(info.Entries))
	for _, entry := range info.Entries {
		if isLiveEntry(entry) {
			continue
		}
		url := searchEntryURL(source, entry)
		if url == "" || seen[url] {
			continue
		}
		title := cleanText(firstNonEmpty(entry.Track, entry.Title))
		if title == "" {
			continue
		}
		artist := entryArtist(&entry)
		switch {
		case artist != "":
			if stripped := stripArtistPrefix(title, artist); stripped != title {
				title = stripped
			} else if stripped := stripArtistSuffix(title, artist); stripped != title {
				title = stripped
			} else if left, right, ok := splitArtistTitle(title); ok {
				artist, title = cleanText(left), cleanText(right)
			}
		case strings.Contains(title, " - "):
			if left, right, ok := splitArtistTitle(title); ok {
				artist, title = cleanText(left), cleanText(right)
			}
		}
		if cleaned := cleanVideoSuffix(title); cleaned != "" {
			title = cleaned
		}
		title = cleanSearchTitle(title)
		if title == "" {
			continue
		}
		seen[url] = true
		results = append(results, SearchResult{
			URL:      url,
			Kind:     kind,
			Title:    title,
			Artist:   artist,
			Duration: secondsToDuration(entry.Duration),
			CoverURL: infoThumbnail(&entry),
			Source:   source,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

var inlineQualifierPattern = regexp.MustCompile(`(?i)\s*[\[(]\s*(official\s*(music\s*)?(video|audio)(\s+remaster(ed)?)?|official\s*lyric\s*video|lyric\s*video|video\s*(oficial|lyric)|audio\s*oficial|performance\s*video|audio|visualizer)\s*[\])]`)

// cleanSearchTitle removes common video qualifiers that appear in the middle
// of a YouTube title, e.g. "Song (Official Video) ft. Artist".
func cleanSearchTitle(title string) string {
	title = inlineQualifierPattern.ReplaceAllString(title, " ")
	return cleanText(title)
}

func searchEntryURL(source SearchSource, entry ytInfo) string {
	if u := strings.TrimSpace(entry.WebpageURL); strings.HasPrefix(u, "http") {
		return u
	}
	raw := strings.TrimSpace(entry.URL)
	if strings.HasPrefix(raw, "http") {
		return raw
	}
	if source == SearchYouTube && youtubeIDPattern.MatchString(entry.ID) {
		return "https://www.youtube.com/watch?v=" + entry.ID
	}
	return ""
}

// EnqueueResult adds a selected search result directly to the download queue.
// The result metadata is used as a seed, so the pipeline does not have to
// resolve the track a second time.
func (m *Manager) EnqueueResult(result SearchResult) (Job, error) {
	link, err := linkFromSearchResult(result)
	if err != nil {
		return Job{}, err
	}
	return m.enqueueLink(link)
}

func linkFromSearchResult(result SearchResult) (Link, error) {
	if strings.TrimSpace(result.URL) == "" {
		return Link{}, errors.New("search result has no URL")
	}
	if result.Kind != KindYouTube && result.Kind != KindSoundCloud {
		return Link{}, errors.New("search result has an unsupported source")
	}
	meta := Meta{
		Title:       cleanText(result.Title),
		Artist:      cleanText(result.Artist),
		Duration:    result.Duration,
		CoverURL:    result.CoverURL,
		SourceURL:   result.URL,
		DownloadURL: result.URL,
	}
	return Link{
		Raw:  result.URL,
		URL:  result.URL,
		Kind: result.Kind,
		ID:   trackIDFromURL(result.URL),
		Seed: &meta,
	}, nil
}

// EnqueueResults adds several selected search results as one group. Duplicate
// URLs are skipped, preserving the order of the result list.
func (m *Manager) EnqueueResults(results []SearchResult, groupTitle string) (BatchResult, error) {
	if len(results) == 0 {
		return BatchResult{}, errors.New("no tracks selected")
	}
	if err := m.checkTools(true); err != nil {
		return BatchResult{}, err
	}

	type item struct {
		link Link
	}
	items := make([]item, 0, len(results))
	seen := make(map[string]bool, len(results))
	result := BatchResult{}
	for _, searchResult := range results {
		url := strings.TrimSpace(searchResult.URL)
		if url == "" {
			result.Skipped++
			continue
		}
		if seen[url] {
			result.Duplicates++
			continue
		}
		link, err := linkFromSearchResult(searchResult)
		if err != nil {
			result.Skipped++
			continue
		}
		seen[url] = true
		items = append(items, item{link: link})
	}
	if len(items) == 0 {
		return BatchResult{}, errors.New("all selected results were skipped")
	}
	if strings.TrimSpace(groupTitle) == "" {
		groupTitle = "Search results"
	}

	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return BatchResult{}, errors.New("downloader is shutting down")
	}
	m.nextGroupID++
	groupID := m.nextGroupID
	ids := make([]int, 0, len(items))
	for i, it := range items {
		m.nextID++
		title := it.link.URL
		artist := ""
		if it.link.Seed != nil {
			if s := cleanText(it.link.Seed.Title); s != "" {
				title = s
			}
			artist = cleanText(it.link.Seed.Artist)
		}
		job := &Job{
			ID:          m.nextID,
			URL:         it.link.URL,
			Kind:        it.link.Kind,
			Title:       title,
			Artist:      artist,
			Stage:       "queued",
			Detail:      "waiting for a free slot",
			Progress:    -1,
			Status:      StatusQueued,
			Created:     time.Now(),
			GroupID:     groupID,
			GroupTitle:  groupTitle,
			GroupKind:   KindUnknown,
			TrackNumber: i + 1,
			TrackTotal:  len(items),
			link:        it.link,
		}
		m.jobs[job.ID] = job
		m.order = append(m.order, job.ID)
		ids = append(ids, job.ID)
	}
	m.mu.Unlock()

	for _, id := range ids {
		select {
		case m.queue <- id:
		case <-m.ctx.Done():
			return BatchResult{}, errors.New("downloader is shutting down")
		}
	}
	result.GroupID = groupID
	result.Queued = len(ids)
	return result, nil
}
