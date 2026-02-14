package session

import (
	"os"
	"sync"
	"testing"
	"time"
)

// newTestTracker creates a tracker with process check disabled (always returns false).
func newTestTracker(idleThreshold, pollInterval time.Duration, onComplete OnComplete) *Tracker {
	t := NewTracker(idleThreshold, pollInterval, testLogger(), onComplete)
	t.processCheck = func() bool { return false } // no claude process in tests
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
	tracker.processCheck = func() bool { return true }

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
