package router

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

var (
	errServerNameDuplicate     = errors.New("server name already exists")
	errServerAgentURLDuplicate = errors.New("server agent url already exists")
)

func (r *Router) health(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()

	services := map[string]map[string]string{
		"postgresql": {"status": "online"},
		"redis":      {"status": "online"},
	}
	status := "ok"
	if err := r.store.Pool().Ping(ctx); err != nil {
		status = "degraded"
		services["postgresql"] = map[string]string{"status": "offline", "error": err.Error()}
		r.notifyPlatformServiceIssue(req.Context(), "postgresql", err.Error())
	} else {
		r.clearPlatformServiceNotification(req.Context(), "postgresql")
	}
	if err := r.realtime.Ping(ctx); err != nil {
		status = "degraded"
		services["redis"] = map[string]string{"status": "offline", "error": err.Error()}
		r.notifyPlatformServiceIssue(req.Context(), "redis", err.Error())
	} else {
		r.clearPlatformServiceNotification(req.Context(), "redis")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   status,
		"time":     time.Now().UTC().Format(time.RFC3339Nano),
		"services": services,
	})
}

func (r *Router) ensureServerUnique(ctx context.Context, name, agentURL string) error {
	if _, err := r.store.GetServerByName(ctx, name); err == nil {
		return errServerNameDuplicate
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if _, err := r.store.GetServerByAgentURL(ctx, agentURL); err == nil {
		return errServerAgentURLDuplicate
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	return nil
}

func validAgentRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "dns", "dhcp":
		return true
	default:
		return false
	}
}

func (r *Router) state(w http.ResponseWriter, req *http.Request) {
	state, err := r.store.State(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state_failed", "读取状态失败")
		return
	}
	r.attachAgentHealthRuntime(req.Context(), state.Servers)
	writeJSON(w, http.StatusOK, filterStateForUser(state, currentUser(req)))
}

func (r *Router) attachAgentHealthRuntime(ctx context.Context, servers []domain.Server) {
	for i := range servers {
		if strings.TrimSpace(servers[i].ID) == "" {
			continue
		}
		var cached struct {
			FailureCount *int `json:"failureCount"`
		}
		ok, err := r.realtime.GetJSON(ctx, agentHealthRuntimeKey(servers[i].ID), &cached)
		if err != nil {
			r.logger.Warn("Read agent health runtime cache failed", "server", servers[i].ID, "error", err)
			continue
		}
		if ok && cached.FailureCount != nil {
			servers[i].FailureCount = *cached.FailureCount
		}
	}
}

func (r *Router) createServer(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "servers.manage") {
		return
	}
	var body domain.Server
	if !decode(w, req, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.AgentURL = strings.TrimRight(strings.TrimSpace(body.AgentURL), "/")
	body.APIKey = strings.TrimSpace(body.APIKey)
	body.Role = strings.TrimSpace(body.Role)
	if body.Name == "" || body.AgentURL == "" {
		writeError(w, http.StatusBadRequest, "invalid_server", "Agent 名称和地址不能为空")
		return
	}
	if body.Role == "" {
		writeError(w, http.StatusBadRequest, "invalid_server", "请选择 Agent 角色")
		return
	}
	if !validAgentRole(body.Role) {
		writeError(w, http.StatusBadRequest, "invalid_server", "Agent 角色仅支持 DNS 或 DHCP")
		return
	}
	if !strings.HasPrefix(body.AgentURL, "http://") && !strings.HasPrefix(body.AgentURL, "https://") {
		writeError(w, http.StatusBadRequest, "invalid_agent_url", "Agent 地址必须以 http:// 或 https:// 开头")
		return
	}
	if err := r.ensureServerUnique(req.Context(), body.Name, body.AgentURL); err != nil {
		switch {
		case errors.Is(err, errServerNameDuplicate):
			writeError(w, http.StatusConflict, "server_name_exists", "Agent 名称已存在")
		case errors.Is(err, errServerAgentURLDuplicate):
			writeError(w, http.StatusConflict, "server_agent_url_exists", "Agent 接口地址已存在")
		default:
			r.logger.Error("Check server uniqueness failed", "error", err)
			writeError(w, http.StatusInternalServerError, "check_server_unique_failed", "检查 Agent 唯一性失败")
		}
		return
	}
	if strings.TrimSpace(body.Host) == "" {
		body.Host = body.Name
	}
	item, err := r.store.CreateServer(req.Context(), body)
	if err != nil {
		if repository.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "server_name_exists", "Agent 名称已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "create_server_failed", "添加服务器失败")
		return
	}
	r.writeAudit(req, "Created server", item.Name, "Server", "success", serverAuditMetadata(item, nil))
	writeJSON(w, http.StatusCreated, item)
}

