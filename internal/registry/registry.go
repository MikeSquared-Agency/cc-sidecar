package registry

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const bucketName = "CC_SESSION_REGISTRY"

// TaskMapping holds the task_id and owner_uuid for a CC session.
type TaskMapping struct {
	TaskID    string `json:"task_id"`
	OwnerUUID string `json:"owner_uuid"`
}

// Registry looks up task→session mappings from a NATS KV bucket.
type Registry struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// New creates a new registry client.
func New(js jetstream.JetStream, logger *slog.Logger) *Registry {
	return &Registry{
		js:     js,
		logger: logger.With("component", "registry"),
	}
}

// Lookup retrieves the task mapping for a session ID.
// Returns nil if no mapping exists.
func (r *Registry) Lookup(sessionID string) *TaskMapping {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := r.js.KeyValue(ctx, bucketName)
	if err != nil {
		r.logger.Debug("KV bucket not available", "error", err)
		return nil
	}

	entry, err := kv.Get(ctx, sessionID)
	if err != nil {
		// Key not found is normal for ad-hoc sessions.
		return nil
	}

	var mapping TaskMapping
	if err := json.Unmarshal(entry.Value(), &mapping); err != nil {
		r.logger.Warn("failed to unmarshal task mapping", "session_id", sessionID, "error", err)
		return nil
	}

	return &mapping
}
