// Package download turns shared links into tagged MP3 files in the library.
package download

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"tuiplay/internal/library"
)

// Status is the lifecycle state of a download job.
type Status int

const (
	StatusQueued Status = iota
	StatusResolving
	StatusDownloading
	StatusConverting
	StatusTagging
	StatusDone
	StatusFailed
	StatusCanceled
)

func (s Status) String() string {
	switch s {
	case StatusQueued:
		return "queued"
	case StatusResolving:
		return "resolving"
	case StatusDownloading:
		return "downloading"
	case StatusConverting:
		return "converting"
	case StatusTagging:
		return "tagging"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// Terminal reports whether the job will not change state anymore.
func (s Status) Terminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusCanceled
}

// Job is a snapshot of one download.
type Job struct {
	ID       int
	URL      string
	Kind     Kind
	Title    string
	Artist   string
	Stage    string
	Detail   string
	Progress float64
	Status   Status
	Err      string
	Warning  string
	Path     string
	Created  time.Time
	Finished time.Time

	link   Link
	cancel context.CancelFunc
}

// Options configures a Manager.
type Options struct {
	MusicDir    string
	DataDir     string
	TempDir     string
	Tools       Tools
	CookiesFile string
	HTTPClient  *http.Client
}

// Manager serializes downloads and exposes their progress.
type Manager struct {
	opts   Options
	client *http.Client
	tools  Tools

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu           sync.Mutex
	jobs         map[int]*Job
	order        []int
	nextID       int
	queue        chan int
	activeCancel context.CancelFunc
}

// NewManager creates a manager and starts its single worker.
func NewManager(opts Options) *Manager {
	if opts.TempDir == "" {
		opts.TempDir = filepath.Join(opts.DataDir, "tmp")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = defaultHTTPClient()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		opts:   opts,
		client: opts.HTTPClient,
		tools:  opts.Tools,
		ctx:    ctx,
		cancel: cancel,
		jobs:   make(map[int]*Job),
		queue:  make(chan int, 64),
	}
	_ = os.MkdirAll(opts.MusicDir, 0o755)
	_ = os.MkdirAll(opts.TempDir, 0o755)
	m.cleanOldTemp()
	m.wg.Add(1)
	go m.worker()
	return m
}

func defaultHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
	}
	return &http.Client{Transport: transport}
}

// Close stops the worker and any active download.
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	if m.activeCancel != nil {
		m.activeCancel()
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2500 * time.Millisecond):
	}
}

// Enqueue validates a link and adds it to the download queue.
func (m *Manager) Enqueue(raw string) (Job, error) {
	link, err := Classify(raw)
	if err != nil {
		return Job{}, err
	}
	needsYtDlp := link.Kind != KindDirect
	if missing := m.tools.Missing(needsYtDlp); len(missing) > 0 {
		return Job{}, fmt.Errorf("%s is required — %s", missing[0], InstallHint(missing[0]))
	}

	select {
	case <-m.ctx.Done():
		return Job{}, errors.New("downloader is shutting down")
	default:
	}

	m.mu.Lock()
	m.nextID++
	job := &Job{
		ID:       m.nextID,
		URL:      link.URL,
		Kind:     link.Kind,
		Title:    link.URL,
		Stage:    "queued",
		Detail:   "waiting for a free slot",
		Progress: -1,
		Status:   StatusQueued,
		Created:  time.Now(),
		link:     link,
	}
	m.jobs[job.ID] = job
	m.order = append(m.order, job.ID)
	snapshot := *job
	m.mu.Unlock()

	select {
	case m.queue <- job.ID:
		return snapshot, nil
	case <-m.ctx.Done():
		return Job{}, errors.New("downloader is shutting down")
	}
}

// Jobs returns a copy of all jobs in submission order.
func (m *Manager) Jobs() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.order))
	for _, id := range m.order {
		if j := m.jobs[id]; j != nil {
			out = append(out, *j)
		}
	}
	return out
}

// Get returns one job snapshot.
func (m *Manager) Get(id int) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Cancel stops a queued or active job.
func (m *Manager) Cancel(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.Status.Terminal() {
		return
	}
	if j.cancel != nil {
		j.cancel()
	}
	j.Status = StatusCanceled
	j.Stage = "canceled"
	j.Detail = "download canceled"
	j.Finished = time.Now()
}

// ClearFinished removes completed jobs from the list.
func (m *Manager) ClearFinished() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.order[:0]
	removed := 0
	for _, id := range m.order {
		j := m.jobs[id]
		if j != nil && j.Status.Terminal() {
			delete(m.jobs, id)
			removed++
			continue
		}
		kept = append(kept, id)
	}
	m.order = kept
	return removed
}

// ActiveCount returns the number of queued or running jobs.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, j := range m.jobs {
		if !j.Status.Terminal() {
			n++
		}
	}
	return n
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case id := <-m.queue:
			m.run(id)
		}
	}
}

