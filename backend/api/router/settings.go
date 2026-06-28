package router

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"zonelease/backend/internal/domain"
)

func (r *Router) getSystemBaseConfig(w http.ResponseWriter, req *http.Request) {
	config, err := r.store.GetSystemBaseConfig(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_failed", "读取基础配置失败")
		return
	}
	writeJSON(w, http.StatusOK, r.withRuntimeDefaults(config))
}

func (r *Router) publicSystemBaseConfig(w http.ResponseWriter, req *http.Request) {
	config, err := r.store.GetSystemBaseConfig(req.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, r.withRuntimeDefaults(domain.SystemBaseConfig{}))
		return
	}
	writeJSON(w, http.StatusOK, r.withRuntimeDefaults(config))
}

func (r *Router) updateSystemBaseConfig(w http.ResponseWriter, req *http.Request) {
	var body domain.SystemBaseConfig
	if !decode(w, req, &body) {
		return
	}
	var ok bool
	body, ok = r.normalizeBaseConfig(w, body)
	if !ok {
		return
	}
	saved, err := r.store.UpdateSystemBaseConfig(req.Context(), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_failed", "保存基础配置失败")
		return
	}
	r.writeAudit(req, "Updated system base config", "base", "System", "success", map[string]any{
		"siteName":  saved.SiteName,
		"loginName": saved.LoginName,
		"appName":   saved.AppName,
	})
	writeJSON(w, http.StatusOK, saved)
}

func (r *Router) normalizeBaseConfig(w http.ResponseWriter, item domain.SystemBaseConfig) (domain.SystemBaseConfig, bool) {
	item = domain.NormalizeSystemBaseConfig(item)
	if err := validateBaseConfig(item); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_base_config", err.Error())
		return domain.SystemBaseConfig{}, false
	}
	return item, true
}

func (r *Router) withRuntimeDefaults(item domain.SystemBaseConfig) domain.SystemBaseConfig {
	return domain.NormalizeSystemBaseConfig(item)
}

func validateBaseConfig(item domain.SystemBaseConfig) error {
	if len([]rune(item.SiteName)) > 60 || len([]rune(item.LoginName)) > 60 || len([]rune(item.AppName)) > 60 || len([]rune(item.AppSubtitle)) > 60 {
		return fmt.Errorf("名称长度不能超过 60 个字符")
	}
	if item.IconData == "" || (!strings.HasPrefix(item.IconData, "/") && !strings.HasPrefix(item.IconData, "data:image/")) {
		return fmt.Errorf("图标必须是站内路径或图片 Data URL")
	}
	if item.ResetCodeTTLMinutes < 1 || item.ResetCodeTTLMinutes > 60 {
		return fmt.Errorf("找回密码验证码有效期需在 1 到 60 分钟之间")
	}
	if item.ResetCaptchaTTLMinutes < 1 || item.ResetCaptchaTTLMinutes > 10 {
		return fmt.Errorf("图形验证码有效期需在 1 到 10 分钟之间")
	}
	if item.PasswordResetSendCooldownMinutes < 0.5 || item.PasswordResetSendCooldownMinutes > 10 {
		return fmt.Errorf("找回密码发送冷却需在 0.5 到 10 分钟之间")
	}
	if !isHalfMinuteStep(item.PasswordResetSendCooldownMinutes) {
		return fmt.Errorf("找回密码发送冷却需按 0.5 分钟递增")
	}
	if item.PasswordResetRateLimitMinutes < 5 || item.PasswordResetRateLimitMinutes > 10 {
		return fmt.Errorf("找回密码频率窗口需在 5 到 10 分钟之间")
	}
	if item.RuntimeSyncConcurrency < 1 || item.RuntimeSyncConcurrency > 20 {
		return fmt.Errorf("全量同步并发需在 1 到 20 个之间")
	}
	if item.DNSRecordConcurrency < 1 || item.DNSRecordConcurrency > 50 {
		return fmt.Errorf("DNS 区域并发需在 1 到 50 个之间")
	}
	if item.DHCPScopeConcurrency < 1 || item.DHCPScopeConcurrency > 50 {
		return fmt.Errorf("DHCP 作用域并发需在 1 到 50 个之间")
	}
	if item.OperationRefreshDelaySeconds < 1 || item.OperationRefreshDelaySeconds > 60 {
		return fmt.Errorf("操作后刷新等待需在 1 到 60 秒之间")
	}
	if item.AgentOfflineFailureCount < 1 || item.AgentOfflineFailureCount > 20 {
		return fmt.Errorf("Agent 离线失败次数需在 1 到 20 次之间")
	}
	if item.AgentConnectionTimeoutSeconds < 1 || item.AgentConnectionTimeoutSeconds > 20 {
		return fmt.Errorf("Agent 连接超时时间需在 1 到 20 秒之间")
	}
	if item.AgentOperationTimeoutSeconds < 1 || item.AgentOperationTimeoutSeconds > 60 {
		return fmt.Errorf("Agent 操作超时时间需在 1 到 60 秒之间")
	}
	if item.AgentFullSyncTimeoutSeconds < 60 || item.AgentFullSyncTimeoutSeconds > 600 {
		return fmt.Errorf("Agent 全量同步超时时间需在 60 到 600 秒之间")
	}
	if item.AgentHealthCheckIntervalMinutes < 1 || item.AgentHealthCheckIntervalMinutes > 60 {
		return fmt.Errorf("Agent 连通性检查间隔需在 1 到 60 分钟之间")
	}
	if item.AgentHealthCheckConcurrency < 1 || item.AgentHealthCheckConcurrency > 20 {
		return fmt.Errorf("Agent 自动检查并发需在 1 到 20 个之间")
	}
	return nil
}

func isHalfMinuteStep(value float64) bool {
	return math.Abs(value*2-math.Round(value*2)) < 0.000001
}

func (r *Router) agentOfflineFailureLimit(ctx context.Context) int {
	config, err := r.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return domain.DefaultSystemBaseConfig().AgentOfflineFailureCount
	}
	return domain.NormalizeSystemBaseConfig(config).AgentOfflineFailureCount
}

func (r *Router) agentConnectionTimeout(ctx context.Context) time.Duration {
	config, err := r.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return time.Duration(domain.DefaultSystemBaseConfig().AgentConnectionTimeoutSeconds) * time.Second
	}
	return time.Duration(domain.NormalizeSystemBaseConfig(config).AgentConnectionTimeoutSeconds) * time.Second
}

func (r *Router) agentOperationTimeout(ctx context.Context) time.Duration {
	config, err := r.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return time.Duration(domain.DefaultSystemBaseConfig().AgentOperationTimeoutSeconds) * time.Second
	}
	return time.Duration(domain.NormalizeSystemBaseConfig(config).AgentOperationTimeoutSeconds) * time.Second
}

func (r *Router) operationRefreshDelay(ctx context.Context) time.Duration {
	config, err := r.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return time.Duration(domain.DefaultSystemBaseConfig().OperationRefreshDelaySeconds) * time.Second
	}
	return time.Duration(domain.NormalizeSystemBaseConfig(config).OperationRefreshDelaySeconds) * time.Second
}
