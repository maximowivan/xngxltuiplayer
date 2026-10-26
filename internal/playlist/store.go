// Package playlist stores named, ordered lists of track paths.
package playlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Playlist is a named ordered collection of absolute track paths.
type Playlist struct {
	Name    string    `json:"name"`
	Tracks  []string  `json:"tracks"`
	Created time.Time `json:"created"`
}

// Clone returns a deep copy.
func (p Playlist) Clone() Playlist {
	cp := p
	cp.Tracks = append([]string(nil), p.Tracks...)
	return cp
}

type fileFormat struct {
	Version   int        `json:"version"`
	Playlists []Playlist `json:"playlists"`
}

// Store is a concurrency-safe playlist collection backed by a JSON file.
type Store struct {
	mu    sync.Mutex
	path  string
	items []Playlist
}

// Load reads playlists from dataDir. A missing file is not an error.
func Load(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "playlists.json")}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse playlists: %w", err)
	}
	seen := make(map[string]bool)
	for _, p := range f.Playlists {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || utf8.RuneCountInString(p.Name) > 48 {
			continue
		}
		key := strings.ToLower(p.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		p.Tracks = dedupePaths(p.Tracks)
		if p.Created.IsZero() {
			p.Created = time.Now()
		}
		s.items = append(s.items, p.Clone())
	}
	return s, nil
}

// List returns a copy of all playlists in display order.
func (s *Store) List() []Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Playlist, len(s.items))
	for i, p := range s.items {
		out[i] = p.Clone()
	}
	return out
}

// Names returns playlist names in display order.
func (s *Store) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.items))
	for i, p := range s.items {
		out[i] = p.Name
	}
	return out
}

// Get returns a copy of the named playlist.
func (s *Store) Get(name string) (Playlist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if strings.EqualFold(p.Name, name) {
			return p.Clone(), true
		}
	}
	return Playlist{}, false
}

// Has reports whether a playlist with the given name exists.
func (s *Store) Has(name string) bool {
	_, ok := s.Get(name)
	return ok
}

// Create adds a new empty playlist.
func (s *Store) Create(name string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Playlist{}, errors.New("playlist name cannot be empty")
	}
	if utf8.RuneCountInString(name) > 48 {
		return Playlist{}, errors.New("playlist name is too long (max 48 characters)")
	}
	if strings.ContainsAny(name, "\r\n\t") {
		return Playlist{}, errors.New("playlist name contains invalid characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if strings.EqualFold(p.Name, name) {
			return Playlist{}, fmt.Errorf("playlist %q already exists", name)
		}
	}
	p := Playlist{Name: name, Created: time.Now()}
	s.items = append(s.items, p)
	if err := s.saveLocked(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return Playlist{}, err
	}
	return p.Clone(), nil
}

