package router

import (
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/service/notify"
)

type statusResponse struct {
	Status string `json:"status"`
}

type healthResponse struct {
	Status   string                           `json:"status"`
	Time     string                           `json:"time"`
	Services map[string]healthServiceResponse `json:"services"`
}

type healthServiceResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type serverHealthResponse struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type systemBaseConfigResponse struct {
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

type notificationChannelResponse = domain.NotificationChannel

type notificationChannelListResponse struct {
	Items []notificationChannelResponse `json:"items"`
	Total int                           `json:"total"`
}

type notificationResponse = domain.Notification

type notificationListResponse struct {
	Items []notificationResponse `json:"items"`
	Total int                    `json:"total"`
}

type settingsUserResponse = domain.User

type settingsUserListResponse struct {
	Items []settingsUserResponse `json:"items"`
	Roles []map[string]string    `json:"roles"`
}

type settingsUserDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

type roleResponse = domain.Role

type roleListResponse struct {
	Items []roleResponse `json:"items"`
	Total int            `json:"total"`
}

type userGroupResponse = domain.UserGroup

type userGroupListResponse struct {
	Items []userGroupResponse `json:"items"`
	Total int                 `json:"total"`
}

type permissionResponse = domain.Permission

type permissionListResponse struct {
	Items []permissionResponse `json:"items"`
	Total int                  `json:"total"`
}

type authProviderResponse = domain.AuthProvider

type authProviderListResponse struct {
	Items []authProviderResponse `json:"items"`
}

type publicAuthProviderResponse = domain.PublicAuthProvider

type publicAuthProviderListResponse struct {
	Items []publicAuthProviderResponse `json:"items"`
	Total int                          `json:"total"`
}

type authProviderTestResponse struct {
	Status       string `json:"status"`
	MatchedUsers int    `json:"matchedUsers"`
}

type templatePreviewResponse = notify.TemplatePreview

type stateResponse = domain.State

type loginResponse = domain.Session

type meResponse = domain.User

type captchaResponse = domain.PasswordResetCaptcha

type verifyResetResponse = resetVerifyResponse

type sendResetResponse = resetSendResponse

type refreshTaskResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Status     string         `json:"status"`
	Payload    map[string]any `json:"payload" swaggertype:"object"`
	CreatedBy  string         `json:"createdBy"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
	FinishedAt string         `json:"finishedAt,omitempty"`
}

type refreshTaskListResponse struct {
	Items []refreshTaskResponse `json:"items"`
}

type serverResponse = domain.Server

type dnsZoneResponse = domain.DNSZone
type dnsZoneCreateResponseDoc = dnsZoneCreateResponse

type dnsRecordResponse = domain.DNSRecord
type dnsRecordCreateResponse = domain.DNSRecordCreateResponse

type dhcpScopeResponse = domain.DHCPScope

type dhcpExclusionResponse = domain.DHCPExclusion

type dhcpReservationResponse = domain.DHCPReservation

type dhcpReservationUpdateRequest = agentReservationUpdatePayload

type errorDocResponse = errorResponse
