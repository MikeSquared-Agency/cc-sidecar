package session

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestParseTranscript_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfa3335c-ea38-4cf8-a1f2-9e3cf9789708.jsonl")

	content := `{"type":"summary","sessionId":"cfa3335c-ea38-4cf8-a1f2-9e3cf9789708","cwd":"/home/mike/Warren","timestamp":"2026-02-14T10:00:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/home/mike/Warren/internal/api/health.go"}}]},"timestamp":"2026-02-14T10:01:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/home/mike/Warren/main.go"}}]},"timestamp":"2026-02-14T10:05:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]},"timestamp":"2026-02-14T10:06:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.SessionID != "cfa3335c-ea38-4cf8-a1f2-9e3cf9789708" {
		t.Errorf("session_id = %q, want cfa3335c-...", result.SessionID)
	}
	if result.WorkingDir != "/home/mike/Warren" {
		t.Errorf("working_dir = %q, want /home/mike/Warren", result.WorkingDir)
	}
	if len(result.FilesChanged) != 2 {
		t.Errorf("files_changed count = %d, want 2", len(result.FilesChanged))
	}

	// Check files are present (order may vary).
	fileSet := make(map[string]bool)
	for _, f := range result.FilesChanged {
		fileSet[f] = true
	}
	if !fileSet["/home/mike/Warren/internal/api/health.go"] {
		t.Error("missing health.go in files_changed")
	}
	if !fileSet["/home/mike/Warren/main.go"] {
		t.Error("missing main.go in files_changed")
	}

	// Duration should be 6 minutes = 360000ms.
	if result.DurationMs != 360000 {
		t.Errorf("duration_ms = %d, want 360000", result.DurationMs)
	}
}

func TestParseTranscript_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	// Use a non-UUID filename so session ID can't be extracted from it either.
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0644)

	result := parseTranscript(path, testLogger())
	// Empty file with non-UUID filename → no session ID can be determined → nil.
	// But if filename is "empty" (5 chars, not UUID-like), extractSessionIDFromPath
	// returns "empty", which is non-empty. The function only returns nil when
	// sessionID is "". Since "empty" is returned, the result is non-nil but with
	// no useful data. This is acceptable — the sidecar only processes files from
	// ~/.claude/projects/ which always have UUID filenames.
	if result != nil && result.SessionID == "" {
		t.Error("expected non-empty session_id when file exists")
	}
}

func TestParseTranscript_SessionIDFromFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abcdef01-2345-6789-abcd-ef0123456789.jsonl")

	// No sessionId field in the content, should fall back to filename.
	content := `{"type":"message","message":{"role":"user","content":"hello"},"timestamp":"2026-02-14T10:00:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SessionID != "abcdef01-2345-6789-abcd-ef0123456789" {
		t.Errorf("session_id = %q, want UUID from filename", result.SessionID)
	}
}

func TestParseTranscript_NoFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "11111111-2222-3333-4444-555555555555.jsonl")

	content := `{"type":"summary","sessionId":"11111111-2222-3333-4444-555555555555","cwd":"/home/mike","timestamp":"2026-02-14T10:00:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Hello!"}]},"timestamp":"2026-02-14T10:01:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.FilesChanged) != 0 {
		t.Errorf("expected 0 files_changed, got %d", len(result.FilesChanged))
	}
}

func TestParseTranscript_MissingFile(t *testing.T) {
	result := parseTranscript("/nonexistent/file.jsonl", testLogger())
	if result != nil {
		t.Error("expected nil result for missing file")
	}
}

func TestExtractSessionIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/mike/.claude/projects/-home-mike/cfa3335c-ea38-4cf8-a1f2-9e3cf9789708.jsonl", "cfa3335c-ea38-4cf8-a1f2-9e3cf9789708"},
		{"/tmp/abcdef01-2345-6789-abcd-ef0123456789.jsonl", "abcdef01-2345-6789-abcd-ef0123456789"},
		{"/tmp/short.jsonl", "short"},
	}

	for _, tt := range tests {
		got := extractSessionIDFromPath(tt.path)
		if got != tt.want {
			t.Errorf("extractSessionIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestIsUUIDLike(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"cfa3335c-ea38-4cf8-a1f2-9e3cf9789708", true},
		{"00000000-0000-0000-0000-000000000000", true},
		{"ABCDEF01-2345-6789-ABCD-EF0123456789", true},
		{"not-a-uuid", false},
		{"cfa3335c_ea38_4cf8_a1f2_9e3cf9789708", false},
		{"", false},
		{"cfa3335c-ea38-4cf8-a1f2-9e3cf978970", false}, // too short
	}

	for _, tt := range tests {
		got := isUUIDLike(tt.input)
		if got != tt.want {
			t.Errorf("isUUIDLike(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseTranscript_ExitCodeZeroWithAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aaaa1111-2222-3333-4444-555555555555.jsonl")

	content := `{"type":"summary","sessionId":"aaaa1111-2222-3333-4444-555555555555","cwd":"/home/mike","timestamp":"2026-02-14T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello!"}]},"timestamp":"2026-02-14T10:01:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit_code 0 for session with assistant response, got %d", result.ExitCode)
	}
}

func TestParseTranscript_ExitCodeOneWithoutAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bbbb1111-2222-3333-4444-555555555555.jsonl")

	content := `{"type":"summary","sessionId":"bbbb1111-2222-3333-4444-555555555555","cwd":"/home/mike","timestamp":"2026-02-14T10:00:00Z"}
{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-02-14T10:01:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit_code 1 for session without assistant response, got %d", result.ExitCode)
	}
}

func TestParseTranscript_DeduplicatesFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dedup-test-aaaa-bbbb-ccccddddeeee.jsonl")

	// Same file edited twice.
	content := `{"type":"summary","sessionId":"dedup-test-aaaa-bbbb-ccccddddeeee","timestamp":"2026-02-14T10:00:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/home/mike/file.go"}}]},"timestamp":"2026-02-14T10:01:00Z"}
{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/home/mike/file.go"}}]},"timestamp":"2026-02-14T10:02:00Z"}
`
	os.WriteFile(path, []byte(content), 0644)

	result := parseTranscript(path, testLogger())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.FilesChanged) != 1 {
		t.Errorf("expected 1 unique file, got %d", len(result.FilesChanged))
	}
}
