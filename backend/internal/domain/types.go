package domain

import (
	"encoding/json"
	"strings"
	"time"
)

type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	Source      string     `json:"source"`
	Roles       []Role     `json:"roles,omitempty"`
	DirectRoles []Role     `json:"directRoles"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Permissions []string   `json:"permissions"`
}

type Role struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Disabled    bool      `json:"disabled"`
	Members     []User    `json:"members,omitempty"`
	Roles       []Role    `json:"roles,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Permission struct {
	Key                   string `json:"key"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Category              string `json:"category"`
	ImpliedReadPermission string `json:"impliedReadPermission,omitempty"`
}

type Session struct {
	Token      string    `json:"token"`
	Provider   string    `json:"provider"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	User       User      `json:"user"`
}

type RefreshTask struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	Payload    json.RawMessage `json:"payload"`
	CreatedBy  string          `json:"createdBy"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

type PasswordResetCaptcha struct {
	Token     string    `json:"token"`
	Question  string    `json:"question"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type PasswordResetChannel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MaskedTo   string `json:"maskedTo"`
	RequiresTo bool   `json:"requiresTo"`
}

type Server struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	Role         string `json:"role"`
	AgentURL     string `json:"agentUrl"`
	APIKey       string `json:"apiKey,omitempty"`
	TLSInsecure  bool   `json:"tlsInsecure"`
	Status       string `json:"status"`
	FailureCount int    `json:"failureCount,omitempty"`
	LastChecked  string `json:"lastChecked"`
}

type SystemBaseConfig struct {
	SiteName                         string  `json:"siteName"`
	LoginName                        string  `json:"loginName"`
	AppName                          string  `json:"appName"`
	AppSubtitle                      string  `json:"appSubtitle"`
	IconData                         string  `json:"iconData"`
	ResetCodeTTLMinutes              int     `json:"resetCodeTtlMinutes"`
	ResetCaptchaTTLMinutes           int     `json:"resetCaptchaTtlMinutes"`
	PasswordResetSendCooldownMinutes float64 `json:"passwordResetSendCooldownMinutes"`
	PasswordResetRateLimitMinutes    int     `json:"passwordResetRateLimitMinutes"`
	RuntimeSyncConcurrency           int     `json:"runtimeSyncConcurrency"`
	DNSRecordConcurrency             int     `json:"dnsRecordConcurrency"`
	DHCPScopeConcurrency             int     `json:"dhcpScopeConcurrency"`
	OperationRefreshDelaySeconds     int     `json:"operationRefreshDelaySeconds"`
	AgentOfflineFailureCount         int     `json:"agentOfflineFailureCount"`
	AgentConnectionTimeoutSeconds    int     `json:"agentConnectionTimeoutSeconds"`
	AgentOperationTimeoutSeconds     int     `json:"agentOperationTimeoutSeconds"`
	AgentFullSyncTimeoutSeconds      int     `json:"agentFullSyncTimeoutSeconds"`
	AgentHealthCheckIntervalMinutes  int     `json:"agentHealthCheckIntervalMinutes"`
	AgentHealthCheckConcurrency      int     `json:"agentHealthCheckConcurrency"`
}

func DefaultSystemBaseConfig() SystemBaseConfig {
	return SystemBaseConfig{
		SiteName:                         "ZoneLease",
		LoginName:                        "ZoneLease",
		AppName:                          "ZoneLease",
		AppSubtitle:                      "DNS / DHCP Control",
		IconData:                         "/favicon.svg",
		ResetCodeTTLMinutes:              10,
		ResetCaptchaTTLMinutes:           1,
		PasswordResetSendCooldownMinutes: 0.5,
		PasswordResetRateLimitMinutes:    5,
		RuntimeSyncConcurrency:           3,
		DNSRecordConcurrency:             3,
		DHCPScopeConcurrency:             5,
		OperationRefreshDelaySeconds:     10,
		AgentOfflineFailureCount:         3,
		AgentConnectionTimeoutSeconds:    5,
		AgentOperationTimeoutSeconds:     20,
		AgentFullSyncTimeoutSeconds:      300,
		AgentHealthCheckIntervalMinutes:  1,
		AgentHealthCheckConcurrency:      1,
	}
}

