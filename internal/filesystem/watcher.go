package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileEvent represents a file system event
type FileEvent struct {
	Path      string
	Op        fsnotify.Op
	Operation string // "create", "update", "delete", "rename"
}

// Watcher watches for file system changes
type Watcher struct {
	watcher   *fsnotify.Watcher
	events    chan FileEvent
	errors    chan error
	ignoreMap map[string]bool
	mu        sync.RWMutex
	closed    bool
}

// NewWatcher creates a new file system watcher
func NewWatcher() (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	w := &Watcher{
		watcher:   fsWatcher,
		events:    make(chan FileEvent, 100),
		errors:    make(chan error, 10),
		ignoreMap: make(map[string]bool),
	}

	go w.processEvents()

	return w, nil
}

// Add adds a path to watch (recursively)
func (w *Watcher) Add(path string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return fmt.Errorf("watcher is closed")
	}

	// Walk the directory tree and add all directories
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only watch directories (files are watched via parent directory)
		if info.IsDir() {
			if err := w.watcher.Add(p); err != nil {
				return fmt.Errorf("failed to add watch: %w", err)
			}
		}

		return nil
	})
}

// AddDir adds a directory to watch
func (w *Watcher) AddDir(dirPath string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return fmt.Errorf("watcher is closed")
	}

	return w.watcher.Add(dirPath)
}

// IgnorePath temporarily ignores events for a path
func (w *Watcher) IgnorePath(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ignoreMap[path] = true
}

// WatchPath re-enables watching for a path
func (w *Watcher) WatchPath(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.ignoreMap, path)
}

// Events returns the events channel
func (w *Watcher) Events() <-chan FileEvent {
	return w.events
}

// Errors returns the errors channel
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// Close closes the watcher
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	close(w.events)
	close(w.errors)
	return w.watcher.Close()
}

// processEvents processes file system events
func (w *Watcher) processEvents() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Check if this path should be ignored
			w.mu.RLock()
			ignored := w.ignoreMap[event.Name]
			// Also ignore any path containing .p2p-sync
			if !ignored && strings.Contains(event.Name, ".p2p-sync") {
				ignored = true
			}
			w.mu.RUnlock()

			if ignored {
				continue
			}

			// Determine operation type
			operation := "unknown"
			if event.Op&fsnotify.Create == fsnotify.Create {
				operation = "create"
			} else if event.Op&fsnotify.Write == fsnotify.Write {
				operation = "update"
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				operation = "delete"
			} else if event.Op&fsnotify.Rename == fsnotify.Rename {
				operation = "rename"
			}

			// Skip temporary files
			if filepath.Base(event.Name)[0] == '.' &&
				len(filepath.Base(event.Name)) > 1 &&
				filepath.Base(event.Name)[:2] == ".p" {
				continue
			}

			// Check if watcher is closed before sending
			w.mu.RLock()
			closed := w.closed
			w.mu.RUnlock()

			if !closed {
				select {
				case w.events <- FileEvent{
					Path:      event.Name,
					Op:        event.Op,
					Operation: operation,
				}:
				default:
					// Channel full, drop event (shouldn't happen with buffered channel)
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			default:
				// Channel full, drop error
			}
		}
	}
}
