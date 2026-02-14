package publisher

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/MikeSquared-Agency/cc-sidecar/internal/registry"
	"github.com/MikeSquared-Agency/cc-sidecar/internal/session"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	subjectCompleted = "swarm.cc.session.completed"
	subjectFailed    = "swarm.cc.session.failed"
)

// Event is the standardised Hermes envelope.
type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// SessionData is the payload for cc.session.completed/failed events.
type SessionData struct {
	SessionID      string   `json:"session_id"`
	TaskID         string   `json:"task_id"`
	OwnerUUID      string   `json:"owner_uuid"`
	AgentType      string   `json:"agent_type"`
	TranscriptPath string   `json:"transcript_path"`
	FilesChanged   []string `json:"files_changed"`
	ExitCode       int      `json:"exit_code"`
	DurationMs     int64    `json:"duration_ms"`
	WorkingDir     string   `json:"working_dir"`
	Timestamp      string   `json:"timestamp"`
}

// Publisher publishes CC session events to NATS.
type Publisher struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	logger *slog.Logger
}

// New creates a publisher and connects to NATS.
func New(url, token string, logger *slog.Logger) (*Publisher, error) {
	opts := []nats.Option{
		nats.Name("cc-sidecar"),
		nats.Timeout(5 * time.Second),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	return &Publisher{
		nc:     nc,
		js:     js,
		logger: logger.With("component", "publisher"),
	}, nil
}

// JetStream returns the underlying JetStream context for KV access.
func (p *Publisher) JetStream() jetstream.JetStream {
	return p.js
}

// PublishCompleted publishes a session completed event.
func (p *Publisher) PublishCompleted(s *session.CompletedSession, reg *registry.Registry) error {
	return p.publish(subjectCompleted, "cc.session.completed", s, reg)
}

// PublishFailed publishes a session failed event.
func (p *Publisher) PublishFailed(s *session.CompletedSession, reg *registry.Registry) error {
	return p.publish(subjectFailed, "cc.session.failed", s, reg)
}

func (p *Publisher) publish(subject, eventType string, s *session.CompletedSession, reg *registry.Registry) error {
	// Look up task mapping.
	var taskID, ownerUUID string
	if mapping := reg.Lookup(s.SessionID); mapping != nil {
		taskID = mapping.TaskID
		ownerUUID = mapping.OwnerUUID
	}

	data := SessionData{
		SessionID:      s.SessionID,
		TaskID:         taskID,
		OwnerUUID:      ownerUUID,
		AgentType:      "claude-code",
		TranscriptPath: s.TranscriptPath,
		FilesChanged:   s.FilesChanged,
		ExitCode:       s.ExitCode,
		DurationMs:     s.DurationMs,
		WorkingDir:     s.WorkingDir,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session data: %w", err)
	}

	ev := Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    "cc-sidecar",
		Timestamp: time.Now().UTC(),
		Data:      raw,
	}

	evBytes, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.nc.Publish(subject, evBytes); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	p.logger.Info("published session event", "subject", subject, "session_id", s.SessionID, "task_id", taskID)
	return nil
}

// Close drains and closes the NATS connection.
func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Drain()
	}
}
