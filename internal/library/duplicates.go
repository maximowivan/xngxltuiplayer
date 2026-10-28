package library

import (
	"crypto/sha256"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"tuiplay/internal/model"
)

const duplicateDurationTolerance = 2 * time.Second

// DuplicateGroup is a set of tracks that look like the same recording. Exact
// reports whether the files have identical SHA-256 content.
type DuplicateGroup struct {
	Key    string
	Title  string
	Artist string
	Tracks []model.Track
	Exact  bool
}

// FindDuplicates groups the library by normalized artist/title and a small
// duration tolerance. It does not alter any files.
func (l *Library) FindDuplicates() ([]DuplicateGroup, error) {
	tracks := l.Tracks()
	buckets := make(map[string][]model.Track)
	for _, t := range tracks {
		title := normalizeDuplicateText(t.Title)
		if title == "" {
			continue
		}
		artist := normalizeDuplicateText(t.Artist)
		key := artist + "|" + title
		buckets[key] = append(buckets[key], t)
	}

	var groups []DuplicateGroup
	for key, bucket := range buckets {
		if len(bucket) < 2 {
			continue
		}
		for _, cluster := range clusterByDuration(bucket) {
			if len(cluster) < 2 {
				continue
			}
			groups = append(groups, DuplicateGroup{
				Key:    key,
				Title:  cluster[0].DisplayTitle(),
				Artist: cluster[0].DisplayArtist(),
				Tracks: cluster,
				Exact:  sameFileContent(cluster),
			})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Tracks) != len(groups[j].Tracks) {
			return len(groups[i].Tracks) > len(groups[j].Tracks)
		}
		if groups[i].Artist != groups[j].Artist {
			return groups[i].Artist < groups[j].Artist
		}
		return groups[i].Title < groups[j].Title
	})
	return groups, nil
}

func clusterByDuration(tracks []model.Track) [][]model.Track {
	sorted := append([]model.Track(nil), tracks...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Duration < sorted[j].Duration
	})
	var clusters [][]model.Track
	for _, track := range sorted {
		placed := false
		for i := range clusters {
			ref := clusters[i][0].Duration
			if ref <= 0 || track.Duration <= 0 || absDuration(ref-track.Duration) <= duplicateDurationTolerance {
				clusters[i] = append(clusters[i], track)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, []model.Track{track})
		}
	}
	return clusters
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func normalizeDuplicateText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func sameFileContent(tracks []model.Track) bool {
	if len(tracks) < 2 {
		return false
	}
	var first [sha256.Size]byte
	haveFirst := false
	for _, track := range tracks {
		hash, err := sha256File(track.Path)
		if err != nil {
			return false
		}
		if !haveFirst {
			first = hash
			haveFirst = true
			continue
		}
		if hash != first {
			return false
		}
	}
	return true
}

func sha256File(path string) ([sha256.Size]byte, error) {
	var out [sha256.Size]byte
	f, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return out, err
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}
