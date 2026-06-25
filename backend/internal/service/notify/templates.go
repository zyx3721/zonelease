package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	defaultProblemTemplate         = "[{{alert.levelText}}] {{alert.title}}\n{{alert.message}}\n来源：{{alert.sourceType}}/{{alert.sourceId}}"
	defaultRecoveryTemplate        = "[恢复] {{alert.title}}\n{{alert.message}}\n来源：{{alert.sourceType}}/{{alert.sourceId}}\n恢复时间：{{alert.resolvedAt}}"
	defaultProblemSubjectTemplate  = "{{alert.title}}"
	defaultRecoverySubjectTemplate = "恢复：{{alert.title}}"
)

var templateTokenPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

type TemplatePreview struct {
	ProblemSubject  string `json:"problemSubject"`
	ProblemText     string `json:"problemText"`
	RecoverySubject string `json:"recoverySubject"`
	RecoveryText    string `json:"recoveryText"`
	ContentType     string `json:"contentType,omitempty"`
}

type notificationEvent struct {
	Type  string
	Alert alertPayload
}

type alertPayload struct {
	ID          string
	Level       string
	Status      string
	SourceType  string
	SourceID    string
	Title       string
	Message     string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	ResolvedAt  *time.Time
	Metadata    map[string]any
}

type templateConfig struct {
	ProblemTemplate         string `json:"problemTemplate"`
	RecoveryTemplate        string `json:"recoveryTemplate"`
	ProblemSubjectTemplate  string `json:"problemSubjectTemplate"`
	RecoverySubjectTemplate string `json:"recoverySubjectTemplate"`
	EmailContentType        string `json:"emailContentType"`
	SendRecovery            bool   `json:"sendRecovery"`
}

func ValidateTemplateConfig(data []byte) error {
	_, err := PreviewTemplateConfig("email", data)
	return err
}

func PreviewTemplateConfig(channelID string, data []byte) (TemplatePreview, error) {
	if channelID != "email" {
		return TemplatePreview{}, fmt.Errorf("不支持的通知媒介 %s", channelID)
	}
	cfg := notificationTemplateConfig(data)
	problemAlert, recoveryAlert := templatePreviewAlerts()
	problem := notificationEvent{Type: "problem", Alert: problemAlert}
	recovery := notificationEvent{Type: "recovery", Alert: recoveryAlert}
	return TemplatePreview{
		ProblemSubject:  alertSubject(problem, cfg),
		ProblemText:     alertText(problem, cfg),
		RecoverySubject: alertSubject(recovery, cfg),
		RecoveryText:    alertText(recovery, cfg),
		ContentType:     emailContentType(cfg),
	}, nil
}

func notificationTemplateConfig(data []byte) templateConfig {
	var cfg templateConfig
	if len(data) > 0 {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

func templatePreviewAlerts() (alertPayload, alertPayload) {
	firstSeen := time.Date(2026, 6, 6, 9, 30, 0, 0, time.Local)
	lastSeen := time.Date(2026, 6, 6, 9, 45, 0, 0, time.Local)
	resolvedAt := time.Date(2026, 6, 6, 10, 5, 0, 0, time.Local)
	alert := alertPayload{
		ID: "preview-alert", Level: "warning", Status: "active", SourceType: "system", SourceID: "zonelease",
		Title: "ZoneLease 测试通知", Message: "这是一条邮件通知模板测试消息",
		Metadata: map[string]any{"service": "password-reset", "metric": "smtp"},
		FirstSeenAt: firstSeen, LastSeenAt: lastSeen,
	}
	recovery := alert
	recovery.Status = "resolved"
	recovery.ResolvedAt = &resolvedAt
	return alert, recovery
}

func alertText(event notificationEvent, cfg templateConfig) string {
	template := strings.TrimSpace(cfg.ProblemTemplate)
	if event.Type == "recovery" {
		template = strings.TrimSpace(cfg.RecoveryTemplate)
	}
	if template == "" {
		template = defaultProblemTemplate
		if event.Type == "recovery" {
			template = defaultRecoveryTemplate
		}
	}
	return renderAlertTemplate(template, event)
}

func alertSubject(event notificationEvent, cfg templateConfig) string {
	template := strings.TrimSpace(cfg.ProblemSubjectTemplate)
	if event.Type == "recovery" {
		template = strings.TrimSpace(cfg.RecoverySubjectTemplate)
	}
	if template == "" {
		template = defaultProblemSubjectTemplate
		if event.Type == "recovery" {
			template = defaultRecoverySubjectTemplate
		}
	}
	return renderAlertTemplate(template, event)
}

func renderAlertTemplate(template string, event notificationEvent) string {
	values := alertTemplateValues(event)
	return templateTokenPattern.ReplaceAllStringFunc(template, func(token string) string {
		matches := templateTokenPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return token
		}
		return values[matches[1]]
	})
}

func alertTemplateValues(event notificationEvent) map[string]string {
	alert := event.Alert
	values := map[string]string{
		"event.type":        event.Type,
		"event.statusText":  eventStatusText(event.Type),
		"alert.id":          alert.ID,
		"alert.level":       alert.Level,
		"alert.levelText":   alertLevelText(alert.Level),
		"alert.status":      alert.Status,
		"alert.title":       alert.Title,
		"alert.message":     alert.Message,
		"alert.sourceType":  alert.SourceType,
		"alert.sourceId":    alert.SourceID,
		"alert.firstSeenAt": formatTemplateTime(alert.FirstSeenAt),
		"alert.lastSeenAt":  formatTemplateTime(alert.LastSeenAt),
		"alert.resolvedAt":  formatOptionalTemplateTime(alert.ResolvedAt),
	}
	for key, value := range alert.Metadata {
		values["metadata."+key] = stringifyTemplateValue(value)
	}
	return values
}

func emailContentType(cfg templateConfig) string {
	if strings.EqualFold(strings.TrimSpace(cfg.EmailContentType), "text/html") {
		return "text/html"
	}
	return "text/plain"
}

func eventStatusText(eventType string) string {
	if eventType == "recovery" {
		return "恢复"
	}
	return "告警"
}

func alertLevelText(level string) string {
	switch level {
	case "critical":
		return "严重"
	case "warning":
		return "警告"
	case "info":
		return "信息"
	default:
		return level
	}
}

func formatTemplateTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func formatOptionalTemplateTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTemplateTime(*value)
}

func stringifyTemplateValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(typed); err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return strings.TrimSpace(buffer.String())
	}
}
