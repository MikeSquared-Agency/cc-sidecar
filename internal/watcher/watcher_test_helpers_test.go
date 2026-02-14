package watcher

import (
	"log/slog"
	"os"
)

// touchRecorder is a test double for Toucher that records Touch calls.
type touchRecorder struct {
	touches *[]string
}

func (r *touchRecorder) Touch(path string) {
	*r.touches = append(*r.touches, path)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
