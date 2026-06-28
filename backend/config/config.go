package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSessionHours     = 24
	defaultSessionIdleHours = 12
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Auth     AuthConfig
	Runtime  RuntimeConfig
	CORS     CORSConfig
}

type ServerConfig struct {
	Host string
	Port string
	Mode string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type AuthConfig struct {
	SessionSecret          string
	SessionExpireHours     int
	SessionIdleExpireHours int
	ResetCodeTTL           time.Duration
	ResetCaptchaTTL        time.Duration
	ResetVerificationTTL   time.Duration
	ResetSendCooldownSecs  int
}

type RuntimeConfig struct {
	RefreshTTL           time.Duration
	DNSDeepSyncInterval  time.Duration
	DHCPDeepSyncInterval time.Duration
	MetricRetentionDays  int
	LogRetentionDays     int
	MetricStreamMaxLen   int64
}

type CORSConfig struct {
	Origin string
}

func Load(logger *slog.Logger) (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Host: env("SERVER_HOST", "127.0.0.1"),
			Port: env("SERVER_PORT", "8080"),
			Mode: env("SERVER_MODE", "release"),
		},
		Database: DatabaseConfig{
			Host:     env("DB_HOST", "localhost"),
			Port:     env("DB_PORT", "5432"),
			Name:     env("DB_NAME", "zonelease"),
			User:     env("DB_USER", "zonelease"),
			Password: env("DB_PASSWORD", "zonelease_dev"),
			SSLMode:  env("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       envIntAllowZero("REDIS_DB", 0),
		},
		Auth: AuthConfig{
			SessionSecret:          os.Getenv("JWT_SECRET"),
			SessionExpireHours:     envInt("JWT_EXPIRE_HOURS", defaultSessionHours),
			SessionIdleExpireHours: envInt("SESSION_IDLE_TIMEOUT_HOURS", defaultSessionIdleHours),
			ResetCodeTTL:           10 * time.Minute,
			ResetCaptchaTTL:        time.Minute,
			ResetVerificationTTL:   10 * time.Minute,
			ResetSendCooldownSecs:  30,
		},
		Runtime: RuntimeConfig{
			RefreshTTL:           2 * time.Minute,
			DNSDeepSyncInterval:  envScheduleInterval("RUNTIME_DNS_DEEP_SYNC_INTERVAL", 24*time.Hour),
			DHCPDeepSyncInterval: envScheduleInterval("RUNTIME_DHCP_DEEP_SYNC_INTERVAL", time.Hour),
			MetricRetentionDays:  envInt("METRIC_RETENTION_DAYS", 30),
			LogRetentionDays:     envInt("LOG_RETENTION_DAYS", 30),
			MetricStreamMaxLen:   int64(envInt("METRIC_STREAM_MAXLEN", 10000)),
		},
		CORS: CORSConfig{
			Origin: env("CORS_ORIGIN", "http://localhost:5173"),
		},
	}

	if err := cfg.Database.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Auth.SessionSecret == "" {
		secret, err := randomSecret(32)
		if err != nil {
			return Config{}, fmt.Errorf("generate session secret: %w", err)
		}
		cfg.Auth.SessionSecret = secret
		logger.Warn("JWT_SECRET is not set; generated a temporary secret for this process")
	}
	return cfg, nil
}

func (s ServerConfig) Addr() string {
	host := strings.TrimSpace(s.Host)
	port := strings.TrimSpace(s.Port)
	if port == "" {
		port = "8080"
	}
	if host == "" || host == "0.0.0.0" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

func (d DatabaseConfig) Validate() error {
	if strings.TrimSpace(d.Host) == "" {
		return fmt.Errorf("DB_HOST cannot be empty")
	}
	if strings.TrimSpace(d.Port) == "" {
		return fmt.Errorf("DB_PORT cannot be empty")
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("DB_NAME cannot be empty")
	}
	if strings.TrimSpace(d.User) == "" {
		return fmt.Errorf("DB_USER cannot be empty")
	}
	return nil
}

func (d DatabaseConfig) DSN() string {
	values := url.Values{}
	values.Set("sslmode", d.SSLMode)
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.User, d.Password),
		Host:     net.JoinHostPort(d.Host, d.Port),
		Path:     d.Name,
		RawQuery: values.Encode(),
	}).String()
}

func (a AuthConfig) SessionTTL() time.Duration {
	return time.Duration(a.SessionExpireHours) * time.Hour
}

func (a AuthConfig) SessionIdleTTL() time.Duration {
	return time.Duration(a.SessionIdleExpireHours) * time.Hour
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envIntAllowZero(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDurationAllowZero(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if value == "0" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envScheduleInterval(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if value == "0" {
		return 0
	}
	parsed, err := parseScheduleDuration(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func parseScheduleDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, fmt.Errorf("duration cannot be empty")
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(value, "d")))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func randomSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
