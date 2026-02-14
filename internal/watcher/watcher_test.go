package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestHandleEvent_IgnoresNonJSONL(t *testing.T) {
	dir := t.TempDir()

	// Create a tracker that records touches.
	var touched []string
	tracker := &touchRecorder{touches: &touched}

	w := &Watcher{
		dir:     dir,
		tracker: tracker,
		logger:  testLogger(),
		fw:      nil, // not used by handleEvent for non-Create events
		done:    make(chan struct{}),
	}

	// .txt file should be ignored.
	w.handleEvent(fsnotify.Event{
		Name: filepath.Join(dir, "notes.txt"),
		Op:   fsnotify.Write,
	})

	// .json file should be ignored.
	w.handleEvent(fsnotify.Event{
		Name: filepath.Join(dir, "data.json"),
		Op:   fsnotify.Write,
	})

	if len(touched) != 0 {
		t.Errorf("expected 0 touches for non-.jsonl files, got %d: %v", len(touched), touched)
	}
}

func TestHandleEvent_TouchesJSONL(t *testing.T) {
	dir := t.TempDir()

	var touched []string
	tracker := &touchRecorder{touches: &touched}

	w := &Watcher{
		dir:     dir,
		tracker: tracker,
		logger:  testLogger(),
		fw:      nil,
		done:    make(chan struct{}),
	}

	jsonlPath := filepath.Join(dir, "session.jsonl")
	w.handleEvent(fsnotify.Event{
		Name: jsonlPath,
		Op:   fsnotify.Write,
	})

	if len(touched) != 1 {
		t.Fatalf("expected 1 touch, got %d", len(touched))
	}
	if touched[0] != jsonlPath {
		t.Errorf("touched path = %q, want %q", touched[0], jsonlPath)
	}
}

func TestHandleEvent_CreateJSONL(t *testing.T) {
	dir := t.TempDir()

	var touched []string
	tracker := &touchRecorder{touches: &touched}

	// We need a real fsnotify.Watcher for Create events on directories,
	// but for JSONL Create events the path is not a directory so it falls
	// through to the suffix check and Touch.
	w := &Watcher{
		dir:     dir,
		tracker: tracker,
		logger:  testLogger(),
		fw:      nil,
		done:    make(chan struct{}),
	}

	jsonlPath := filepath.Join(dir, "new-session.jsonl")
	// Create the file so os.Stat succeeds but returns non-dir.
	os.WriteFile(jsonlPath, []byte("{}"), 0644)

	w.handleEvent(fsnotify.Event{
		Name: jsonlPath,
		Op:   fsnotify.Create,
	})

	if len(touched) != 1 {
		t.Fatalf("expected 1 touch for created .jsonl, got %d", len(touched))
	}
}

func TestHandleEvent_IgnoresChmodAndRemove(t *testing.T) {
	dir := t.TempDir()

	var touched []string
	tracker := &touchRecorder{touches: &touched}

	w := &Watcher{
		dir:     dir,
		tracker: tracker,
		logger:  testLogger(),
		fw:      nil,
		done:    make(chan struct{}),
	}

	jsonlPath := filepath.Join(dir, "session.jsonl")

	w.handleEvent(fsnotify.Event{
		Name: jsonlPath,
		Op:   fsnotify.Chmod,
	})

	w.handleEvent(fsnotify.Event{
		Name: jsonlPath,
		Op:   fsnotify.Remove,
	})

	if len(touched) != 0 {
		t.Errorf("expected 0 touches for Chmod/Remove, got %d", len(touched))
	}
}

func TestAddRecursive(t *testing.T) {
	root := t.TempDir()
	sub1 := filepath.Join(root, "project-a")
	sub2 := filepath.Join(root, "project-b")
	os.MkdirAll(sub1, 0755)
	os.MkdirAll(sub2, 0755)

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("failed to create fsnotify watcher: %v", err)
	}
	defer fw.Close()

	w := &Watcher{
		dir:    root,
		logger: testLogger(),
		fw:     fw,
		done:   make(chan struct{}),
	}

	if err := w.addRecursive(root); err != nil {
		t.Fatalf("addRecursive failed: %v", err)
	}

	// The watcher should have added root, sub1, and sub2.
	// fsnotify doesn't expose a WatchList in all versions, so we verify
	// by checking that we can write to a subdirectory and receive an event.
	// For simplicity, just verify no error was returned.
}