// Delete removes a playlist by name.
func (s *Store) Delete(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.items {
		if strings.EqualFold(p.Name, name) {
			removed := s.items[i]
			s.items = append(s.items[:i], s.items[i+1:]...)
			if err := s.saveLocked(); err != nil {
				s.items = append(s.items[:i], append([]Playlist{removed}, s.items[i:]...)...)
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// Add appends a track path to a playlist. It reports whether the path was new.
func (s *Store) Add(name, trackPath string) (bool, error) {
	added, _, err := s.AddMany(name, []string{trackPath})
	return added > 0, err
}

// Contains reports whether a playlist already holds the given track.
func (s *Store) Contains(name, trackPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if !strings.EqualFold(s.items[i].Name, name) {
			continue
		}
		for _, existing := range s.items[i].Tracks {
			if samePath(existing, trackPath) {
				return true
			}
		}
		return false
	}
	return false
}

// AddMany appends several paths at once, skipping duplicates. It reports how
// many paths were added and how many were already present.
func (s *Store) AddMany(name string, paths []string) (added, skipped int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(name)
	if idx < 0 {
		return 0, 0, fmt.Errorf("playlist %q not found", name)
	}
	backup := append([]string(nil), s.items[idx].Tracks...)
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if containsPathLocked(s.items[idx].Tracks, path) {
			skipped++
			continue
		}
		s.items[idx].Tracks = append(s.items[idx].Tracks, path)
		added++
	}
	if added == 0 {
		return 0, skipped, nil
	}
	if err := s.saveLocked(); err != nil {
		s.items[idx].Tracks = backup
		return 0, 0, err
	}
	return added, skipped, nil
}

// MoveMany moves paths from one playlist to another, skipping copies that the
// target already has. added counts new target entries, moved counts entries
// removed from the source, skipped counts duplicates in the target.
func (s *Store) MoveMany(from, to string, paths []string) (added, moved, skipped int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fromIdx, toIdx := s.indexLocked(from), s.indexLocked(to)
	if fromIdx < 0 {
		return 0, 0, 0, fmt.Errorf("playlist %q not found", from)
	}
	if toIdx < 0 {
		return 0, 0, 0, fmt.Errorf("playlist %q not found", to)
	}
	if fromIdx == toIdx {
		return 0, 0, len(paths), nil
	}

	sourceBackup := append([]string(nil), s.items[fromIdx].Tracks...)
	targetBackup := append([]string(nil), s.items[toIdx].Tracks...)

	moving := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		key := pathKey(path)
		if moving[key] {
			continue
		}
		moving[key] = true
		if containsPathLocked(s.items[toIdx].Tracks, path) {
			skipped++
		} else {
			s.items[toIdx].Tracks = append(s.items[toIdx].Tracks, path)
			added++
		}
	}

	kept := s.items[fromIdx].Tracks[:0]
	for _, existing := range s.items[fromIdx].Tracks {
		if moving[pathKey(existing)] {
			moved++
			continue
		}
		kept = append(kept, existing)
	}
	s.items[fromIdx].Tracks = kept
	if added == 0 && moved == 0 {
		return 0, 0, skipped, nil
	}
	if err := s.saveLocked(); err != nil {
		s.items[fromIdx].Tracks = sourceBackup
		s.items[toIdx].Tracks = targetBackup
		return 0, 0, 0, err
	}
	return added, moved, skipped, nil
}

// RemoveMany removes several paths from a playlist in one save.
func (s *Store) RemoveMany(name string, paths []string) (removed int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(name)
	if idx < 0 {
		return 0, fmt.Errorf("playlist %q not found", name)
	}
	drop := make(map[string]bool, len(paths))
	for _, path := range paths {
		drop[pathKey(path)] = true
	}
	backup := append([]string(nil), s.items[idx].Tracks...)
	kept := s.items[idx].Tracks[:0]
	for _, existing := range s.items[idx].Tracks {
		if drop[pathKey(existing)] {
			removed++
			continue
		}
		kept = append(kept, existing)
	}
	if removed == 0 {
		return 0, nil
	}
	s.items[idx].Tracks = kept
	if err := s.saveLocked(); err != nil {
		s.items[idx].Tracks = backup
		return 0, err
	}
	return removed, nil
}

func (s *Store) indexLocked(name string) int {
	for i := range s.items {
		if strings.EqualFold(s.items[i].Name, name) {
			return i
		}
	}
	return -1
}

func containsPathLocked(paths []string, target string) bool {
	for _, existing := range paths {
		if samePath(existing, target) {
			return true
		}
	}
	return false
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// Remove removes a track path from a playlist.
func (s *Store) Remove(name, trackPath string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if !strings.EqualFold(s.items[i].Name, name) {
			continue
		}
		for j, existing := range s.items[i].Tracks {
			if samePath(existing, trackPath) {
				removed := existing
				s.items[i].Tracks = append(s.items[i].Tracks[:j], s.items[i].Tracks[j+1:]...)
				if err := s.saveLocked(); err != nil {
					s.items[i].Tracks = append(s.items[i].Tracks[:j], append([]string{removed}, s.items[i].Tracks[j:]...)...)
					return false, err
				}
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("playlist %q not found", name)
}

// RemovePathEverywhere drops a track from every playlist, e.g. after deleting
// the underlying file.
func (s *Store) RemovePathEverywhere(trackPath string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	backup := make([]Playlist, len(s.items))
	for i, p := range s.items {
		backup[i] = p.Clone()
		kept := p.Tracks[:0]
		for _, existing := range p.Tracks {
			if samePath(existing, trackPath) {
				changed = true
				continue
			}
			kept = append(kept, existing)
		}
		s.items[i].Tracks = kept
	}
	if !changed {
		return false, nil
	}
	if err := s.saveLocked(); err != nil {
		s.items = backup
		return false, err
	}
	return true, nil
}

// CleanMissing removes paths that no longer exist from every playlist.
func (s *Store) CleanMissing() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup := make([]Playlist, len(s.items))
	removed := 0
	for i, p := range s.items {
		backup[i] = p.Clone()
		kept := p.Tracks[:0]
		for _, existing := range p.Tracks {
			if _, err := os.Stat(existing); err != nil {
				removed++
				continue
			}
			kept = append(kept, existing)
		}
		s.items[i].Tracks = kept
	}
	if removed == 0 {
		return 0, nil
	}
	if err := s.saveLocked(); err != nil {
		s.items = backup
		return 0, err
	}
	return removed, nil
}

// Save persists the current playlists.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	f := fileFormat{Version: 1, Playlists: s.items}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(s.path)
		if err2 := os.Rename(tmp, s.path); err2 != nil {
			_ = os.Remove(tmp)
			return errors.Join(err, err2)
		}
	}
	return nil
}

func dedupePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		dup := false
		for _, q := range out {
			if samePath(p, q) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// SortByName returns playlists ordered case-insensitively. It does not mutate
// the store.
func SortByName(items []Playlist) []Playlist {
	out := append([]Playlist(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
