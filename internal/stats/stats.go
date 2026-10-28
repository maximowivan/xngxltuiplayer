// Package stats stores per-track play counts, favourites and recent activity.
package stats

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"tuiplay/internal/atomicfile"
)

// TrackStats is the small amount of per-track data kept between runs.
type TrackStats struct {
	Plays      int       `json:"plays,omitempty"`
	LastPlayed time.Time `json:"last_played,omitempty"`
	Favorite   bool      `json:"favorite,omitempty"`
}

type fileFormat struct {
	Version int                   `json:"version"`
	Tracks  map[string]TrackStats `json:"tracks"`
}

// Store is a concurrency-safe statistics store backed by data/stats.json.
type Store struct {
	mu     sync.Mutex
	path   string
	tracks map[string]TrackStats
}

// Load reads data/stats.json. A missing or broken file yields an empty store;
// a valid .bak is restored automatically by atomicfile.
func Load(dataDir string) *Store {
	s := &Store{
		path:   filepath.Join(dataDir, "stats.json"),
		tracks: make(map[string]TrackStats),
	}
	var f fileFormat
	if err := atomicfile.ReadJSON(s.path, &f); err != nil {
		return s
	}
	if f.Version != 1 {
		return s
	}
	for path, st := range f.Tracks {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		s.tracks[pathKey(path)] = st
	}
	return s
}

// IsFavorite reports whether path is marked as a favourite.
func (s *Store) IsFavorite(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracks[pathKey(path)].Favorite
}

// FavoriteCount returns the number of favourite tracks.
func (s *Store) FavoriteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, st := range s.tracks {
		if st.Favorite {
			n++
		}
	}
	return n
}

// SetFavorite marks or unmarks a track as a favourite.
func (s *Store) SetFavorite(path string, favorite bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pathKey(path)
	st := s.tracks[key]
	st.Favorite = favorite
	if st.Plays == 0 && st.LastPlayed.IsZero() && !favorite {
		delete(s.tracks, key)
	} else {
		s.tracks[key] = st
	}
	return s.saveLocked()
}

// ToggleFavorite flips a track's favourite flag and returns the new value.
func (s *Store) ToggleFavorite(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pathKey(path)
	st := s.tracks[key]
	st.Favorite = !st.Favorite
	s.tracks[key] = st
	return st.Favorite, s.saveLocked()
}

// MarkPlayed increments the play counter and updates the last-played time.
func (s *Store) MarkPlayed(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pathKey(path)
	st := s.tracks[key]
	st.Plays++
	st.LastPlayed = time.Now()
	s.tracks[key] = st
	return s.saveLocked()
}

// Plays returns the stored play count for a track.
func (s *Store) Plays(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracks[pathKey(path)].Plays
}

// Recent returns up to limit paths ordered by most recent playback. A limit
// of zero or less returns every played track.
func (s *Store) Recent(limit int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	type item struct {
		path string
		at   time.Time
	}
	items := make([]item, 0, len(s.tracks))
	for path, st := range s.tracks {
		if st.LastPlayed.IsZero() {
			continue
		}
		items = append(items, item{path: path, at: st.LastPlayed})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].at.Equal(items[j].at) {
			return items[i].at.After(items[j].at)
		}
		return items[i].path < items[j].path
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out
}

// Merge folds statistics for a duplicate track into the keeper. Play counts
// are combined by taking the larger value, favourites are OR-ed and the latest
// play time wins.
func (s *Store) Merge(oldPath, newPath string) error {
	oldKey := pathKey(oldPath)
	newKey := pathKey(newPath)
	if oldKey == newKey {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.tracks[oldKey]
	if !ok {
		return nil
	}
	dst := s.tracks[newKey]
	if src.Plays > dst.Plays {
		dst.Plays = src.Plays
	}
	dst.Favorite = dst.Favorite || src.Favorite
	if src.LastPlayed.After(dst.LastPlayed) {
		dst.LastPlayed = src.LastPlayed
	}
	s.tracks[newKey] = dst
	delete(s.tracks, oldKey)
	return s.saveLocked()
}

// Remove drops all statistics for a deleted track.
func (s *Store) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tracks, pathKey(path))
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteJSON(s.path, fileFormat{Version: 1, Tracks: s.tracks})
}

func pathKey(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
