package presence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Activity is the player state published to Discord.
type Activity struct {
	Clear     bool
	TrackKey  string
	Title     string
	Artist    string
	Album     string
	CoverURL  string
	SourceURL string
	Position  time.Duration
	Duration  time.Duration
	Paused    bool

	coverOnly    bool
	noAssets     bool
	forcePlaying bool
}

// Client keeps a Rich Presence connection alive in the background.
type Client struct {
	cfg     Config
	http    *http.Client
	updates chan Activity
	status  chan Status
	sendErr chan string
	done    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup

	logPath string
	logMu   sync.Mutex

	coverMu   sync.Mutex
	covers    map[string]string
	resolving map[string]bool
}

// New starts a client. It silently retries while Discord is closed.
func New(cfg Config) *Client {
	c := &Client{
		cfg:       cfg,
		http:      &http.Client{Timeout: 6 * time.Second},
		updates:   make(chan Activity, 1),
		status:    make(chan Status, 4),
		sendErr:   make(chan string, 1),
		done:      make(chan struct{}),
		covers:    make(map[string]string),
		resolving: make(map[string]bool),
	}
	if cfg.dataDir != "" {
		c.logPath = filepath.Join(cfg.dataDir, "discord.log")
		c.trimLog()
	}
	c.logf("presence started (client %s, type %d)", cfg.ClientID, cfg.activityType())
	c.wg.Add(1)
	go c.run()
	return c
}

// Status reports connection changes. The channel is best-effort and never
// blocks the presence worker.
func (c *Client) Status() <-chan Status {
	if c == nil {
		return nil
	}
	return c.status
}

func (c *Client) report(s Status) {
	select {
	case c.status <- s:
		return
	default:
	}
	select {
	case <-c.status:
	default:
	}
	select {
	case c.status <- s:
	default:
	}
}

func (c *Client) logf(format string, args ...any) {
	if c == nil || c.logPath == "" {
		return
	}
	c.logMu.Lock()
	defer c.logMu.Unlock()
	f, err := os.OpenFile(c.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func (c *Client) trimLog() {
	info, err := os.Stat(c.logPath)
	if err == nil && info.Size() > 512*1024 {
		_ = os.Remove(c.logPath)
	}
}

// Update publishes a new activity. Rapid calls are coalesced.
func (c *Client) Update(a Activity) {
	if c == nil || a.Clear {
		return
	}
	if a.Title == "" {
		return
	}
	if a.CoverURL == "" {
		if url := c.cachedCover(a); url != "" {
			a.CoverURL = url
		}
	}
	c.push(a)
	if a.CoverURL == "" {
		c.lookupCover(a)
	}
}

// Clear removes the activity from the profile.
func (c *Client) Clear() {
	if c == nil {
		return
	}
	c.push(Activity{Clear: true})
}

// Close clears the activity and stops the background worker.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.done)
		c.wg.Wait()
	})
}

func (c *Client) push(a Activity) {
	for {
		select {
		case c.updates <- a:
			return
		default:
			select {
			case <-c.updates:
			default:
			}
		}
	}
}

