package session

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CompletedSession holds parsed info about a completed CC session.
type CompletedSession struct {
	SessionID      string
	TranscriptPath string
	FilesChanged   []string
	WorkingDir     string
	DurationMs     int64
	ExitCode       int
}

// trackedFile tracks a JSONL transcript file being written to.
type trackedFile struct {
	path       string
	lastWrite  time.Time
	reported   bool
	reportedAt time.Time
}

// cleanupGrace is how long a reported file stays in the map before eviction.
// This allows Touch() to reset the reported flag if the file is written again.
const cleanupGrace = 5 * time.Minute

// OnComplete is called when a session is detected as complete.
type OnComplete func(s *CompletedSession)

// ProcessChecker returns true if a claude process is still running
// whose working directory matches the given transcript path's project.
type ProcessChecker func(transcriptPath string) bool

// Tracker monitors active JSONL files and detects session completion.
type Tracker struct {
	mu            sync.Mutex
	files         map[string]*trackedFile
	idleThreshold time.Duration
	pollInterval  time.Duration
	onComplete    OnComplete
	processCheck  ProcessChecker
	logger        *slog.Logger
	done          chan struct{}
}

// NewTracker creates a session tracker.
func NewTracker(idleThreshold, pollInterval time.Duration, logger *slog.Logger, onComplete OnComplete) *Tracker {
	return &Tracker{
		files:         make(map[string]*trackedFile),
		idleThreshold: idleThreshold,
		pollInterval:  pollInterval,
		onComplete:    onComplete,
		processCheck:  isClaudeRunningForTranscript,
		logger:        logger.With("component", "tracker"),
		done:          make(chan struct{}),
	}
}

// Touch marks a transcript file as recently written.
func (t *Tracker) Touch(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if tf, ok := t.files[path]; ok {
		tf.lastWrite = time.Now()
		tf.reported = false // reset if file is being written again
	} else {
		t.files[path] = &trackedFile{
			path:      path,
			lastWrite: time.Now(),
		}
		t.logger.Info("tracking new transcript", "path", path)
	}
}

// Start begins the polling loop to detect idle sessions. Blocks until Stop.
func (t *Tracker) Start() {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.check()
		case <-t.done:
			return
		}
	}
}

// Stop halts the tracker.
func (t *Tracker) Stop() {
	close(t.done)
}

func (t *Tracker) check() {
	// Collect paths to complete under the lock, then process outside it.
	var readyPaths []string

	t.mu.Lock()
	now := time.Now()
	for path, tf := range t.files {
		// Evict reported files after the grace period to prevent unbounded
		// growth of the files map. The grace window allows Touch() to reset
		// the reported flag if the file is written to again shortly after
		// completion.
		if tf.reported && !tf.reportedAt.IsZero() && now.Sub(tf.reportedAt) >= cleanupGrace {
			t.logger.Debug("evicting completed transcript from tracker", "path", path)
			delete(t.files, path)
			continue
		}

		if tf.reported {
			continue
		}

		idle := now.Sub(tf.lastWrite)
		if idle < t.idleThreshold {
			continue
		}

		// Check if claude process is still running for this transcript.
		if t.processCheck(path) {
			continue
		}

		t.logger.Info("session idle, no claude process — completing", "path", path, "idle", idle)
		tf.reported = true
		tf.reportedAt = now
		readyPaths = append(readyPaths, path)
	}
	t.mu.Unlock()

	// Parse transcripts and invoke callbacks outside the lock to avoid
	// blocking Touch() during network I/O (NATS publish, KV lookup).
	for _, path := range readyPaths {
		completed := parseTranscript(path, t.logger)
		if completed != nil {
			t.onComplete(completed)
		}
	}
}

// isClaudeRunningForTranscript checks /proc for a running claude process
// whose working directory matches the project encoded in the transcript path.
//
// Transcript paths follow the pattern:
//
//	~/.claude/projects/{project-slug}/{session-id}.jsonl
//
// The project slug encodes the absolute working directory with "/" replaced by
// "-" and leading slash dropped (e.g., "-home-mike-Warren" for "/home/mike/Warren").
// We extract this slug, then compare it against each claude process's cwd
// (read from /proc/<pid>/cwd). If the transcript path doesn't contain the
// expected structure, we fall back to the global "any claude process" check.
func isClaudeRunningForTranscript(transcriptPath string) bool {
	projectDir := projectDirFromTranscript(transcriptPath)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only look at numeric PID dirs.
		name := entry.Name()
		if len(name) == 0 || name[0] < '0' || name[0] > '9' {
			continue
		}

		cmdline, err := os.ReadFile(filepath.Join("/proc", name, "cmdline"))
		if err != nil {
			continue
		}

		// cmdline is null-separated; check if any arg contains "claude"
		isClaude := false
		parts := strings.Split(string(cmdline), "\x00")
		for _, part := range parts {
			base := filepath.Base(part)
			if base == "claude" || strings.HasPrefix(base, "claude-") {
				isClaude = true
				break
			}
		}
		if !isClaude {
			continue
		}

		// If we have no project dir to match against, fall back to
		// "any claude process is running".
		if projectDir == "" {
			return true
		}

		// Read the process's working directory and compare.
		cwd, err := os.Readlink(filepath.Join("/proc", name, "cwd"))
		if err != nil {
			continue
		}
		if cwd == projectDir {
			return true
		}
	}

	return false
}

// projectDirFromTranscript extracts the working directory from a transcript
// path. Claude Code stores transcripts at:
//
//	~/.claude/projects/{project-slug}/{session-id}.jsonl
//
// The project slug is the absolute path with "/" replaced by "-" and the
// leading slash dropped. For example, "/home/mike/Warren" becomes
// "-home-mike-Warren". We reverse this encoding to recover the original path
// and verify the directory exists. If the path doesn't match the expected
// layout or the decoded directory doesn't exist, we return "" to signal that
// the caller should fall back to the global check.
func projectDirFromTranscript(transcriptPath string) string {
	// Walk up to find the "projects" component.
	dir := filepath.Dir(transcriptPath) // e.g., ~/.claude/projects/-home-mike-Warren
	parent := filepath.Dir(dir)         // e.g., ~/.claude/projects
	if filepath.Base(parent) != "projects" {
		return ""
	}

	slug := filepath.Base(dir) // e.g., "-home-mike-Warren"
	if slug == "" || slug == "." {
		return ""
	}

	// The slug is the absolute path with each "/" replaced by "-".
	// Reconstruct by replacing "-" with "/".
	// The slug starts with "-" because the absolute path starts with "/".
	candidate := strings.ReplaceAll(slug, "-", "/")

	// Verify the directory actually exists to avoid false positives.
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return ""
	}

	return candidate
}