func NormalizeSystemBaseConfig(item SystemBaseConfig) SystemBaseConfig {
	defaults := DefaultSystemBaseConfig()
	item.SiteName = firstConfigText(item.SiteName, defaults.SiteName)
	item.LoginName = firstConfigText(item.LoginName, defaults.LoginName)
	item.AppName = firstConfigText(item.AppName, defaults.AppName)
	item.AppSubtitle = firstConfigText(item.AppSubtitle, defaults.AppSubtitle)
	item.IconData = firstConfigText(item.IconData, defaults.IconData)
	if item.ResetCodeTTLMinutes <= 0 {
		item.ResetCodeTTLMinutes = defaults.ResetCodeTTLMinutes
	}
	if item.ResetCaptchaTTLMinutes <= 0 {
		item.ResetCaptchaTTLMinutes = defaults.ResetCaptchaTTLMinutes
	}
	if item.PasswordResetSendCooldownMinutes <= 0 {
		item.PasswordResetSendCooldownMinutes = defaults.PasswordResetSendCooldownMinutes
	}
	if item.PasswordResetRateLimitMinutes <= 0 {
		item.PasswordResetRateLimitMinutes = defaults.PasswordResetRateLimitMinutes
	}
	if item.RuntimeSyncConcurrency <= 0 {
		item.RuntimeSyncConcurrency = defaults.RuntimeSyncConcurrency
	}
	if item.DNSRecordConcurrency <= 0 {
		item.DNSRecordConcurrency = defaults.DNSRecordConcurrency
	}
	if item.DHCPScopeConcurrency <= 0 {
		item.DHCPScopeConcurrency = defaults.DHCPScopeConcurrency
	}
	if item.OperationRefreshDelaySeconds <= 0 {
		item.OperationRefreshDelaySeconds = defaults.OperationRefreshDelaySeconds
	}
	if item.AgentOfflineFailureCount <= 0 {
		item.AgentOfflineFailureCount = defaults.AgentOfflineFailureCount
	}
	if item.AgentConnectionTimeoutSeconds <= 0 {
		item.AgentConnectionTimeoutSeconds = defaults.AgentConnectionTimeoutSeconds
	}
	if item.AgentOperationTimeoutSeconds <= 0 {
		item.AgentOperationTimeoutSeconds = defaults.AgentOperationTimeoutSeconds
	}
	if item.AgentFullSyncTimeoutSeconds <= 0 {
		item.AgentFullSyncTimeoutSeconds = defaults.AgentFullSyncTimeoutSeconds
	}
	if item.AgentHealthCheckIntervalMinutes <= 0 {
		item.AgentHealthCheckIntervalMinutes = defaults.AgentHealthCheckIntervalMinutes
	}
	if item.AgentHealthCheckConcurrency <= 0 {
		item.AgentHealthCheckConcurrency = defaults.AgentHealthCheckConcurrency
	}
	return item
}

func firstConfigText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type NotificationChannel struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name,omitempty"`
	Enabled              bool            `json:"enabled"`
	PasswordResetEnabled bool            `json:"passwordResetEnabled"`
	Config               json.RawMessage `json:"config" swaggertype:"object"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type Notification struct {
	ID          string          `json:"id"`
	Level       string          `json:"level"`
	Title       string          `json:"title"`
	Message     string          `json:"message"`
	SourceType  string          `json:"sourceType"`
	SourceID    string          `json:"sourceId"`
	Metadata    json.RawMessage `json:"metadata" swaggertype:"object"`
	ReadAt      *time.Time      `json:"readAt,omitempty"`
	DismissedAt *time.Time      `json:"dismissedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type AuthProvider struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config" swaggertype:"object"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type PublicAuthProvider struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type DNSZone struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Reverse       bool   `json:"reverse"`
	DynamicUpdate string `json:"dynamicUpdate"`
	ServerID      string `json:"serverId"`
	LastSyncedAt  string `json:"lastSyncedAt,omitempty"`
	SyncStatus    string `json:"syncStatus,omitempty"`
	LastError     string `json:"lastError,omitempty"`
}

type DNSRecord struct {
	ID           string `json:"id"`
	ZoneID       string `json:"zoneId"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Value        string `json:"value"`
	TTL          int    `json:"ttl"`
	CreatePTR    bool   `json:"createPtr,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
}

type DNSRecordCreateResponse struct {
	DNSRecord
	RelatedRecords []DNSRecord `json:"relatedRecords,omitempty"`
	Warnings       []string    `json:"warnings,omitempty"`
}

type DHCPScope struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Subnet               string `json:"subnet"`
	DefaultGateway       string `json:"defaultGateway,omitempty"`
	StartRange           string `json:"startRange"`
	EndRange             string `json:"endRange"`
	LeaseDurationHours   int    `json:"leaseDurationHours"`
	LeaseDurationSeconds int    `json:"leaseDurationSeconds"`
	State                string `json:"state"`
	ServerID             string `json:"serverId"`
	ExternalID           string `json:"externalId,omitempty"`
	LastSyncedAt         string `json:"lastSyncedAt,omitempty"`
	SyncStatus           string `json:"syncStatus,omitempty"`
	LastError            string `json:"lastError,omitempty"`
}

type DHCPExclusion struct {
	ID           string `json:"id"`
	ScopeID      string `json:"scopeId"`
	StartIP      string `json:"startIp"`
	EndIP        string `json:"endIp"`
	ExternalID   string `json:"externalId,omitempty"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
}

type DHCPLease struct {
	ID           string `json:"id"`
	ScopeID      string `json:"scopeId"`
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	Hostname     string `json:"hostname"`
	State        string `json:"state"`
	ExpiresAt    string `json:"expiresAt"`
	ExternalID   string `json:"externalId,omitempty"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
}

type DHCPReservation struct {
	ID           string `json:"id"`
	ScopeID      string `json:"scopeId"`
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	ExternalID   string `json:"externalId,omitempty"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
}

type AuditEntry struct {
	ID        string `json:"id"`
	TS        string `json:"ts"`
	User      string `json:"user"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Module    string `json:"module"`
	Result    string `json:"result"`
	IPAddress string `json:"ipAddress,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type State struct {
	Servers      []Server          `json:"servers"`
	Zones        []DNSZone         `json:"zones"`
	Records      []DNSRecord       `json:"records"`
	Scopes       []DHCPScope       `json:"scopes"`
	Exclusions   []DHCPExclusion   `json:"exclusions"`
	Leases       []DHCPLease       `json:"leases"`
	Reservations []DHCPReservation `json:"reservations"`
	Audit        []AuditEntry      `json:"audit"`
}
