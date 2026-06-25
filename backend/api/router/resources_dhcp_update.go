package router

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"zonelease/backend/internal/domain"
)

type agentReservationUpdatePayload struct {
	Old domain.DHCPReservation `json:"old"`
	New domain.DHCPReservation `json:"new"`
}

func (r *Router) updateScope(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/scopes/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	current, err := r.store.GetScope(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "scope_not_found", "DHCP 作用域不存在")
		return
	}
	var body domain.DHCPScope
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_scope", "作用域参数不完整")
		return
	}
	server, err := r.store.GetServer(req.Context(), current.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	scopeExternalID := dhcpExternalID(current.ExternalID, current.Subnet, current.ID)
	body.ID = scopeExternalID
	body.ExternalID = scopeExternalID
	body.ServerID = server.ID
	body.Subnet = current.Subnet
	if strings.TrimSpace(body.StartRange) == "" {
		body.StartRange = current.StartRange
	}
	if strings.TrimSpace(body.EndRange) == "" {
		body.EndRange = current.EndRange
	}
	if strings.TrimSpace(body.State) == "" {
		body.State = current.State
	}
	normalizeDHCPScopeLeaseDuration(&body)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scopeExternalID, body.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()

	var agentScope domain.DHCPScope
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Put(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes/"+url.PathEscape(scopeExternalID), body, &agentScope, server.TLSInsecure); err != nil {
		writeError(w, http.StatusBadGateway, "agent_update_scope_failed", "Agent 更新 DHCP 作用域失败："+err.Error())
		return
	}
	body.ID = current.ID
	body.ExternalID = scopeExternalID
	body.ServerID = server.ID
	if updated, err := r.store.UpdateScope(req.Context(), body); err == nil {
		body = updated
	}
	r.writeAudit(req, "Updated DHCP scope", body.Name, "DHCP", "success", map[string]any{
		"scope":      body.Name,
		"scopeId":    body.ID,
		"subnet":     body.Subnet,
		"rangeStart": body.StartRange,
		"rangeEnd":   body.EndRange,
		"server":     server.Name,
	})
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, body)
}

func (r *Router) updateReservation(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/reservations/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	current, err := r.store.GetReservation(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "reservation_not_found", "DHCP 保留地址不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), current.ScopeID)
	if err != nil {
		writeError(w, statusFromErr(err), "scope_not_found", "DHCP 作用域不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), scope.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	var body domain.DHCPReservation
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.IP) == "" || strings.TrimSpace(body.MAC) == "" || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "保留地址参数不完整")
		return
	}
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()

	oldReservation := current
	oldReservation.ScopeID = scopeExternalID
	newReservation := body
	newReservation.ID = current.ID
	newReservation.ScopeID = scopeExternalID
	if strings.TrimSpace(newReservation.ExternalID) == "" {
		newReservation.ExternalID = newReservation.IP
	}
	var agentReservation domain.DHCPReservation
	payload := agentReservationUpdatePayload{Old: oldReservation, New: newReservation}
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/reservations/update", payload, &agentReservation, server.TLSInsecure); err != nil {
		writeError(w, http.StatusBadGateway, "agent_update_reservation_failed", "Agent 更新 DHCP 保留地址失败："+err.Error())
		return
	}
	newReservation.ScopeID = scope.ID
	if updated, err := r.store.UpdateReservation(req.Context(), newReservation); err == nil {
		newReservation = updated
	}
	r.writeAudit(req, "Updated DHCP reservation", newReservation.IP, "DHCP", "success", map[string]any{
		"scope":  scope.Name,
		"oldIp":  current.IP,
		"ip":     newReservation.IP,
		"mac":    newReservation.MAC,
		"name":   newReservation.Name,
		"server": server.Name,
	})
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, newReservation)
}
