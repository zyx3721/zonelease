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

type agentScopeUpdatePayload struct {
	domain.DHCPScope
	OldStartRange string   `json:"oldStartRange,omitempty"`
	OldEndRange   string   `json:"oldEndRange,omitempty"`
	ChangedFields []string `json:"changedFields,omitempty"`
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
	if !r.ensureAgentNotSyncing(w, req, server) {
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
	if err := validateDHCPScopeDefaultGateway(body.Subnet, body.DefaultGateway); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_default_gateway", err.Error())
		return
	}
	normalizeDHCPScopeLeaseDuration(&body)
	changedFields := dhcpScopeChangedFields(current, body)
	if len(changedFields) == 0 {
		writeJSON(w, http.StatusOK, current)
		return
	}
	refreshTarget := dhcpScopeRefreshTarget(server.ID, current.ID, scopeExternalID, body.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()

	var agentScope domain.DHCPScope
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	agentPayload := agentScopeUpdatePayload{
		DHCPScope:     body,
		OldStartRange: current.StartRange,
		OldEndRange:   current.EndRange,
		ChangedFields: changedFields,
	}
	if err := r.agent.Put(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes/"+url.PathEscape(scopeExternalID), agentPayload, &agentScope, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_update_scope_failed", "Agent 更新 DHCP 作用域失败："+err.Error())
		return
	}
	body.ID = current.ID
	body.ExternalID = scopeExternalID
	body.ServerID = server.ID
	if updated, err := r.store.UpdateScope(req.Context(), body); err == nil {
		body = updated
	}
	r.writeAudit(req, "Updated DHCP scope", body.Name, "DHCP", "success", dhcpScopeAuditMetadata(server, body.ID, body.Name, map[string]any{
		"subnet":     body.Subnet,
		"rangeStart": body.StartRange,
		"rangeEnd":   body.EndRange,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, body)
}

func dhcpScopeChangedFields(current, next domain.DHCPScope) []string {
	var fields []string
	if strings.TrimSpace(current.Name) != strings.TrimSpace(next.Name) {
		fields = append(fields, "name")
	}
	if strings.TrimSpace(current.Description) != strings.TrimSpace(next.Description) {
		fields = append(fields, "description")
	}
	if strings.TrimSpace(current.DefaultGateway) != strings.TrimSpace(next.DefaultGateway) {
		fields = append(fields, "gateway")
	}
	if current.LeaseDurationSeconds != next.LeaseDurationSeconds {
		fields = append(fields, "lease")
	}
	if strings.TrimSpace(current.StartRange) != strings.TrimSpace(next.StartRange) ||
		strings.TrimSpace(current.EndRange) != strings.TrimSpace(next.EndRange) {
		fields = append(fields, "range")
	}
	if strings.TrimSpace(current.State) != strings.TrimSpace(next.State) {
		fields = append(fields, "state")
	}
	return fields
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
	if !r.ensureAgentNotSyncing(w, req, server) {
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
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()

	oldReservation := current
	oldReservation.ScopeID = scopeExternalID
	newReservation := body
	newReservation.ID = current.ID
	newReservation.ScopeID = scopeExternalID
	newReservation.IP = current.IP
	newReservation.MAC = current.MAC
	if strings.TrimSpace(newReservation.ExternalID) == "" {
		newReservation.ExternalID = newReservation.IP
	}
	var agentReservation domain.DHCPReservation
	payload := agentReservationUpdatePayload{Old: oldReservation, New: newReservation}
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/reservations/update", payload, &agentReservation, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_update_reservation_failed", "Agent 更新 DHCP 保留地址失败："+err.Error())
		return
	}
	newReservation.ScopeID = scope.ID
	updated, err := r.store.UpdateReservation(req.Context(), newReservation)
	if err != nil {
		writeError(w, statusFromErr(err), "update_reservation_failed", "更新 DHCP 保留地址快照失败")
		return
	}
	if err := r.store.UpdateLeaseHostnameByScopeIP(req.Context(), scope.ID, updated.IP, updated.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "update_lease_snapshot_failed", "更新 DHCP 租约快照失败")
		return
	}
	newReservation = updated
	r.writeAudit(req, "Updated DHCP reservation", newReservation.IP, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{
		"oldIp": current.IP,
		"ip":    newReservation.IP,
		"mac":   newReservation.MAC,
		"name":  newReservation.Name,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, newReservation)
}
