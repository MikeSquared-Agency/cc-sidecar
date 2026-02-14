package publisher

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionDataJSON(t *testing.T) {
	data := SessionData{
		SessionID:      "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		AgentType:      "claude-code",
		TranscriptPath: "/home/mike/.claude/projects/-home-mike/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl",
		FilesChanged:   []string{"/home/mike/main.go"},
		ExitCode:       0,
		DurationMs:     60000,
		WorkingDir:     "/home/mike",
		Timestamp:      "2026-02-14T10:00:00Z",
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if decoded["session_id"] != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("session_id = %v, want aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", decoded["session_id"])
	}
	if decoded["agent_type"] != "claude-code" {
		t.Errorf("agent_type = %v, want claude-code", decoded["agent_type"])
	}
	if decoded["working_dir"] != "/home/mike" {
		t.Errorf("working_dir = %v, want /home/mike", decoded["working_dir"])
	}
	if int(decoded["exit_code"].(float64)) != 0 {
		t.Errorf("exit_code = %v, want 0", decoded["exit_code"])
	}
	if int(decoded["duration_ms"].(float64)) != 60000 {
		t.Errorf("duration_ms = %v, want 60000", decoded["duration_ms"])
	}
}

func TestSessionDataOmitempty(t *testing.T) {
	// TaskID and OwnerUUID should be omitted when empty.
	data := SessionData{
		SessionID: "test-session",
		AgentType: "claude-code",
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if _, ok := decoded["task_id"]; ok {
		t.Error("expected task_id to be omitted when empty")
	}
	if _, ok := decoded["owner_uuid"]; ok {
		t.Error("expected owner_uuid to be omitted when empty")
	}
}

func TestSessionDataWithTaskID(t *testing.T) {
	data := SessionData{
		SessionID: "test-session",
		TaskID:    "task-123",
		OwnerUUID: "owner-456",
		AgentType: "claude-code",
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if decoded["task_id"] != "task-123" {
		t.Errorf("task_id = %v, want task-123", decoded["task_id"])
	}
	if decoded["owner_uuid"] != "owner-456" {
		t.Errorf("owner_uuid = %v, want owner-456", decoded["owner_uuid"])
	}
}

func TestEventEnvelopeStructure(t *testing.T) {
	sessionData := SessionData{
		SessionID: "test-session",
		AgentType: "claude-code",
		Timestamp: "2026-02-14T10:00:00Z",
	}

	dataRaw, err := json.Marshal(sessionData)
	if err != nil {
		t.Fatalf("marshal session data failed: %v", err)
	}

	ev := Event{
		ID:        "evt-001",
		Type:      "cc.session.completed",
		Source:    "cc-sidecar",
		Timestamp: time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC),
		Data:      dataRaw,
	}

	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if decoded["id"] != "evt-001" {
		t.Errorf("id = %v, want evt-001", decoded["id"])
	}
	if decoded["type"] != "cc.session.completed" {
		t.Errorf("type = %v, want cc.session.completed", decoded["type"])
	}
	if decoded["source"] != "cc-sidecar" {
		t.Errorf("source = %v, want cc-sidecar", decoded["source"])
	}

	// Verify nested data can be unpacked.
	dataField, ok := decoded["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data field to be an object")
	}
	if dataField["session_id"] != "test-session" {
		t.Errorf("data.session_id = %v, want test-session", dataField["session_id"])
	}
}

func TestEventRoundTrip(t *testing.T) {
	sessionData := SessionData{
		SessionID:      "round-trip-session",
		TaskID:         "task-rt",
		AgentType:      "claude-code",
		TranscriptPath: "/some/path.jsonl",
		FilesChanged:   []string{"/a.go", "/b.go"},
		ExitCode:       1,
		DurationMs:     5000,
		WorkingDir:     "/home/mike",
		Timestamp:      "2026-02-14T12:00:00Z",
	}

	dataRaw, _ := json.Marshal(sessionData)

	ev := Event{
		ID:        "evt-rt",
		Type:      "cc.session.failed",
		Source:    "cc-sidecar",
		Timestamp: time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC),
		Data:      dataRaw,
	}

	evBytes, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(evBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ID != ev.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, ev.ID)
	}
	if decoded.Type != ev.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, ev.Type)
	}
	if decoded.Source != ev.Source {
		t.Errorf("Source = %q, want %q", decoded.Source, ev.Source)
	}
	if !decoded.Timestamp.Equal(ev.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, ev.Timestamp)
	}

	var decodedData SessionData
	if err := json.Unmarshal(decoded.Data, &decodedData); err != nil {
		t.Fatalf("unmarshal nested data: %v", err)
	}
	if decodedData.SessionID != "round-trip-session" {
		t.Errorf("SessionID = %q, want round-trip-session", decodedData.SessionID)
	}
	if decodedData.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", decodedData.ExitCode)
	}
	if len(decodedData.FilesChanged) != 2 {
		t.Errorf("FilesChanged count = %d, want 2", len(decodedData.FilesChanged))
	}
}
