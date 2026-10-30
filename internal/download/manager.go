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
	"strings"
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
	ID          int
	URL         string
	Kind        Kind
	Title       string
	Artist      string
	Stage       string
	Detail      string
	Progress    float64
	Status      Status
	Err         string
	Warning     string
	Path        string
	Created     time.Time
	Finished    time.Time
	GroupID     int
	GroupTitle  string
	GroupKind   Kind
	TrackNumber int
	TrackTotal  int

	link    Link
	cancel  context.CancelFunc
	running bool
}

// Options configures a Manager.
type Options struct {
	MusicDir    string
	DataDir     string
	TempDir     string
	Tools       Tools
	CookiesFile string
	HTTPClient  *http.Client
	Workers     int
}

// Manager runs downloads and exposes their progress.
type Manager struct {
	opts   Options
	client *http.Client
	tools  Tools

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu          sync.Mutex
	jobs        map[int]*Job
	order       []int
	nextID      int
	nextGroupID int
	queue       chan int
	active      map[int]context.CancelFunc

	// runJSON executes yt-dlp and returns its JSON output. Tests replace it to
	// exercise playlist parsing without the real binary.
	runJSON func(ctx context.Context, args []string) ([]byte, error)
}

// NewManager creates a manager and starts its download workers.
func NewManager(opts Options) *Manager {
	if opts.TempDir == "" {
		opts.TempDir = filepath.Join(opts.DataDir, "tmp")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = defaultHTTPClient()
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 2
	}
	if workers > 6 {
		workers = 6
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		opts:   opts,
		client: opts.HTTPClient,
		tools:  opts.Tools,
		ctx:    ctx,
		cancel: cancel,
		jobs:   make(map[int]*Job),
		queue:  make(chan int, 1024),
		active: make(map[int]context.CancelFunc),
	}
	_ = os.MkdirAll(opts.MusicDir, 0o755)
	_ = os.MkdirAll(opts.TempDir, 0o755)
	m.cleanOldTemp()
	m.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go m.worker()
	}
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

// Close stops all workers and active downloads.
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	for _, cancel := range m.active {
		cancel()
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

func (m *Manager) checkTools(needsYtDlp bool) error {
	missing := m.tools.Missing(needsYtDlp)
	if len(missing) == 0 {
		return nil
	}
	hints := make([]string, len(missing))
	for i, name := range missing {
		hints[i] = InstallHint(name)
	}
	verb := "is required"
	if len(missing) > 1 {
		verb = "are required"
	}
	return fmt.Errorf("%s %s — %s",
		strings.Join(missing, ", "), verb, strings.Join(hints, "; "))
}

// Enqueue validates a link and adds it to the download queue.
func (m *Manager) Enqueue(raw string) (Job, error) {
	link, err := Classify(raw)
	if err != nil {
		return Job{}, err
	}
	if link.Kind.Batch() {
		return Job{}, errors.New("this link contains several tracks — use the playlist preview")
	}
	if err := m.checkTools(link.Kind != KindDirect); err != nil {
		return Job{}, err
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

// Retry re-queues one failed or canceled job. It returns false when the job is
// missing or still active.
func (m *Manager) Retry(id int) bool {
	m.mu.Lock()
	j := m.jobs[id]
	if j == nil || j.running || (j.Status != StatusFailed && j.Status != StatusCanceled) {
		m.mu.Unlock()
		return false
	}
	resetJobForRetry(j)
	m.mu.Unlock()

	select {
	case m.queue <- id:
		return true
	case <-m.ctx.Done():
		return false
	}
}

// RetryFailed re-queues every failed or canceled job and returns how many were
// accepted.
func (m *Manager) RetryFailed() int {
	m.mu.Lock()
	ids := make([]int, 0)
	for _, id := range m.order {
		j := m.jobs[id]
		if j == nil || j.running || (j.Status != StatusFailed && j.Status != StatusCanceled) {
			continue
		}
		resetJobForRetry(j)
		ids = append(ids, id)
	}
	m.mu.Unlock()

	queued := 0
	for _, id := range ids {
		select {
		case m.queue <- id:
			queued++
		case <-m.ctx.Done():
			return queued
		}
	}
	return queued
}

// CancelAll stops every queued or running job.
func (m *Manager) CancelAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, id := range m.order {
		j := m.jobs[id]
		if j == nil || j.Status.Terminal() {
			continue
		}
		if j.cancel != nil {
			j.cancel()
		}
		j.Status = StatusCanceled
		j.Stage = "canceled"
		j.Detail = "download canceled"
		j.Finished = time.Now()
		n++
	}
	return n
}

func resetJobForRetry(j *Job) {
	j.Status = StatusQueued
	j.Stage = "queued"
	j.Detail = "waiting for a free slot"
	j.Progress = -1
	j.Err = ""
	j.Warning = ""
	j.Path = ""
	j.Finished = time.Time{}
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
	if j == nil || j.Status.Terminal() || j.running {
		m.mu.Unlock()
		return
	}
	j.running = true
	ctx, cancel := context.WithCancel(m.ctx)
	j.cancel = cancel
	j.Status = StatusResolving
	j.Stage = "resolving"
	j.Detail = "fetching metadata"
	j.Progress = -1
	link := j.link
	m.active[id] = cancel
	m.mu.Unlock()

	path, warning, err := safeProcess(func() (string, string, error) {
		return m.process(ctx, id, link)
	})
	ctxErr := ctx.Err()

	cancel()
	m.mu.Lock()
	if j.cancel != nil {
		j.cancel = nil
	}
	delete(m.active, id)
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
	j.running = false
	m.mu.Unlock()
}

// safeProcess turns an unexpected panic in a download pipeline into a normal
// failed job. Without this, one malformed response could kill the single
// worker goroutine and leave every later job queued forever.
func safeProcess(fn func() (string, string, error)) (path, warning string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("internal download error: %v", recovered)
		}
	}()
	return fn()
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
