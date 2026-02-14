package session

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jsonlLine represents a line from a Claude Code JSONL transcript.
type jsonlLine struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
	CWD       string          `json:"cwd"`

	// Tool use fields (nested in message).
	ToolName string `json:"-"`
	FilePath string `json:"-"`
}

// messageContent is used for extracting tool_use data.
type messageContent struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type toolInput struct {
	FilePath string `json:"file_path"`
}

// parseTranscript extracts session metadata from a JSONL transcript file.
func parseTranscript(path string, logger *slog.Logger) *CompletedSession {
	f, err := os.Open(path)
	if err != nil {
		logger.Error("failed to open transcript", "path", path, "error", err)
		return nil
	}
	defer f.Close()

	var (
		sessionID        string
		workingDir       string
		filesChanged     = make(map[string]bool)
		firstTS          time.Time
		lastTS           time.Time
		hasAssistantMsg  bool
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		// Extract session ID.
		if entry.SessionID != "" && sessionID == "" {
			sessionID = entry.SessionID
		}

		// Extract working directory.
		if entry.CWD != "" && workingDir == "" {
			workingDir = entry.CWD
		}

		// Extract timestamps.
		if entry.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				if firstTS.IsZero() {
					firstTS = ts
				}
				lastTS = ts
			}
		}

		// Track whether the session produced any assistant responses.
		if entry.Type == "assistant" {
			hasAssistantMsg = true
		}

		// Extract file changes from tool_use entries.
		extractFileChanges(line, filesChanged)
	}

	if scanner.Err() != nil {
		logger.Warn("scanner error reading transcript", "path", path, "error", scanner.Err())
	}

	// Fall back to extracting session ID from filename.
	if sessionID == "" {
		sessionID = extractSessionIDFromPath(path)
	}

	if sessionID == "" {
		logger.Warn("could not determine session ID", "path", path)
		return nil
	}

	files := make([]string, 0, len(filesChanged))
	for f := range filesChanged {
		files = append(files, f)
	}

	var durationMs int64
	if !firstTS.IsZero() && !lastTS.IsZero() {
		durationMs = lastTS.Sub(firstTS).Milliseconds()
	}

	// Determine exit code heuristically. CC JSONL transcripts don't record
	// explicit exit codes. A session with no assistant messages likely indicates
	// a startup failure or crash.
	exitCode := 0
	if !hasAssistantMsg {
		exitCode = 1
	}

	return &CompletedSession{
		SessionID:      sessionID,
		TranscriptPath: path,
		FilesChanged:   files,
		WorkingDir:     workingDir,
		DurationMs:     durationMs,
		ExitCode:       exitCode,
	}
}

// extractFileChanges looks for Write/Edit tool_use blocks and extracts file_path.
func extractFileChanges(line []byte, files map[string]bool) {
	var msg struct {
		Message messageContent `json:"message"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return
	}

	if msg.Message.Role != "assistant" {
		return
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Message.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		if block.Name != "Write" && block.Name != "Edit" {
			continue
		}

		var input toolInput
		if err := json.Unmarshal(block.Input, &input); err != nil {
			continue
		}
		if input.FilePath != "" {
			files[input.FilePath] = true
		}
	}
}

// extractSessionIDFromPath pulls a UUID-like portion from the transcript filename.
func extractSessionIDFromPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".jsonl")

	// The filename is typically a UUID like cfa3335c-ea38-4cf8-a1f2-9e3cf9789708
	// or may have a prefix. Return the last UUID-length segment.
	if len(base) >= 36 {
		// Try to find a UUID pattern (8-4-4-4-12).
		candidate := base[len(base)-36:]
		if isUUIDLike(candidate) {
			return candidate
		}
	}

	return base
}

func isUUIDLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
