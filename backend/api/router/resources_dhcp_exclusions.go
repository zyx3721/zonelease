package router

import (
	"context"
	"net/http"
	"strings"

	"zonelease/backend/internal/domain"
)

func (r *Router) createExclusion(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	var body domain.DHCPExclusion
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.ScopeID) == "" || strings.TrimSpace(body.StartIP) == "" || strings.TrimSpace(body.EndIP) == "" {
		writeError(w, http.StatusBadRequest, "invalid_exclusion", "排除范围参数不完整")
		return
	}
	scope, err := r.store.GetScope(req.Context(), body.ScopeID)
	if err != nil {
		writeError(w, statusFromErr(err), "scope_not_found", "DHCP 作用域不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), scope.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	body.ScopeID = scopeExternalID
	var agentExclusion domain.DHCPExclusion
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/exclusions", body, &agentExclusion, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_create_exclusion_failed", "Agent 创建 DHCP 排除范围失败："+err.Error())
		return
	}
	if strings.TrimSpace(agentExclusion.StartIP) != "" {
		body.StartIP = agentExclusion.StartIP
		body.EndIP = agentExclusion.EndIP
		body.ExternalID = agentExclusion.ExternalID
	}
	body.ScopeID = scope.ID
	if item, err := r.store.CreateExclusion(req.Context(), body); err == nil {
		body = item
	}
	r.writeAudit(req, "Created DHCP exclusion", body.StartIP+"-"+body.EndIP, "DHCP", "success", map[string]any{
		"scope":   scope.Name,
		"startIp": body.StartIP,
		"endIp":   body.EndIP,
		"server":  server.Name,
	})
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusCreated, body)
}

func (r *Router) deleteExclusion(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/exclusions/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	exclusion, err := r.store.GetExclusion(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "exclusion_not_found", "DHCP 排除范围不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), exclusion.ScopeID)
	if err != nil {
		writeError(w, statusFromErr(err), "scope_not_found", "DHCP 作用域不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), scope.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	var ignored map[string]any
	body := map[string]string{"scopeId": scopeExternalID, "startIp": exclusion.StartIP, "endIp": exclusion.EndIP}
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/exclusions/delete", body, &ignored, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_delete_exclusion_failed", "Agent 删除 DHCP 排除范围失败："+err.Error())
		return
	}
	_ = r.store.DeleteExclusion(req.Context(), id)
	r.writeAudit(req, "Deleted DHCP exclusion", exclusion.StartIP+"-"+exclusion.EndIP, "DHCP", "success", map[string]any{
		"scope":   scope.Name,
		"startIp": exclusion.StartIP,
		"endIp":   exclusion.EndIP,
		"server":  server.Name,
	})
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
