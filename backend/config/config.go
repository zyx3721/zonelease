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
	defaultSessionHours = 12
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
	SessionSecret         string
	SessionExpireHours    int
	LoginMaxFailures      int
	LoginLockoutMinutes   int
	ResetCodeTTL          time.Duration
	ResetCaptchaTTL       time.Duration
	ResetVerificationTTL  time.Duration
	ResetSendCooldownSecs int
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
	return LoadWithOverrides(logger, nil)
}

// LoadWithOverrides 加载配置，overrides 中的非空值优先于对应环境变量，供命令行参数显式覆盖使用
func LoadWithOverrides(logger *slog.Logger, overrides map[string]string) (Config, error) {
	if overrides == nil {
		overrides = map[string]string{}
	}
	cfg := Config{
		Server: ServerConfig{
			Host: lookup(overrides, "server_host", "SERVER_HOST", "127.0.0.1"),
			Port: lookup(overrides, "server_port", "SERVER_PORT", "8080"),
			Mode: lookup(overrides, "server_mode", "SERVER_MODE", "release"),
		},
		Database: DatabaseConfig{
			Host:     lookup(overrides, "db_host", "DB_HOST", "localhost"),
			Port:     lookup(overrides, "db_port", "DB_PORT", "5432"),
			Name:     lookup(overrides, "db_name", "DB_NAME", "zonelease"),
			User:     lookup(overrides, "db_user", "DB_USER", "zonelease"),
			Password: lookup(overrides, "db_password", "DB_PASSWORD", "zonelease_dev"),
			SSLMode:  lookup(overrides, "db_sslmode", "DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     lookup(overrides, "redis_addr", "REDIS_ADDR", "localhost:6379"),
			Password: lookupRaw(overrides, "redis_password", "REDIS_PASSWORD"),
			DB:       lookupIntAllowZero(overrides, "redis_db", "REDIS_DB", 0),
		},
		Auth: AuthConfig{
			SessionSecret:         lookupRaw(overrides, "jwt_secret", "JWT_SECRET"),
			SessionExpireHours:    lookupInt(overrides, "jwt_expire_hours", "JWT_EXPIRE_HOURS", defaultSessionHours),
			LoginMaxFailures:      lookupInt(overrides, "login_max_failures", "LOGIN_MAX_FAILURES", 5),
			LoginLockoutMinutes:   lookupInt(overrides, "login_lockout_minutes", "LOGIN_LOCKOUT_MINUTES", 2),
			ResetCodeTTL:          10 * time.Minute,
			ResetCaptchaTTL:       time.Minute,
			ResetVerificationTTL:  10 * time.Minute,
			ResetSendCooldownSecs: 30,
		},
		Runtime: RuntimeConfig{
			RefreshTTL:           2 * time.Minute,
			DNSDeepSyncInterval:  lookupScheduleInterval(overrides, "runtime_dns_deep_sync_interval", "RUNTIME_DNS_DEEP_SYNC_INTERVAL", 24*time.Hour),
			DHCPDeepSyncInterval: lookupScheduleInterval(overrides, "runtime_dhcp_deep_sync_interval", "RUNTIME_DHCP_DEEP_SYNC_INTERVAL", time.Hour),
			MetricRetentionDays:  lookupInt(overrides, "metric_retention_days", "METRIC_RETENTION_DAYS", 30),
			LogRetentionDays:     lookupInt(overrides, "log_retention_days", "LOG_RETENTION_DAYS", 30),
			MetricStreamMaxLen:   int64(lookupInt(overrides, "metric_stream_maxlen", "METRIC_STREAM_MAXLEN", 10000)),
		},
		CORS: CORSConfig{
			Origin: lookup(overrides, "cors_origin", "CORS_ORIGIN", "http://localhost:5173"),
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

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// lookup 按命令行覆盖值、环境变量、默认值的顺序解析字符串配置
func lookup(overrides map[string]string, key, envName, fallback string) string {
	if value := strings.TrimSpace(overrides[key]); value != "" {
		return value
	}
	return env(envName, fallback)
}

// lookupRaw 解析不做空白修剪的配置值，环境变量路径保持原样读取，适用于密码与密钥
func lookupRaw(overrides map[string]string, key, envName string) string {
	if value := strings.TrimSpace(overrides[key]); value != "" {
		return value
	}
	return os.Getenv(envName)
}

// lookupInt 按命令行覆盖值、环境变量、默认值的顺序解析正整数配置
func lookupInt(overrides map[string]string, key, envName string, fallback int) int {
	if value := strings.TrimSpace(overrides[key]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fallback
		}
		return parsed
	}
	return envInt(envName, fallback)
}

// lookupIntAllowZero 按命令行覆盖值、环境变量、默认值的顺序解析非负整数配置
func lookupIntAllowZero(overrides map[string]string, key, envName string, fallback int) int {
	if value := strings.TrimSpace(overrides[key]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return fallback
		}
		return parsed
	}
	return envIntAllowZero(envName, fallback)
}

// lookupScheduleInterval 按命令行覆盖值、环境变量、默认值的顺序解析调度间隔配置
func lookupScheduleInterval(overrides map[string]string, key, envName string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(overrides[key]); value != "" {
		parsed, err := parseScheduleDuration(value)
		if err != nil || parsed < 0 {
			return fallback
		}
		return parsed
	}
	return envScheduleInterval(envName, fallback)
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