func (r *Router) probeServer(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "servers.manage") {
		return
	}
	var body domain.Server
	if !decode(w, req, &body) {
		return
	}
	body.AgentURL = strings.TrimRight(strings.TrimSpace(body.AgentURL), "/")
	body.APIKey = strings.TrimSpace(body.APIKey)
	body.Role = strings.TrimSpace(body.Role)
	if body.AgentURL == "" {
		writeError(w, http.StatusBadRequest, "missing_agent_fields", "Agent 地址不能为空")
		return
	}
	if !validAgentRole(body.Role) {
		writeError(w, http.StatusBadRequest, "invalid_server", "Agent 角色仅支持 DNS 或 DHCP")
		return
	}
	if !strings.HasPrefix(body.AgentURL, "http://") && !strings.HasPrefix(body.AgentURL, "https://") {
		writeError(w, http.StatusBadRequest, "invalid_agent_url", "Agent 地址必须以 http:// 或 https:// 开头")
		return
	}
	status, detail := "Online", ""
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentConnectionTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Validate(agentCtx, body.AgentURL, body.APIKey, body.Role, body.TLSInsecure); err != nil {
		status, detail = "Offline", agent.UserFacingErrorMessage(err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status, "detail": detail})
}

func (r *Router) deleteServer(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "servers.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/servers/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "server_not_found", "服务器不存在")
		return
	}
	if err := r.store.DeleteServer(req.Context(), id); err != nil {
		writeError(w, statusFromErr(err), "delete_server_failed", "删除服务器失败")
		return
	}
	if err := r.store.DismissNotificationsBySource(req.Context(), notificationSourceAgentHealth, id); err != nil {
		r.logger.Warn("Clear deleted agent offline notification failed", "server", id, "error", err)
	} else {
		r.invalidateUnreadNotificationCount(req.Context())
	}
	if err := r.realtime.Delete(req.Context(), agentHealthRuntimeKey(id)); err != nil {
		r.logger.Warn("Clear deleted agent health runtime failed", "server", id, "error", err)
	}
	r.writeAudit(req, "Deleted server", server.Name, "Server", "success", serverAuditMetadata(server, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) serverAction(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "servers.manage") {
		return
	}
	id, action := pathIDAction(req.URL.Path, "/api/servers/")
	if id == "" || (action != "ping" && action != "sync") {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	if action == "sync" {
		if !r.ensureAgentNotSyncing(w, req, server) {
			return
		}
		task, err := r.enqueueServerRefresh(server.ID, server.Name, currentUser(req).ID, req.URL.Query().Get("skipHealthCheck") == "1")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "sync_server_failed", "创建 Agent 同步任务失败")
			return
		}
		r.writeAudit(req, "Queued server sync", server.Name, "Server", "success", serverAuditMetadata(server, nil))
		writeJSON(w, http.StatusAccepted, task)
		return
	}
	status, detail := "Online", ""
	started := time.Now()
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentConnectionTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Validate(agentCtx, server.AgentURL, server.APIKey, server.Role, server.TLSInsecure); err != nil {
		status, detail = "Offline", agent.UserFacingErrorMessage(err)
	}
	offlineFailureLimit := r.agentOfflineFailureLimit(req.Context())
	if status == "Offline" && req.URL.Query().Get("mode") != "auto" {
		offlineFailureLimit = 1
	}
	healthUpdate, err := r.store.UpdateServerHealth(req.Context(), id, status, offlineFailureLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update_health_failed", "更新健康状态失败")
		return
	}
	r.cacheAgentHealth(req.Context(), server, status, detail, healthUpdate.FailureCount, time.Since(started))
	if healthUpdate.Status == "Online" {
		r.clearAgentOfflineNotification(req.Context(), server)
	} else if healthUpdate.BecameOffline() {
		r.notifyAgentOffline(req.Context(), server, detail)
	}
	if req.URL.Query().Get("mode") != "auto" {
		metadata := map[string]any{"status": status}
		if strings.TrimSpace(detail) != "" {
			metadata["error"] = detail
		}
		r.writeAudit(req, "Checked server health", server.Name, "Server", auditResult(status == "Online"), serverAuditMetadata(server, metadata))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status, "detail": detail})
}

func (r *Router) cacheAgentHealth(ctx context.Context, server domain.Server, status, detail string, failureCount int, duration time.Duration) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	payload := map[string]any{
		"serverId":       server.ID,
		"serverName":     server.Name,
		"status":         status,
		"error":          detail,
		"failureCount":   failureCount,
		"durationMillis": duration.Milliseconds(),
		"checkedAt":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := r.realtime.Set(ctx, agentHealthRuntimeKey(server.ID), payload, 30*time.Minute); err != nil {
		r.logger.Warn("Cache agent health runtime failed", "server", server.ID, "error", err)
	}
}

func agentHealthRuntimeKey(serverID string) string {
	return "zonelease:runtime:agent-health:" + strings.TrimSpace(serverID)
}
