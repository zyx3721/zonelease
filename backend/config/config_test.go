package config

import (
	"log/slog"
	"testing"
	"time"
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

func TestLoadRuntimeDeepSyncIntervals(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("RUNTIME_DNS_DEEP_SYNC_INTERVAL", "2d")
	t.Setenv("RUNTIME_DHCP_DEEP_SYNC_INTERVAL", "30m")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.DNSDeepSyncInterval != 48*time.Hour {
		t.Fatalf("expected DNS interval 48h, got %s", cfg.Runtime.DNSDeepSyncInterval)
	}
	if cfg.Runtime.DHCPDeepSyncInterval != 30*time.Minute {
		t.Fatalf("expected DHCP interval 30m, got %s", cfg.Runtime.DHCPDeepSyncInterval)
	}
}

func TestLoadRuntimeDeepSyncIntervalDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.DNSDeepSyncInterval != 24*time.Hour {
		t.Fatalf("expected default DNS interval 24h, got %s", cfg.Runtime.DNSDeepSyncInterval)
	}
	if cfg.Runtime.DHCPDeepSyncInterval != time.Hour {
		t.Fatalf("expected default DHCP interval 1h, got %s", cfg.Runtime.DHCPDeepSyncInterval)
	}
}

func TestLoadRuntimeDeepSyncIntervalAllowsZero(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("RUNTIME_DNS_DEEP_SYNC_INTERVAL", "0")
	t.Setenv("RUNTIME_DHCP_DEEP_SYNC_INTERVAL", "0")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.DNSDeepSyncInterval != 0 {
		t.Fatalf("expected disabled DNS interval, got %s", cfg.Runtime.DNSDeepSyncInterval)
	}
	if cfg.Runtime.DHCPDeepSyncInterval != 0 {
		t.Fatalf("expected disabled DHCP interval, got %s", cfg.Runtime.DHCPDeepSyncInterval)
	}
}
