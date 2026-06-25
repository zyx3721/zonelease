package config

import (
	"log/slog"
	"testing"
)

func TestLoadLogRetentionDays(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("LOG_RETENTION_DAYS", "45")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.LogRetentionDays != 45 {
		t.Fatalf("expected log retention days 45, got %d", cfg.Runtime.LogRetentionDays)
	}
}

func TestLoadLogRetentionDaysDefaultsToThirty(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.LogRetentionDays != 30 {
		t.Fatalf("expected default log retention days 30, got %d", cfg.Runtime.LogRetentionDays)
	}
}
