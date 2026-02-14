package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Toucher is satisfied by session.Tracker — accepts file paths on write events.
type Toucher interface {
	Touch(path string)
}

// Watcher monitors ~/.claude/projects/ for JSONL transcript changes.
type Watcher struct {
	dir     string
	tracker Toucher
	logger  *slog.Logger
	fw      *fsnotify.Watcher
	done    chan struct{}
}

// New creates a new transcript watcher.
func New(dir string, tracker Toucher, logger *slog.Logger) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		dir:     dir,
		tracker: tracker,
		logger:  logger.With("component", "watcher"),
		fw:      fw,
		done:    make(chan struct{}),
	}

	// Add the root dir and all subdirectories.
	if err := w.addRecursive(dir); err != nil {
		fw.Close()
		return nil, err
	}

	return w, nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if d.IsDir() {
			if err := w.fw.Add(path); err != nil {
				w.logger.Warn("could not watch directory", "path", path, "error", err)
			}
		}
		return nil
	})
}

// Start begins watching for file events. Blocks until Stop is called.
func (w *Watcher) Start() {
	w.logger.Info("watching for transcript changes", "dir", w.dir)

	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			w.logger.Error("watcher error", "error", err)
		case <-w.done:
			return
		}
	}
}

// Stop halts the watcher.
func (w *Watcher) Stop() {
	close(w.done)
	w.fw.Close()
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Watch for new directories (new project dirs).
	if event.Op&fsnotify.Create != 0 {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			if err := w.fw.Add(event.Name); err != nil {
				w.logger.Warn("could not watch new directory", "path", event.Name, "error", err)
			}
			return
		}
	}

	// Only care about .jsonl file writes.
	if !strings.HasSuffix(event.Name, ".jsonl") {
		return
	}

	if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
		w.tracker.Touch(event.Name)
	}
}