func (c *Client) run() {
	defer c.wg.Done()

	var (
		conn        *ipcConn
		disconnect  chan struct{}
		current     *Activity
		dirty       bool
		lastSent    time.Time
		retryAt     time.Time
		lastConnErr string
		sendTimer   *time.Timer
		sendC       <-chan time.Time
		retryTimer  *time.Timer
		retryC      <-chan time.Time
	)
	stopTimer := func(t **time.Timer) {
		if *t != nil {
			(*t).Stop()
			*t = nil
		}
	}
	defer func() {
		stopTimer(&sendTimer)
		stopTimer(&retryTimer)
	}()

	for {
		now := time.Now()
		if conn == nil && !now.Before(retryAt) {
			nc, err := connectIPC(c.cfg.ClientID)
			if err != nil {
				text := DescribeError(err)
				if text != lastConnErr {
					c.logf("connect failed: %v", err)
					c.report(Status{Err: err})
					lastConnErr = text
				}
				retryAt = time.Now().Add(10 * time.Second)
			} else {
				conn = nc
				disconnect = make(chan struct{})
				go nc.readLoop(disconnect, c.handleRPCMessage)
				lastSent = time.Time{}
				lastConnErr = ""
				c.logf("connected to Discord (client %s)", c.cfg.ClientID)
				c.report(Status{Connected: true})
				if current != nil {
					if err := c.send(conn, *current); err != nil {
						c.logf("initial SET_ACTIVITY failed: %v", err)
						conn.close()
						conn = nil
						disconnect = nil
						retryAt = time.Now().Add(5 * time.Second)
					} else {
						c.logf("activity sent")
						lastSent = time.Now()
						dirty = false
					}
				}
			}
		}

		if conn != nil && dirty && current != nil {
			if wait := 4*time.Second - time.Since(lastSent); wait <= 0 {
				if err := c.send(conn, *current); err != nil {
					c.logf("SET_ACTIVITY failed: %v", err)
					conn.close()
					conn = nil
					disconnect = nil
					retryAt = time.Now().Add(5 * time.Second)
					continue
				}
				c.logf("activity sent")
				lastSent = time.Now()
				dirty = false
			}
		}

		if conn != nil && dirty && current != nil {
			if wait := 4*time.Second - time.Since(lastSent); wait > 0 {
				if sendTimer == nil {
					sendTimer = time.NewTimer(wait)
					sendC = sendTimer.C
				}
			} else {
				continue
			}
		} else {
			stopTimer(&sendTimer)
			sendC = nil
		}

		if conn == nil {
			if wait := time.Until(retryAt); wait > 0 {
				if retryTimer == nil {
					retryTimer = time.NewTimer(wait)
					retryC = retryTimer.C
				}
			} else {
				continue
			}
		} else {
			stopTimer(&retryTimer)
			retryC = nil
		}

		select {
		case a := <-c.updates:
			if a.coverOnly {
				if current == nil || current.TrackKey != a.TrackKey {
					continue
				}
				merged := *current
				merged.CoverURL = a.CoverURL
				current = &merged
				dirty = true
				continue
			}
			// Track changes and pause/resume should appear immediately. Other
			// updates stay behind the small rate-limit window.
			if current == nil || current.TrackKey != a.TrackKey || current.Paused != a.Paused {
				lastSent = time.Time{}
				if a.Clear {
					c.logf("activity cleared")
				} else {
					c.logf("activity queued: %s | %s | paused=%v", a.Title, a.Artist, a.Paused)
				}
			}
			current = &a
			dirty = true

		case <-sendC:
			stopTimer(&sendTimer)
			sendC = nil
			if conn != nil && dirty && current != nil {
				if err := c.send(conn, *current); err != nil {
					c.logf("SET_ACTIVITY failed: %v", err)
					conn.close()
					conn = nil
					disconnect = nil
					retryAt = time.Now().Add(5 * time.Second)
				} else {
					c.logf("activity sent")
					lastSent = time.Now()
					dirty = false
				}
			}

		case errMsg := <-c.sendErr:
			if current == nil {
				continue
			}
			c.logf("SET_ACTIVITY rejected: %s", errMsg)
			if !current.noAssets && (strings.TrimSpace(current.CoverURL) != "" || strings.TrimSpace(c.cfg.FallbackImage) != "") {
				merged := *current
				merged.noAssets = true
				current = &merged
				dirty = true
				continue
			}
			if !current.forcePlaying && c.cfg.activityType() != 0 {
				merged := *current
				merged.forcePlaying = true
				current = &merged
				dirty = true
				continue
			}
			c.report(Status{Err: fmt.Errorf("Discord rejected the activity: %s", errMsg)})

		case <-retryC:
			stopTimer(&retryTimer)
			retryC = nil
			retryAt = time.Time{}

		case <-disconnect:
			if conn != nil {
				conn.close()
			}
			conn = nil
			disconnect = nil
			retryAt = time.Now().Add(5 * time.Second)
			stopTimer(&sendTimer)
			sendC = nil
			c.logf("disconnected from Discord")
			select {
			case <-c.done:
			default:
				c.report(Status{Err: errors.New("connection to Discord lost")})
			}

		case <-c.done:
			c.logf("presence stopped")
			if conn != nil {
				_ = c.send(conn, Activity{Clear: true})
				conn.close()
			}
			return
		}
	}
}

