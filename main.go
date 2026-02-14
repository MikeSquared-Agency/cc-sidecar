package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/MikeSquared-Agency/cc-sidecar/internal/publisher"
	"github.com/MikeSquared-Agency/cc-sidecar/internal/registry"
	"github.com/MikeSquared-Agency/cc-sidecar/internal/session"
	"github.com/MikeSquared-Agency/cc-sidecar/internal/watcher"

	"gopkg.in/yaml.v3"
)

// Config holds the sidecar configuration.
type Config struct {
	NATS struct {
		URL   string `yaml:"url"`
		Token string `yaml:"token"`
	} `yaml:"nats"`
	WatchDir      string        `yaml:"watch_dir"`
	IdleThreshold time.Duration `yaml:"idle_threshold"`
	PollInterval  time.Duration `yaml:"poll_interval"`
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := loadConfig(*configPath, logger)

	// Expand watch dir.
	watchDir := expandHome(cfg.WatchDir)

	// Connect to NATS and create publisher.
	pub, err := publisher.New(cfg.NATS.URL, cfg.NATS.Token, logger)
	if err != nil {
		logger.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer pub.Close()
	logger.Info("connected to NATS", "url", cfg.NATS.URL)

	// Create registry client for task_id lookups.
	reg := registry.New(pub.JetStream(), logger)

	// Create session tracker.
	tracker := session.NewTracker(cfg.IdleThreshold, cfg.PollInterval, logger, func(s *session.CompletedSession) {
		if s.ExitCode != 0 {
			if err := pub.PublishFailed(s, reg); err != nil {
				logger.Error("failed to publish session failed", "error", err, "session_id", s.SessionID)
			}
		} else {
			if err := pub.PublishCompleted(s, reg); err != nil {
				logger.Error("failed to publish session completed", "error", err, "session_id", s.SessionID)
			}
		}
	})

	// Create watcher.
	w, err := watcher.New(watchDir, tracker, logger)
	if err != nil {
		logger.Error("failed to create watcher", "error", err)
		os.Exit(1)
	}

	go w.Start()
	go tracker.Start()

	logger.Info("cc-sidecar started", "watch_dir", watchDir, "idle_threshold", cfg.IdleThreshold)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")
	w.Stop()
	tracker.Stop()
}

func loadConfig(path string, logger *slog.Logger) Config {
	cfg := Config{
		WatchDir:      "~/.claude/projects/",
		IdleThreshold: 10 * time.Second,
		PollInterval:  15 * time.Second,
	}
	cfg.NATS.URL = "nats://localhost:4222"

	// Load config file if provided.
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("could not read config file, using defaults", "path", path, "error", err)
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			logger.Warn("could not parse config file, using defaults", "path", path, "error", err)
		}
	}

	// Env overrides (highest precedence).
	if v := os.Getenv("CC_SIDECAR_NATS_URL"); v != "" {
		cfg.NATS.URL = v
	}
	if v := os.Getenv("CC_SIDECAR_NATS_TOKEN"); v != "" {
		cfg.NATS.Token = v
	}
	if v := os.Getenv("CC_SIDECAR_WATCH_DIR"); v != "" {
		cfg.WatchDir = v
	}
	if v := os.Getenv("CC_SIDECAR_IDLE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.IdleThreshold = d
		}
	}

	return cfg
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn("could not determine home directory, path may be malformed", "path", path, "error", err)
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
