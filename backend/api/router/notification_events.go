package router

import (
	"context"
	"fmt"
	"strings"

	"zonelease/backend/internal/domain"
)

const (
	notificationSourceAgentHealth     = "agent_health"
	notificationSourcePlatformService = "platform_service"
)

func (r *Router) notifyPlatformServiceIssue(ctx context.Context, serviceID, detail string) {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		return
	}
	title := platformServiceNotificationTitle(serviceID)
	message := platformServiceNotificationMessage(serviceID)
	if err := r.createUnreadDedupedNotification(ctx, "critical", title, message, notificationSourcePlatformService, serviceID, map[string]any{
		"service": serviceID,
		"status":  "offline",
		"error":   detail,
	}); err != nil {
		r.logger.Warn("Create platform service notification failed", "service", serviceID, "error", err)
		return
	}
	r.invalidateUnreadNotificationCount(ctx)
}

func (r *Router) clearPlatformServiceNotification(ctx context.Context, serviceID string) {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		return
	}
	if err := r.store.DismissNotificationsBySource(ctx, notificationSourcePlatformService, serviceID); err != nil {
		r.logger.Warn("Clear platform service notification failed", "service", serviceID, "error", err)
		return
	}
	r.invalidateUnreadNotificationCount(ctx)
}

func (r *Router) notifyAgentOffline(ctx context.Context, server domain.Server, detail string) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	name := strings.TrimSpace(server.Name)
	if name == "" {
		name = server.Host
	}
	if name == "" {
		name = server.ID
	}
	if err := r.createUnreadDedupedNotification(ctx, "critical", "Agent 离线", fmt.Sprintf("%s 无法通过健康检查", name), notificationSourceAgentHealth, server.ID, map[string]any{
		"server":   name,
		"serverId": server.ID,
		"status":   "Offline",
		"error":    detail,
	}); err != nil {
		r.logger.Warn("Create agent offline notification failed", "server", server.ID, "error", err)
		return
	}
	r.invalidateUnreadNotificationCount(ctx)
}

func (r *Router) clearAgentOfflineNotification(ctx context.Context, server domain.Server) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	if err := r.store.DismissNotificationsBySource(ctx, notificationSourceAgentHealth, server.ID); err != nil {
		r.logger.Warn("Clear agent offline notification failed", "server", server.ID, "error", err)
		return
	}
	r.invalidateUnreadNotificationCount(ctx)
}

func (r *Router) createUnreadDedupedNotification(ctx context.Context, level, title, message, sourceType, sourceID string, metadata map[string]any) error {
	_, _, err := r.store.CreateNotificationIfUnreadMissing(ctx, level, title, message, sourceType, sourceID, metadata)
	return err
}

func platformServiceNotificationTitle(serviceID string) string {
	if serviceID == "postgresql" {
		return "PostgreSQL 连接异常"
	}
	if serviceID == "redis" {
		return "Redis 连接异常"
	}
	return "平台基础服务异常"
}

func platformServiceNotificationMessage(serviceID string) string {
	if serviceID == "postgresql" {
		return "PostgreSQL 连接不可用，请检查数据库服务状态"
	}
	if serviceID == "redis" {
		return "Redis 连接不可用，请检查缓存与事件通道状态"
	}
	return "平台基础服务不可用，请检查服务运行状态"
}
