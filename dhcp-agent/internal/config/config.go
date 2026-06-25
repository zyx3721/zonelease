package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host                     string
	Port                     int
	APIKey                   string
	AllowAnonymous           bool
	LogPath                  string
	PowerShellTimeoutSeconds int
}

func Load() Config {
	return Config{
		Host:                     env("DHCP_AGENT_HOST", "0.0.0.0"),
		Port:                     envInt("DHCP_AGENT_PORT", 8462),
		APIKey:                   env("DHCP_AGENT_API_KEY", ""),
		AllowAnonymous:           envBool("DHCP_AGENT_ALLOW_ANONYMOUS", false),
		LogPath:                  env("DHCP_AGENT_LOG_PATH", "agent.log"),
		PowerShellTimeoutSeconds: envIntRange("DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS", 180, 1, 3600),
	}
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	parsed, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntRange(key string, fallback, min, max int) int {
	value := envInt(key, fallback)
	if value < min || value > max {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(env(key, ""))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}
