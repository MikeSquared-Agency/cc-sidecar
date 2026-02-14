package registry

import (
	"encoding/json"
	"testing"
)

func TestTaskMappingJSON(t *testing.T) {
	mapping := TaskMapping{
		TaskID:    "task-abc-123",
		OwnerUUID: "owner-def-456",
	}

	raw, err := json.Marshal(mapping)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded TaskMapping
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.TaskID != "task-abc-123" {
		t.Errorf("TaskID = %q, want task-abc-123", decoded.TaskID)
	}
	if decoded.OwnerUUID != "owner-def-456" {
		t.Errorf("OwnerUUID = %q, want owner-def-456", decoded.OwnerUUID)
	}
}

func TestTaskMappingJSONFields(t *testing.T) {
	mapping := TaskMapping{
		TaskID:    "t1",
		OwnerUUID: "o1",
	}

	raw, err := json.Marshal(mapping)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	// Verify the JSON field names are snake_case as expected.
	if _, ok := decoded["task_id"]; !ok {
		t.Error("expected JSON field 'task_id'")
	}
	if _, ok := decoded["owner_uuid"]; !ok {
		t.Error("expected JSON field 'owner_uuid'")
	}
}

func TestTaskMappingUnmarshalFromKV(t *testing.T) {
	// Simulate what a KV entry value would look like.
	kvValue := []byte(`{"task_id":"task-from-kv","owner_uuid":"uuid-from-kv"}`)

	var mapping TaskMapping
	if err := json.Unmarshal(kvValue, &mapping); err != nil {
		t.Fatalf("unmarshal KV value failed: %v", err)
	}

	if mapping.TaskID != "task-from-kv" {
		t.Errorf("TaskID = %q, want task-from-kv", mapping.TaskID)
	}
	if mapping.OwnerUUID != "uuid-from-kv" {
		t.Errorf("OwnerUUID = %q, want uuid-from-kv", mapping.OwnerUUID)
	}
}

func TestBucketName(t *testing.T) {
	// Verify the bucket name constant hasn't been accidentally changed.
	if bucketName != "CC_SESSION_REGISTRY" {
		t.Errorf("bucketName = %q, want CC_SESSION_REGISTRY", bucketName)
	}
}
