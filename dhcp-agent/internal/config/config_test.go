package config

import "testing"

func TestLoadReadsDHCPLogPath(t *testing.T) {
	t.Setenv("DHCP_AGENT_LOG_PATH", "custom-dhcp-agent.log")
	cfg := Load()
	if cfg.LogPath != "custom-dhcp-agent.log" {
		t.Fatalf("unexpected log path: %s", cfg.LogPath)
	}
}

func TestLoadDoesNotReadDNSLogPath(t *testing.T) {
	t.Setenv("DNS_AGENT_LOG_PATH", "dns-agent.log")
	cfg := Load()
	if cfg.LogPath != "agent.log" {
		t.Fatalf("dhcp agent should not read dns log path fallback: %s", cfg.LogPath)
	}
}

func TestLoadReadsDHCPPowerShellTimeout(t *testing.T) {
	t.Setenv("DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS", "240")
	cfg := Load()
	if cfg.PowerShellTimeoutSeconds != 240 {
		t.Fatalf("unexpected powershell timeout: %d", cfg.PowerShellTimeoutSeconds)
	}
}

func TestLoadDoesNotReadDNSPowerShellTimeout(t *testing.T) {
	t.Setenv("DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS", "240")
	cfg := Load()
	if cfg.PowerShellTimeoutSeconds != 180 {
		t.Fatalf("dhcp agent should not read dns timeout fallback: %d", cfg.PowerShellTimeoutSeconds)
	}
}

func TestLoadFallsBackForInvalidDHCPPowerShellTimeout(t *testing.T) {
	t.Setenv("DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS", "9999")
	cfg := Load()
	if cfg.PowerShellTimeoutSeconds != 180 {
		t.Fatalf("invalid powershell timeout should fallback: %d", cfg.PowerShellTimeoutSeconds)
	}
}
