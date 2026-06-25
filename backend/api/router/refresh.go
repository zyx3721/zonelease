package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"zonelease/backend/internal/domain"
	syncsvc "zonelease/backend/internal/service/sync"
)

type createRefreshRequest struct {
	Type string `json:"type"`
}

func (r *Router) createRefresh(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "refresh.manage") {
		return
	}
	var body createRefreshRequest
	if !decode(w, req, &body) {
		return
	}
	if body.Type == "" {
		body.Type = syncsvc.RefreshAllType
	}
	user := currentUser(req)
	task, err := r.store.CreateRefreshTask(req.Context(), body.Type, map[string]any{
		"message": "刷新任务已创建",
	}, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "refresh_failed", "创建刷新任务失败")
		return
	}
	_ = r.realtime.PublishRefresh(req.Context(), refreshEvent(task.ID, "queued", "刷新任务已排队"))
	go r.sync.RunRefreshTask(context.Background(), task.ID, body.Type, nil)
	r.writeAudit(req, "Queued refresh", body.Type, "System", "success", map[string]any{"task": task.ID, "type": body.Type})
	writeJSON(w, http.StatusAccepted, task)
}

func (r *Router) refreshTasks(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "audit.read") {
		return
	}
	limit := 30
	all := false
	if raw := req.URL.Query().Get("limit"); raw != "" {
		if raw == "all" {
			all = true
		} else {
			value, err := strconv.Atoi(raw)
			if err != nil || value <= 0 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit 必须为正整数")
				return
			}
			limit = value
		}
	}
	var (
		tasks []domain.RefreshTask
		err   error
	)
	if all {
		tasks, err = r.store.ListAllRefreshTasks(req.Context())
	} else {
		tasks, err = r.store.ListRefreshTasks(req.Context(), limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "refresh_tasks_failed", "读取刷新任务失败")
		return
	}
	r.attachRefreshTaskRuntime(req.Context(), tasks)
	writeJSON(w, http.StatusOK, map[string]any{"items": tasks})
}

func (r *Router) attachRefreshTaskRuntime(ctx context.Context, tasks []domain.RefreshTask) {
	for i := range tasks {
		if tasks[i].ID == "" || (tasks[i].Status != "queued" && tasks[i].Status != "running") {
			continue
		}
		var cached struct {
			Status  string          `json:"status"`
			Payload json.RawMessage `json:"payload"`
		}
		ok, err := r.realtime.GetJSON(ctx, "zonelease:runtime:refresh-task:"+tasks[i].ID, &cached)
		if err != nil {
			r.logger.Warn("Read refresh task runtime cache failed", "task", tasks[i].ID, "error", err)
			continue
		}
		if !ok {
			continue
		}
		if cached.Status != "" {
			tasks[i].Status = cached.Status
		}
		if len(cached.Payload) > 0 {
			tasks[i].Payload = cached.Payload
		}
	}
}

func (r *Router) events(w http.ResponseWriter, req *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unsupported", "当前连接不支持事件流")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	pubsub := r.realtime.SubscribeRefresh(req.Context())
	defer pubsub.Close()

	fmt.Fprint(w, "event: connected\n")
	fmt.Fprint(w, "data: {}\n\n")
	if events, err := r.realtime.RecentRefreshEvents(req.Context(), 20); err == nil {
		for _, raw := range events {
			eventType := refreshEventType(raw)
			fmt.Fprintf(w, "event: %s\n", eventType)
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
	} else {
		r.logger.Warn("Read recent refresh events failed", "error", err)
	}
	flusher.Flush()

	ch := pubsub.Channel()
	for {
		select {
		case <-req.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: %s\n", refreshEventType(msg.Payload))
			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flusher.Flush()
		}
	}
}

func refreshEventType(payload string) string {
	var event struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(payload), &event)
	if event.Type == "" {
		return "runtime.updated"
	}
	return event.Type
}
