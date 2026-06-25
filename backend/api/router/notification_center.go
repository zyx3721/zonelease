package router

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

type unreadNotificationCountResponse struct {
	Count int `json:"count"`
}

func (r *Router) notifications(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, total, err := r.store.ListNotifications(req.Context(), limit)
	if err != nil {
		r.logger.Error("List notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_notifications_failed", "读取通知消息失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (r *Router) unreadNotificationCount(w http.ResponseWriter, req *http.Request) {
	if count, ok, err := r.realtime.GetInt(req.Context(), unreadNotificationCountCacheKey); err == nil && ok {
		writeJSON(w, http.StatusOK, unreadNotificationCountResponse{Count: count})
		return
	} else if err != nil {
		r.logger.Warn("Read unread notification count cache failed", "error", err)
	}
	count, err := r.store.CountUnreadNotifications(req.Context())
	if err != nil {
		r.logger.Error("Count unread notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "count_notifications_failed", "读取未读通知数量失败")
		return
	}
	r.cacheUnreadNotificationCount(req.Context(), count)
	writeJSON(w, http.StatusOK, unreadNotificationCountResponse{Count: count})
}

func (r *Router) markAllNotificationsRead(w http.ResponseWriter, req *http.Request) {
	if err := r.store.MarkAllNotificationsRead(req.Context()); err != nil {
		r.logger.Error("Mark all notifications read failed", "error", err)
		writeError(w, http.StatusInternalServerError, "mark_notifications_read_failed", "标记通知已读失败")
		return
	}
	r.invalidateUnreadNotificationCount(req.Context())
	r.writeAudit(req, "notifications.read_all", "notifications", "System", "success", map[string]any{"target": "notifications"})
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func (r *Router) clearNotifications(w http.ResponseWriter, req *http.Request) {
	if err := r.store.DismissNotifications(req.Context()); err != nil {
		r.logger.Error("Clear notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "clear_notifications_failed", "清空通知失败")
		return
	}
	r.invalidateUnreadNotificationCount(req.Context())
	r.writeAudit(req, "notifications.clear", "notifications", "System", "success", map[string]any{"target": "notifications"})
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func (r *Router) markNotificationRead(w http.ResponseWriter, req *http.Request) {
	id := pathID(req.URL.Path, "/api/notifications/")
	if id == "" {
		writeError(w, http.StatusNotFound, "notification_not_found", "通知消息不存在")
		return
	}
	if err := r.store.MarkNotificationRead(req.Context(), id); err != nil {
		writeError(w, statusFromErr(err), "notification_not_found", "通知消息不存在")
		return
	}
	r.invalidateUnreadNotificationCount(req.Context())
	r.writeAudit(req, "notifications.read", id, "System", "success", map[string]any{"notification": id})
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

const unreadNotificationCountCacheKey = "zonelease:runtime:notifications:unread-count"

func (r *Router) cacheUnreadNotificationCount(ctx context.Context, count int) {
	if err := r.realtime.SetInt(ctx, unreadNotificationCountCacheKey, count, time.Minute); err != nil {
		r.logger.Warn("Cache unread notification count failed", "error", err)
	}
}

func (r *Router) invalidateUnreadNotificationCount(ctx context.Context) {
	if err := r.realtime.Delete(ctx, unreadNotificationCountCacheKey); err != nil {
		r.logger.Warn("Invalidate unread notification count cache failed", "error", err)
	}
}