func (c *Client) handleRPCMessage(payload []byte) {
	var msg struct {
		Cmd  string          `json:"cmd"`
		Evt  string          `json:"evt"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	if msg.Evt != "ERROR" {
		return
	}
	text := rpcErrorMessage(msg.Data)
	c.logf("%s failed: %s", msg.Cmd, text)
	if msg.Cmd == "SET_ACTIVITY" {
		select {
		case c.sendErr <- text:
		default:
		}
	}
}

func rpcErrorMessage(data json.RawMessage) string {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &e); err == nil && (e.Message != "" || e.Code != 0) {
		if e.Message == "" {
			return fmt.Sprintf("error %d", e.Code)
		}
		return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return "unknown error"
	}
	return text
}

type rpcArgs struct {
	PID      int              `json:"pid"`
	Activity *activityPayload `json:"activity,omitempty"`
}

type activityPayload struct {
	Type       int            `json:"type"`
	Details    string         `json:"details,omitempty"`
	State      string         `json:"state,omitempty"`
	Timestamps *timestamps    `json:"timestamps,omitempty"`
	Assets     *activityAsset `json:"assets,omitempty"`
}

type timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

type activityAsset struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

func (c *Client) send(conn *ipcConn, a Activity) error {
	args := rpcArgs{PID: os.Getpid()}
	if !a.Clear {
		activity := c.buildActivity(a)
		args.Activity = &activity
	}
	return conn.write(opFrame, rpcFrame{
		Cmd:   "SET_ACTIVITY",
		Args:  args,
		Nonce: activityNonce(),
	})
}

func (c *Client) buildActivity(a Activity) activityPayload {
	activityType := c.cfg.activityType()
	if a.forcePlaying {
		activityType = 0
	}
	p := activityPayload{
		Type:    activityType,
		Details: activityText(a.Title, 128),
		State:   activityText(activityState(a), 128),
	}
	if !a.Paused {
		start := time.Now().Add(-a.Position).Unix()
		ts := &timestamps{Start: start}
		if a.Duration > 0 {
			ts.End = start + int64(a.Duration/time.Second)
		}
		p.Timestamps = ts
	}

	if a.noAssets {
		return p
	}
	image := strings.TrimSpace(a.CoverURL)
	if image == "" {
		image = c.cfg.FallbackImage
	}
	if len(image) > 256 {
		image = ""
	}
	if image != "" {
		text := a.Album
		if text == "" {
			text = a.Title
		}
		p.Assets = &activityAsset{LargeImage: image, LargeText: activityText(text, 128)}
	}
	return p
}

// activityText keeps Discord's 2..128 character constraint in mind. Very
// short values are omitted instead of making Discord reject the activity.
func activityText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) < 2 {
		return ""
	}
	return truncateRunes(s, max)
}

func activityState(a Activity) string {
	parts := make([]string, 0, 3)
	if a.Artist != "" {
		parts = append(parts, a.Artist)
	}
	if a.Album != "" {
		parts = append(parts, a.Album)
	}
	if a.Paused {
		pos := formatDuration(a.Position)
		if a.Duration > 0 {
			pos += " / " + formatDuration(a.Duration)
		}
		parts = append(parts, "Paused "+pos)
	}
	if len(parts) == 0 {
		if a.Paused {
			return "Paused"
		}
		return ""
	}
	return truncateRunes(strings.Join(parts, " · "), 128)
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func coverKey(a Activity) string {
	return strings.ToLower(strings.TrimSpace(a.Title)) + "|" +
		strings.ToLower(strings.TrimSpace(a.Artist))
}

func (c *Client) cachedCover(a Activity) string {
	key := coverKey(a)
	if key == "|" {
		return ""
	}
	c.coverMu.Lock()
	defer c.coverMu.Unlock()
	return c.covers[key]
}

func (c *Client) lookupCover(a Activity) {
	key := coverKey(a)
	if key == "|" {
		return
	}
	c.coverMu.Lock()
	if _, done := c.covers[key]; done {
		c.coverMu.Unlock()
		return
	}
	if c.resolving[key] {
		c.coverMu.Unlock()
		return
	}
	c.resolving[key] = true
	c.coverMu.Unlock()

	go func(original Activity, key string) {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		url := lookupSourceArtwork(ctx, c.http, original.SourceURL)
		if url == "" {
			url = lookupArtwork(ctx, c.http, original.Title, original.Artist)
		}

		c.coverMu.Lock()
		c.covers[key] = url
		delete(c.resolving, key)
		c.coverMu.Unlock()

		if url == "" {
			return
		}
		updated := original
		updated.CoverURL = url
		updated.coverOnly = true
		c.push(updated)
	}(a, key)
}
