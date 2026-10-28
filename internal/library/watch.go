package library

import (
	"errors"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	watchDebounce = 500 * time.Millisecond
	watchPoll     = 30 * time.Second
)

// Watcher reports that the music directory changed. Changes are coalesced so a
// download or a large copy produces one refresh instead of hundreds.
type Watcher struct {
	Changes <-chan struct{}

	root     string
	out      chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	fsw *fsnotify.Watcher
	wg  sync.WaitGroup
}

// NewWatcher watches root recursively. If the platform watcher cannot be
// created (for example on some network shares), it automatically falls back to
// polling the directory once every 30 seconds.
func NewWatcher(root string) (*Watcher, error) {
	root = filepath.Clean(root)
	out := make(chan struct{}, 1)
	w := &Watcher{
		Changes: out,
		root:    root,
		out:     out,
		done:    make(chan struct{}),
	}

	fsw, err := fsnotify.NewWatcher()
	if err == nil {
		if addErr := addRecursive(fsw, root); addErr == nil {
			w.fsw = fsw
			w.wg.Add(1)
			go w.loop()
			return w, nil
		}
		_ = fsw.Close()
	}

	// Network shares and unusual filesystems may not implement native
	// notifications. Polling is slower but keeps the UI automatic.
	w.wg.Add(1)
	go w.pollLoop()
	return w, nil
}

// Close stops the watcher and releases its OS handles.
func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	w.stopOnce.Do(func() {
		close(w.done)
		if w.fsw != nil {
			_ = w.fsw.Close()
		}
		w.wg.Wait()
		close(w.out)
	})
	return nil
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	reset := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(watchDebounce)
	}

	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !isHiddenName(filepath.Base(event.Name)) {
					_ = addRecursive(w.fsw, event.Name)
				}
			}
			if isRelevantEvent(event.Name, event.Op) {
				reset()
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			if err != nil && !errors.Is(err, fsnotify.ErrEventOverflow) {
				continue
			}
			reset()
		case <-timer.C:
			w.notify()
		}
	}
}

func (w *Watcher) pollLoop() {
	defer w.wg.Done()
	last := directorySignature(w.root)
	ticker := time.NewTicker(watchPoll)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			current := directorySignature(w.root)
			if current != last {
				last = current
				w.notify()
			}
		}
	}
}

func (w *Watcher) notify() {
	select {
	case w.out <- struct{}{}:
	default:
	}
}

func addRecursive(fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isHiddenName(d.Name()) {
			return filepath.SkipDir
		}
		return fsw.Add(path)
	})
}

func isRelevantEvent(path string, op fsnotify.Op) bool {
	base := filepath.Base(path)
	if isHiddenName(base) {
		return false
	}
	if IsSupported(path) {
		return true
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return true
	}
	if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	// Removed directories have no extension. Refreshing for these is cheap and
	// ensures their tracks disappear from the library.
	if filepath.Ext(base) == "" {
		if _, err := os.Stat(path); err != nil {
			return true
		}
	}
	return false
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// directorySignature is only used by the polling fallback. It includes the
// supported files and their size/mtime, so a tag edit, rename, copy or delete
// changes the signature.
func directorySignature(root string) uint64 {
	h := fnv.New64a()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && isHiddenName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsSupported(path) || isHiddenName(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		_, _ = h.Write([]byte(filepath.Clean(path)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(info.ModTime().UTC().Format(time.RFC3339Nano)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strconv.FormatInt(info.Size(), 10)))
		return nil
	})
	return h.Sum64()
}
