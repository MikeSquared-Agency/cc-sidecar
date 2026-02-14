package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newTestTracker creates a tracker with process check disabled (always returns false).
func newTestTracker(idleThreshold, pollInterval time.Duration, onComplete OnComplete) *Tracker {
	t := NewTracker(idleThreshold, pollInterval, testLogger(), onComplete)
	t.processCheck = func(string) bool { return false } // no claude process in tests
	return t
}

func TestTrackerTouchAndCheck(t *testing.T) {
	var mu sync.Mutex
	var completed []*CompletedSession

	tracker := newTestTracker(
		50*time.Millisecond,
		20*time.Millisecond,
		func(s *CompletedSession) {
			mu.Lock()
			completed = append(completed, s)
			mu.Unlock()
		},
	)

	go tracker.Start()
	defer tracker.Stop()

	dir := t.TempDir()
	path := dir + "/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl"
	content := `{"type":"summary","sessionId":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","timestamp":"2026-02-14T10:00:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	tracker.Touch(path)

	// Wait for idle detection.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(completed)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 completed session, got %d", count)
	}
}

func TestTrackerDoesNotDoubleReport(t *testing.T) {
	var mu sync.Mutex
	var count int

	tracker := newTestTracker(
		50*time.Millisecond,
		20*time.Millisecond,
		func(s *CompletedSession) {
			mu.Lock()
			count++
			mu.Unlock()
		},
	)

	go tracker.Start()
	defer tracker.Stop()

	dir := t.TempDir()
	path := dir + "/11111111-2222-3333-4444-555555555555.jsonl"
	content := `{"type":"summary","sessionId":"11111111-2222-3333-4444-555555555555","timestamp":"2026-02-14T10:00:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)
	tracker.Touch(path)

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	c := count
	mu.Unlock()

	if c != 1 {
		t.Errorf("expected exactly 1 completion callback, got %d", c)
	}
}

func TestTrackerResetOnNewWrite(t *testing.T) {
	var mu sync.Mutex
	var count int

	tracker := newTestTracker(
		100*time.Millisecond,
		20*time.Millisecond,
		func(s *CompletedSession) {
			mu.Lock()
			count++
			mu.Unlock()
		},
	)

	go tracker.Start()
	defer tracker.Stop()

	dir := t.TempDir()
	path := dir + "/22222222-3333-4444-5555-666666666666.jsonl"
	content := `{"type":"summary","sessionId":"22222222-3333-4444-5555-666666666666","timestamp":"2026-02-14T10:00:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)
	tracker.Touch(path)

	// Touch again before idle threshold to reset the timer.
	time.Sleep(50 * time.Millisecond)
	tracker.Touch(path)

	mu.Lock()
	c := count
	mu.Unlock()
	if c != 0 {
		t.Errorf("expected 0 completions at this point, got %d", c)
	}
}

func TestTrackerProcessRunningBlocksCompletion(t *testing.T) {
	var mu sync.Mutex
	var count int

	tracker := NewTracker(
		50*time.Millisecond,
		20*time.Millisecond,
		testLogger(),
		func(s *CompletedSession) {
			mu.Lock()
			count++
			mu.Unlock()
		},
	)
	// Process is "always running" — should never trigger completion.
	tracker.processCheck = func(string) bool { return true }

	go tracker.Start()
	defer tracker.Stop()

	dir := t.TempDir()
	path := dir + "/33333333-4444-5555-6666-777777777777.jsonl"
	os.WriteFile(path, []byte(`{"type":"summary","sessionId":"33333333-4444-5555-6666-777777777777","timestamp":"2026-02-14T10:00:00Z"}`+"\n"), 0644)
	tracker.Touch(path)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	c := count
	mu.Unlock()
	if c != 0 {
		t.Errorf("expected 0 completions while process running, got %d", c)
	}
}

func TestProjectDirFromTranscript(t *testing.T) {
	// Create a real directory to act as the "project dir" so that
	// projectDirFromTranscript's os.Stat check passes.
	realDir := t.TempDir()

	// Build a slug from the real temp dir path: replace "/" with "-".
	// e.g., "/tmp/TestXyz123" -> "-tmp-TestXyz123"
	slug := pathToSlug(realDir)

	// Build a fake transcript path:
	// <somewhere>/projects/<slug>/session.jsonl
	fakeBase := filepath.Join(t.TempDir(), "projects", slug)
	os.MkdirAll(fakeBase, 0755)
	transcriptPath := filepath.Join(fakeBase, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")

	got := projectDirFromTranscript(transcriptPath)
	if got != realDir {
		t.Errorf("projectDirFromTranscript(%q) = %q, want %q", transcriptPath, got, realDir)
	}
}

func TestProjectDirFromTranscript_NoProjectsParent(t *testing.T) {
	// Path without "projects" parent directory should return "".
	got := projectDirFromTranscript("/some/random/path/session.jsonl")
	if got != "" {
		t.Errorf("expected empty string for non-projects path, got %q", got)
	}
}

func TestProjectDirFromTranscript_NonexistentDir(t *testing.T) {
	// Slug that decodes to a non-existent directory.
	transcriptPath := "/home/mike/.claude/projects/-nonexistent-path-that-does-not-exist/session.jsonl"
	got := projectDirFromTranscript(transcriptPath)
	if got != "" {
		t.Errorf("expected empty string for non-existent decoded dir, got %q", got)
	}
}

// pathToSlug converts an absolute path to a Claude project slug.
// e.g., "/home/mike/Warren" -> "-home-mike-Warren"
func pathToSlug(p string) string {
	result := make([]byte, len(p))
	for i, c := range []byte(p) {
		if c == '/' {
			result[i] = '-'
		} else {
			result[i] = c
		}
	}
	return string(result)
}
