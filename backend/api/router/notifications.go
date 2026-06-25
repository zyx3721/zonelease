package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	"zonelease/backend/internal/service/notify"
)

const emailNotificationChannelID = "email"

type notificationChannelRequest struct {
	Enabled              bool           `json:"enabled"`
	PasswordResetEnabled bool           `json:"passwordResetEnabled"`
	ClearConfig          bool           `json:"clearConfig"`
	Config               map[string]any `json:"config"`
}

type testNotificationRequest struct {
	To string `json:"to"`
}

func (r *Router) listNotificationChannels(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListNotificationChannels(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_notifications_failed", "读取通知配置失败")
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelListResponse{Items: redactNotificationChannels(items), Total: len(items)})
}

func (r *Router) notificationRoute(w http.ResponseWriter, req *http.Request) {
	id, action, ok := parseSettingsResourcePath(req.URL.Path, "/api/settings/notifications/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if id != emailNotificationChannelID {
		writeError(w, http.StatusNotFound, "notification_not_found", "通知媒介不存在")
		return
	}
	if req.Method == http.MethodPut && action == "" {
		r.updateNotificationChannel(w, req, id)
		return
	}
	if req.Method == http.MethodPost && action == "test" {
		r.testNotificationChannel(w, req, id)
		return
	}
	if req.Method == http.MethodPost && action == "preview" {
		r.previewNotificationChannel(w, req, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
}

func (r *Router) updateNotificationChannel(w http.ResponseWriter, req *http.Request, id string) {
	var body notificationChannelRequest
	if !decode(w, req, &body) {
		return
	}
	previous, _ := r.store.GetNotificationChannel(req.Context(), id)
	enabled := body.Enabled || body.PasswordResetEnabled
	config, err := sanitizeEmailNotificationConfig(body.Config, configMap(previous.Config), enabled, body.ClearConfig)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_notification_config", notify.UserFacingErrorMessage(err))
		return
	}
	item, err := r.store.UpsertNotificationChannel(req.Context(), id, body.Enabled, body.PasswordResetEnabled, config)
	if err != nil {
		r.logger.Error("Save notification channel failed", "error", err, "channel", id)
		writeError(w, http.StatusInternalServerError, "save_notification_failed", "保存通知配置失败")
		return
	}
	r.writeAudit(req, "settings.notification.update", id, "System", "success", map[string]any{
		"channel":              id,
		"enabled":              item.Enabled,
		"passwordResetEnabled": item.PasswordResetEnabled,
	})
	writeJSON(w, http.StatusOK, redactNotificationChannel(item))
}

func (r *Router) testNotificationChannel(w http.ResponseWriter, req *http.Request, id string) {
	var body testNotificationRequest
	if !decode(w, req, &body) {
		return
	}
	channel, err := r.store.GetNotificationChannel(req.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification_not_found", "通知媒介不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_notification_failed", "读取通知配置失败")
		return
	}
	if err := notify.New(r.store).SendTest(req.Context(), channel, body.To); err != nil {
		writeError(w, http.StatusServiceUnavailable, "notification_test_failed", notify.UserFacingErrorMessage(err))
		return
	}
	r.writeAudit(req, "settings.notification.test", id, "System", "success", map[string]any{"channel": id, "to": strings.TrimSpace(body.To)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) previewNotificationChannel(w http.ResponseWriter, req *http.Request, id string) {
	var body notificationChannelRequest
	if !decode(w, req, &body) {
		return
	}
	previous, _ := r.store.GetNotificationChannel(req.Context(), id)
	config, err := sanitizeEmailNotificationConfig(body.Config, configMap(previous.Config), false, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_notification_config", notify.UserFacingErrorMessage(err))
		return
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_notification_config", "通知配置格式不正确")
		return
	}
	preview, err := notify.PreviewTemplateConfig(id, configBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_notification_template", notify.UserFacingErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func sanitizeEmailNotificationConfig(config map[string]any, previous map[string]any, enabled bool, clearConfig bool) (map[string]any, error) {
	config = normalizeEmailConfigKeys(config)
	for key, value := range config {
		if text, ok := value.(string); ok {
			config[key] = strings.TrimSpace(text)
		}
	}
	retainPassword := boolValue(config["passwordConfigured"]) || boolValue(config[secretPresenceKey("password")])
	discardSecretPresenceMarkers(config, []string{"password"})
	delete(config, "passwordConfigured")
	if !clearConfig && (retainPassword || stringValue(config["password"]) == "") {
		if previousPassword := stringValue(previous["password"]); previousPassword != "" {
			config["password"] = previousPassword
		}
	}
	if value, ok := config["to"].(string); ok {
		config["to"] = splitEmailList(value)
	}
	if !enabled {
		delete(config, "to")
		return removeEmptyConfigValues(config), nil
	}
	if boolValue(config["useTLS"]) && boolValue(config["startTLS"]) {
		return nil, fmt.Errorf("TLS 与 STARTTLS 不能同时启用")
	}
	requiredFields := []struct {
		key   string
		label string
	}{
		{key: "smtpHost", label: "SMTP 主机"},
		{key: "username", label: "用户名"},
		{key: "password", label: "密码"},
		{key: "from", label: "发件人"},
	}
	for _, field := range requiredFields {
		if stringValue(config[field.key]) == "" {
			return nil, fmt.Errorf("%s不能为空", field.label)
		}
	}
	if !configValuePresent(config["smtpPort"]) {
		return nil, fmt.Errorf("SMTP 端口不能为空")
	}
	delete(config, "to")
	if boolValue(config["useTLS"]) {
		config["smtpPort"] = 465
		delete(config, "allowInsecureAuth")
	} else if boolValue(config["startTLS"]) {
		config["smtpPort"] = 587
		delete(config, "allowInsecureAuth")
	} else if !validPortValue(config["smtpPort"]) {
		return nil, fmt.Errorf("SMTP 端口需为 1 到 65535 之间的整数")
	}
	if contentType := stringValue(config["emailContentType"]); contentType != "" && contentType != "text/plain" && contentType != "text/html" {
		return nil, fmt.Errorf("邮件内容类型仅支持 text/plain 或 text/html")
	}
	configBytes, _ := json.Marshal(config)
	if err := notify.ValidateTemplateConfig(configBytes); err != nil {
		return nil, err
	}
	return removeEmptyConfigValues(config), nil
}

func normalizeEmailConfigKeys(config map[string]any) map[string]any {
	if config == nil {
		config = map[string]any{}
	}
	if value, ok := config["fromAddress"]; ok {
		config["from"] = value
		delete(config, "fromAddress")
	}
	if value, ok := config["useTls"]; ok {
		config["useTLS"] = value
		delete(config, "useTls")
	}
	if value, ok := config["startTls"]; ok {
		config["startTLS"] = value
		delete(config, "startTls")
	}
	if value, ok := config["testRecipient"]; ok {
		if len(stringList(config["to"])) == 0 && stringValue(config["to"]) == "" {
			config["to"] = []string{stringValue(value)}
		}
		delete(config, "testRecipient")
	}
	delete(config, "insecureSkipVerify")
	delete(config, "timeoutSeconds")
	return config
}

func configValuePresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case nil:
		return false
	default:
		return true
	}
}

func validPortValue(value any) bool {
	port := numberValue(value)
	return port >= 1 && port <= 65535 && port == float64(int(port))
}

func redactNotificationChannels(items []domain.NotificationChannel) []domain.NotificationChannel {
	redacted := make([]domain.NotificationChannel, len(items))
	for index, item := range items {
		redacted[index] = redactNotificationChannel(item)
	}
	return redacted
}

func redactNotificationChannel(item domain.NotificationChannel) domain.NotificationChannel {
	item.PasswordResetEnabled = item.ID == emailNotificationChannelID && item.PasswordResetEnabled
	item.Config = redactConfigSecrets(normalizeEmailConfigForResponse(configMap(item.Config)), []string{"password"})
	return item
}

func normalizeEmailConfigForResponse(config map[string]any) json.RawMessage {
	if config == nil {
		config = map[string]any{}
	}
	config = normalizeEmailConfigKeys(config)
	bytes, _ := json.Marshal(config)
	return bytes
}

func splitEmailList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