func (m *Manager) run(id int) {
	m.mu.Lock()
	j := m.jobs[id]
	if j == nil || j.Status.Terminal() {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	j.cancel = cancel
	j.Status = StatusResolving
	j.Stage = "resolving"
	j.Detail = "fetching metadata"
	j.Progress = -1
	link := j.link
	m.activeCancel = cancel
	m.mu.Unlock()

	path, warning, err := m.process(ctx, id, link)
	ctxErr := ctx.Err()

	cancel()
	m.mu.Lock()
	if j.cancel != nil {
		j.cancel = nil
	}
	m.activeCancel = nil
	if !j.Status.Terminal() {
		switch {
		case ctxErr != nil || errors.Is(err, context.Canceled):
			j.Status = StatusCanceled
			j.Stage = "canceled"
			j.Detail = "download canceled"
			j.Finished = time.Now()
		case err != nil:
			j.Status = StatusFailed
			j.Stage = "failed"
			j.Err = err.Error()
			j.Detail = err.Error()
			j.Finished = time.Now()
		default:
			j.Status = StatusDone
			j.Stage = "done"
			j.Path = path
			j.Progress = 1
			j.Detail = "saved to library"
			j.Warning = warning
			j.Finished = time.Now()
		}
	}
	m.mu.Unlock()
}

func (m *Manager) process(ctx context.Context, id int, link Link) (string, string, error) {
	meta, err := m.resolve(ctx, link, id)
	if err != nil {
		return "", "", err
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	m.setMeta(id, meta)
	m.setStatus(id, StatusDownloading, "downloading")
	m.updateProgress(id, -1, "starting download")

	tmpDir := filepath.Join(m.opts.TempDir, fmt.Sprintf("job-%d-%d", id, time.Now().UnixNano()))
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	var source string
	if link.Kind == KindDirect {
		source, err = m.fetchDirect(ctx, meta.DownloadURL, tmpDir, func(p float64, detail string) {
			m.updateProgress(id, p, detail)
		})
	} else {
		source, err = m.fetchYtDlp(ctx, meta.DownloadURL, tmpDir, func(p float64, detail string) {
			m.updateProgress(id, p, detail)
		})
	}
	if err != nil {
		return "", "", err
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}

	m.setStatus(id, StatusConverting, "converting")
	m.updateProgress(id, -1, "transcoding to MP3")
	mp3Path := filepath.Join(tmpDir, "audio.mp3")
	if err := m.convert(ctx, source, mp3Path, meta.Duration, func(p float64, detail string) {
		m.updateProgress(id, p, detail)
	}); err != nil {
		return "", "", err
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}

	warning := ""
	var coverJPEG []byte
	if meta.CoverURL != "" {
		m.setStatus(id, StatusTagging, "tagging")
		m.updateProgress(id, -1, "fetching cover art")
		raw, coverErr := m.fetchCover(ctx, meta.CoverURL)
		if coverErr == nil {
			coverJPEG, coverErr = library.NormalizeCover(raw)
		}
		if coverErr != nil {
			warning = appendWarning(warning, "cover art unavailable")
		}
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}

	m.setStatus(id, StatusTagging, "tagging")
	m.updateProgress(id, -1, "writing tags")
	if err := writeTags(mp3Path, meta, coverJPEG); err != nil {
		warning = appendWarning(warning, "tags could not be written")
	}

	finalPath := uniqueFilePath(m.opts.MusicDir, trackFilename(meta))
	if err := moveFile(mp3Path, finalPath); err != nil {
		return "", warning, fmt.Errorf("save track: %w", err)
	}
	if len(coverJPEG) > 0 {
		if _, err := library.WriteCoverJPEG(m.opts.DataDir, finalPath, coverJPEG); err != nil {
			warning = appendWarning(warning, "cover cache unavailable")
		}
	}
	return finalPath, warning, nil
}

func (m *Manager) setMeta(id int, meta Meta) {
	m.updateJob(id, func(j *Job) {
		j.Title = meta.Title
		j.Artist = meta.Artist
	})
}

func (m *Manager) setStatus(id int, status Status, stage string) {
	m.updateJob(id, func(j *Job) {
		j.Status = status
		j.Stage = stage
	})
}

func (m *Manager) setDetail(id int, detail string) {
	m.updateJob(id, func(j *Job) {
		j.Detail = detail
	})
}

func (m *Manager) updateProgress(id int, progress float64, detail string) {
	m.updateJob(id, func(j *Job) {
		if progress >= 0 {
			j.Progress = progress
		}
		if detail != "" {
			j.Detail = detail
		}
	})
}

func (m *Manager) updateJob(id int, fn func(*Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil || j.Status.Terminal() {
		return
	}
	fn(j)
}

func (m *Manager) cleanOldTemp() {
	entries, err := os.ReadDir(m.opts.TempDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(m.opts.TempDir, e.Name()))
	}
}

// SortJobsByID is a helper for UIs that want newest first.
func SortJobsByID(jobs []Job, descending bool) {
	sort.SliceStable(jobs, func(i, j int) bool {
		if descending {
			return jobs[i].ID > jobs[j].ID
		}
		return jobs[i].ID < jobs[j].ID
	})
}
