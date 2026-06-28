package router

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	"zonelease/backend/internal/service/realtime"
	syncsvc "zonelease/backend/internal/service/sync"
)

var (
	errRefreshTargetRunning   = errors.New("refresh target is running")
	errRuntimeLockUnavailable = errors.New("runtime lock unavailable")
)

type dnsZoneCreateResponse struct {
	domain.DNSZone
	Records []domain.DNSRecord `json:"records,omitempty"`
	Warning string             `json:"warning,omitempty"`
}

func (r *Router) createZone(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	var body domain.DNSZone
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.ServerID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_zone", "区域名称和服务器不能为空")
		return
	}
	body.Name = normalizeDNSZoneName(body.Name, body.Reverse)
	response := dnsZoneCreateResponse{}
	createdServer := domain.Server{ID: body.ServerID}
	if srv, err := r.store.GetServer(req.Context(), body.ServerID); err == nil {
		createdServer = srv
		if !r.ensureAgentNotSyncing(w, req, srv) {
			return
		}
		var ignored map[string]any
		agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
		defer cancel()
		if err := r.agent.Post(agentCtx, srv.AgentURL, srv.APIKey, "/dns/zones", body, &ignored, srv.TLSInsecure); err != nil {
			writeError(w, http.StatusBadGateway, "agent_create_zone_failed", "Agent 创建 DNS 区域失败："+err.Error())
			return
		}
		body.ID = repository.DNSZoneID(srv.ID, body.Name)
		body.ServerID = srv.ID
		refreshTarget := dnsZoneRefreshTarget(body.ServerID, body.ID, body.Name)
		finishRefresh := r.refresh.begin(refreshTarget)
		defer finishRefresh()
		body, _ = r.store.UpsertDNSZone(req.Context(), body)
		records, err := r.fetchDNSRecordsFromAgent(agentCtx, srv, body.Name)
		if err != nil {
			response.Warning = "区域默认记录刷新失败：" + err.Error()
		} else {
			for i := range records {
				records[i].ZoneID = body.ID
				records[i].ID = repository.DNSRecordID(srv.ID, body.Name, records[i].Type, records[i].Name, records[i].Value)
			}
			if err := r.store.ReplaceDNSZoneRecords(req.Context(), body, records); err != nil {
				response.Warning = "区域默认记录写入失败：" + err.Error()
			} else {
				response.Records = records
			}
		}
		r.refresh.markDirty(refreshTarget)
	} else {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	r.writeAudit(req, "Created zone", body.Name, "DNS", "success", dnsAuditMetadata(createdServer, body.ID, body.Name, nil))
	response.DNSZone = body
	writeJSON(w, http.StatusCreated, response)
}

func (r *Router) zoneAction(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	id, action := pathIDAction(req.URL.Path, "/api/dns/zones/")
	if id == "" || action != "refresh" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	zone, err := r.store.GetDNSZone(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "zone_not_found", "DNS 区域不存在")
		return
	}
	server, err := r.store.GetServer(req.Context(), zone.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	if !r.ensureAgentNotSyncing(w, req, server) {
		return
	}
	task, err := r.enqueueZoneRefresh(zone.ServerID, zone.ID, zone.Name, currentUser(req).ID)
	if err != nil {
		if errors.Is(err, errRefreshTargetRunning) {
			writeError(w, http.StatusConflict, "refresh_target_running", "当前刷新目标正在执行，请稍后再试")
			return
		}
		if errors.Is(err, errRuntimeLockUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "runtime_lock_unavailable", "运行态锁服务异常，请稍后再试")
			return
		}
		writeError(w, http.StatusInternalServerError, "refresh_zone_failed", "创建区域刷新任务失败")
		return
	}
	r.writeAudit(req, "Queued DNS zone refresh", zone.Name, "DNS", "success", dnsAuditMetadata(server, zone.ID, zone.Name, nil))
	writeJSON(w, http.StatusAccepted, task)
}

func (r *Router) deleteZone(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dns/zones/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	serverID, zoneName, ok := repository.DecodeDNSZoneID(id)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_zone_id", "DNS 区域标识无效")
		return
	}
	server, err := r.store.GetServer(req.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	if !r.ensureAgentNotSyncing(w, req, server) {
		return
	}
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Delete(agentCtx, server.AgentURL, server.APIKey, "/dns/zones/"+url.PathEscape(zoneName), &ignored, server.TLSInsecure); err != nil {
		writeError(w, http.StatusBadGateway, "agent_delete_zone_failed", "Agent 删除 DNS 区域失败："+err.Error())
		return
	}
	r.writeAudit(req, "Deleted zone", zoneName, "DNS", "success", dnsAuditMetadata(server, id, zoneName, nil))
	_ = r.store.DeleteDNSZone(req.Context(), id)
	_ = r.realtime.PublishRefresh(req.Context(), realtime.RefreshEvent{Type: "runtime.updated", Status: "success", Message: "运行态已更新"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) createRecord(w http.ResponseWriter, req *http.Request) {
	r.createDNSRecord(w, req)
}

func (r *Router) deleteRecord(w http.ResponseWriter, req *http.Request) {
	r.deleteDNSRecord(w, req)
}

func (r *Router) updateRecord(w http.ResponseWriter, req *http.Request) {
	r.updateDNSRecord(w, req)
}

func (r *Router) createScope(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	var body domain.DHCPScope
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Subnet) == "" || strings.TrimSpace(body.ServerID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_scope", "作用域参数不完整")
		return
	}
	server, err := r.store.GetServer(req.Context(), body.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	if !r.ensureAgentNotSyncing(w, req, server) {
		return
	}
	body.ExternalID = dhcpExternalID(body.ExternalID, body.ID, body.Subnet)
	body.ID = body.ExternalID
	body.ServerID = server.ID
	if err := validateDHCPScopeDefaultGateway(body.Subnet, body.DefaultGateway); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_default_gateway", err.Error())
		return
	}
	normalizeDHCPScopeLeaseDuration(&body)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, "", body.ExternalID, body.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes", body, &ignored, server.TLSInsecure); err != nil {
		writeError(w, http.StatusBadGateway, "agent_create_scope_failed", "Agent 创建 DHCP 作用域失败："+err.Error())
		return
	}
	if item, err := r.store.CreateScope(req.Context(), body); err == nil {
		body.ID = item.ID
	}
	refreshTarget.ScopeID = body.ID
	r.writeAudit(req, "Created DHCP scope", body.Name, "DHCP", "success", dhcpScopeAuditMetadata(server, body.ID, body.Name, map[string]any{
		"subnet":     body.Subnet,
		"rangeStart": body.StartRange,
		"rangeEnd":   body.EndRange,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusCreated, body)
}

func (r *Router) scopeAction(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id, action := pathIDAction(req.URL.Path, "/api/dhcp/scopes/")
	if id == "" || (action != "toggle" && action != "refresh") {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), id)
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
	if action == "refresh" {
		if !r.ensureAgentNotSyncing(w, req, server) {
			return
		}
		task, err := r.enqueueDHCPScopeRefresh(server.ID, scope.ID, scopeExternalID, scope.Name, currentUser(req).ID)
		if err != nil {
			if errors.Is(err, errRefreshTargetRunning) {
				writeError(w, http.StatusConflict, "refresh_target_running", "当前刷新目标正在执行，请稍后再试")
				return
			}
			if errors.Is(err, errRuntimeLockUnavailable) {
				writeError(w, http.StatusServiceUnavailable, "runtime_lock_unavailable", "运行态锁服务异常，请稍后再试")
				return
			}
			writeError(w, http.StatusInternalServerError, "refresh_scope_failed", "创建 DHCP 作用域刷新任务失败")
			return
		}
		r.writeAudit(req, "Queued DHCP scope refresh", scope.Name, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, nil))
		writeJSON(w, http.StatusAccepted, task)
		return
	}
	if !r.ensureAgentNotSyncing(w, req, server) {
		return
	}
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	agentAction := "deactivate"
	if !strings.EqualFold(scope.State, "Active") {
		agentAction = "activate"
	}
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes/"+url.PathEscape(scopeExternalID)+"/"+agentAction, nil, &ignored, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_toggle_scope_failed", "Agent 切换 DHCP 作用域状态失败："+err.Error())
		return
	}
	_ = r.store.ToggleScope(req.Context(), id)
	r.writeAudit(req, "Toggled DHCP scope", scope.Name, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{
		"action": agentAction,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) deleteScope(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/scopes/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), id)
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
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Delete(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes/"+url.PathEscape(scopeExternalID), &ignored, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_delete_scope_failed", "Agent 删除 DHCP 作用域失败："+err.Error())
		return
	}
	if err := r.store.DeleteScope(req.Context(), id); err != nil {
		writeError(w, statusFromErr(err), "delete_scope_failed", "删除 DHCP 作用域快照失败")
		return
	}
	r.writeAudit(req, "Deleted DHCP scope", scope.Name, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{"subnet": scope.Subnet}))
	_ = r.realtime.PublishRefresh(req.Context(), realtime.RefreshEvent{Type: "runtime.updated", Status: "success", Message: "运行态已更新"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) deleteLease(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/leases/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	lease, err := r.store.GetLease(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "lease_not_found", "DHCP 租约不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), lease.ScopeID)
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
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/leases/release", map[string]string{"scopeId": scopeExternalID, "ip": lease.IP}, &ignored, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
			writeError(w, http.StatusBadGateway, "agent_delete_lease_failed", "Agent 释放 DHCP 租约失败："+err.Error())
			return
		}
		err = r.agent.Delete(agentCtx, server.AgentURL, server.APIKey, "/dhcp/scopes/"+url.PathEscape(scopeExternalID)+"/leases/"+url.PathEscape(lease.IP), &ignored, server.TLSInsecure)
		if err == nil {
			goto leaseReleased
		}
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_delete_lease_failed", "Agent 释放 DHCP 租约失败："+err.Error())
		return
	}
leaseReleased:
	_ = r.store.DeleteLease(req.Context(), id)
	r.writeAudit(req, "Released DHCP lease", lease.IP, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{
		"ip":       lease.IP,
		"mac":      lease.MAC,
		"hostname": lease.Hostname,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) createReservation(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	var body domain.DHCPReservation
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.ScopeID) == "" || strings.TrimSpace(body.IP) == "" || strings.TrimSpace(body.MAC) == "" {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "保留地址参数不完整")
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
	if !r.ensureAgentNotSyncing(w, req, server) {
		return
	}
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	body.ScopeID = scopeExternalID
	var agentReservation domain.DHCPReservation
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/reservations", body, &agentReservation, server.TLSInsecure); err != nil {
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_create_reservation_failed", "Agent 创建 DHCP 保留地址失败："+err.Error())
		return
	}
	if strings.TrimSpace(agentReservation.IP) != "" {
		body.IP = agentReservation.IP
		body.MAC = agentReservation.MAC
		body.Name = agentReservation.Name
		body.Description = agentReservation.Description
	}
	body.ScopeID = scope.ID
	item, err := r.store.CreateReservation(req.Context(), body)
	if err != nil {
		writeError(w, statusFromErr(err), "create_reservation_snapshot_failed", "创建 DHCP 保留地址快照失败")
		return
	}
	body.ID = item.ID
	if err := r.store.MarkLeaseReservedInactiveWithHostname(req.Context(), scope.ID, body.IP, body.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "update_lease_snapshot_failed", "更新 DHCP 租约快照失败")
		return
	}
	r.writeAudit(req, "Created DHCP reservation", body.IP, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{
		"ip":   body.IP,
		"mac":  body.MAC,
		"name": body.Name,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusCreated, body)
}

func (r *Router) deleteReservation(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dhcp.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dhcp/reservations/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	reservation, err := r.store.GetReservation(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "reservation_not_found", "DHCP 保留地址不存在")
		return
	}
	scope, err := r.store.GetScope(req.Context(), reservation.ScopeID)
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
	scopeExternalID := dhcpExternalID(scope.ExternalID, scope.Subnet, scope.ID)
	refreshTarget := dhcpScopeRefreshTarget(server.ID, scope.ID, scopeExternalID, scope.Name)
	finishRefresh := r.refresh.begin(refreshTarget)
	defer finishRefresh()
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.agent.Post(agentCtx, server.AgentURL, server.APIKey, "/dhcp/reservations/delete", map[string]string{"scopeId": scopeExternalID, "ip": reservation.IP, "mac": reservation.MAC}, &ignored, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
			writeError(w, http.StatusBadGateway, "agent_delete_reservation_failed", "Agent 删除 DHCP 保留地址失败："+err.Error())
			return
		}
		err = r.agent.Delete(agentCtx, server.AgentURL, server.APIKey, "/dhcp/reservations/"+url.PathEscape(scopeExternalID)+"/"+url.PathEscape(reservation.IP), &ignored, server.TLSInsecure)
		if err == nil {
			goto reservationDeleted
		}
		r.markDHCPScopeDirtyAfterAgentFailure(refreshTarget, err)
		writeError(w, http.StatusBadGateway, "agent_delete_reservation_failed", "Agent 删除 DHCP 保留地址失败："+err.Error())
		return
	}
reservationDeleted:
	_ = r.store.DeleteReservation(req.Context(), id)
	_ = r.store.DeleteLeaseByScopeIP(req.Context(), scope.ID, reservation.IP)
	r.writeAudit(req, "Deleted DHCP reservation", reservation.IP, "DHCP", "success", dhcpScopeAuditMetadata(server, scope.ID, scope.Name, map[string]any{
		"ip":   reservation.IP,
		"mac":  reservation.MAC,
		"name": reservation.Name,
	}))
	r.refresh.markDirty(refreshTarget)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func dhcpExternalID(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			return strings.Split(value, "/")[0]
		}
		return value
	}
	return ""
}

func normalizeDHCPScopeLeaseDuration(scope *domain.DHCPScope) {
	if scope.LeaseDurationSeconds == -1 {
		scope.LeaseDurationHours = 0
		return
	}
	if scope.LeaseDurationSeconds <= 0 && scope.LeaseDurationHours > 0 {
		scope.LeaseDurationSeconds = scope.LeaseDurationHours * 3600
	}
	if scope.LeaseDurationHours <= 0 && scope.LeaseDurationSeconds > 0 {
		scope.LeaseDurationHours = (scope.LeaseDurationSeconds + 3599) / 3600
	}
	if scope.LeaseDurationHours <= 0 {
		scope.LeaseDurationHours = 24
	}
	if scope.LeaseDurationSeconds <= 0 {
		scope.LeaseDurationSeconds = scope.LeaseDurationHours * 3600
	}
}

func validateDHCPScopeDefaultGateway(subnet, defaultGateway string) error {
	defaultGateway = strings.TrimSpace(defaultGateway)
	if defaultGateway == "" {
		return fmt.Errorf("默认网关不能为空")
	}
	ip := net.ParseIP(defaultGateway).To4()
	if ip == nil {
		return fmt.Errorf("默认网关必须是有效的 IPv4 地址")
	}
	networkIP, maskIP, err := parseDHCPScopeSubnet(subnet)
	if err != nil {
		return err
	}
	mask := net.IPMask(maskIP)
	network := networkIP.Mask(mask)
	if !ip.Mask(mask).Equal(network) {
		return fmt.Errorf("默认网关必须在当前作用域子网内")
	}
	networkUint := binary.BigEndian.Uint32(network)
	gatewayUint := binary.BigEndian.Uint32(ip)
	broadcastUint := networkUint | ^binary.BigEndian.Uint32(maskIP)
	if gatewayUint == networkUint || gatewayUint == broadcastUint {
		return fmt.Errorf("默认网关不能是子网地址或广播地址")
	}
	return nil
}

func parseDHCPScopeSubnet(subnet string) (net.IP, net.IP, error) {
	parts := strings.Split(strings.TrimSpace(subnet), "/")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("子网必须是有效的 IPv4 CIDR")
	}
	ip := net.ParseIP(strings.TrimSpace(parts[0])).To4()
	if ip == nil {
		return nil, nil, fmt.Errorf("子网必须是有效的 IPv4 CIDR")
	}
	maskText := strings.TrimSpace(parts[1])
	if maskText == "" {
		return nil, nil, fmt.Errorf("子网必须是有效的 IPv4 CIDR")
	}
	prefix := -1
	if _, err := fmt.Sscanf(maskText, "%d", &prefix); err != nil || prefix < 0 || prefix > 32 {
		return nil, nil, fmt.Errorf("子网必须是有效的 IPv4 CIDR")
	}
	return ip, net.IP(net.CIDRMask(prefix, 32)).To4(), nil
}

func normalizeDNSZoneName(name string, reverse bool) string {
	name = strings.Trim(strings.TrimSpace(name), ".")
	if !reverse {
		return name
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".in-addr.arpa") || strings.HasSuffix(lower, ".ip6.arpa") {
		return name
	}
	return name + ".in-addr.arpa"
}

func auditResult(ok bool) string {
	if ok {
		return "success"
	}
	return "failed"
}

func (r *Router) ensureAgentNotSyncing(w http.ResponseWriter, req *http.Request, server domain.Server) bool {
	if r.sync.IsAgentSyncRunning(req.Context(), server.ID) {
		writeError(w, http.StatusConflict, "agent_sync_running", "当前 Agent 正在同步，请稍后再操作")
		return false
	}
	return true
}

func dhcpScopeRefreshTarget(serverID, scopeID, scopeExternalID, scopeName string) operationRefreshTarget {
	return operationRefreshTarget{
		Kind:            operationRefreshDHCPScope,
		ServerID:        serverID,
		ScopeID:         scopeID,
		ScopeExternalID: scopeExternalID,
		ScopeName:       scopeName,
	}
}

func (r *Router) markDHCPScopeDirtyAfterAgentFailure(target operationRefreshTarget, err error) {
	r.logger.Warn("Mark DHCP scope dirty after agent operation failure", "server", target.ServerID, "scope", target.ScopeExternalID, "error", err)
	r.refresh.markDirty(target)
}

func filterStateForUser(state domain.State, user domain.User) domain.State {
	canReadDashboard := hasPermission(user, "dashboard.read")
	if !canReadDashboard && !hasPermission(user, "servers.read") {
		state.Servers = nil
	}
	if !canReadDashboard && !hasPermission(user, "dns.read") {
		state.Zones = nil
		state.Records = nil
	}
	if !canReadDashboard && !hasPermission(user, "dhcp.read") {
		state.Scopes = nil
		state.Exclusions = nil
		state.Leases = nil
		state.Reservations = nil
	}
	if !canReadDashboard && !hasPermission(user, "audit.read") {
		state.Audit = nil
	}
	return state
}

func refreshEvent(taskID, status, message string) realtime.RefreshEvent {
	return realtime.RefreshEvent{Type: "runtime.refresh.all", TaskID: taskID, Status: status, Message: message}
}

func (r *Router) isRefreshTargetRunning(ctx context.Context, taskType string, target any) (bool, error) {
	lockKey := syncsvc.RefreshTaskLockKey(taskType, target)
	if lockKey == "" {
		return false, nil
	}
	locked, err := r.realtime.Exists(ctx, lockKey)
	if err != nil {
		r.logger.Warn("Check refresh target lock failed", "type", taskType, "lock", lockKey, "error", err)
		return false, err
	}
	return locked, nil
}

func (r *Router) enqueueZoneRefresh(serverID, zoneID, zoneName, createdBy string) (domain.RefreshTask, error) {
	ctx := context.Background()
	if strings.TrimSpace(zoneID) != "" {
		zone, err := r.store.GetDNSZone(ctx, zoneID)
		if err != nil {
			return domain.RefreshTask{}, err
		}
		serverID = zone.ServerID
		zoneID = zone.ID
		zoneName = zone.Name
	}
	serverName := ""
	if server, err := r.store.GetServer(ctx, serverID); err == nil {
		serverName = server.Name
	}
	target := &syncsvc.ZoneTarget{ServerID: serverID, ServerName: serverName, ZoneID: zoneID, ZoneName: zoneName}
	running, err := r.isRefreshTargetRunning(ctx, syncsvc.RefreshDNSZoneType, target)
	if err != nil {
		return domain.RefreshTask{}, errRuntimeLockUnavailable
	}
	if running {
		return domain.RefreshTask{}, errRefreshTargetRunning
	}
	task, err := r.store.CreateRefreshTask(ctx, syncsvc.RefreshDNSZoneType, map[string]any{
		"message":      "DNS 区域刷新已排队",
		"resourceType": "dns.zone",
		"resourceId":   zoneID,
		"resourceName": zoneName,
		"serverId":     serverID,
		"serverName":   serverName,
	}, createdBy)
	if err != nil {
		return domain.RefreshTask{}, err
	}
	_ = r.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: syncsvc.RefreshDNSZoneType, TaskID: task.ID, Status: "queued", Message: "DNS 区域刷新已排队"})
	go r.sync.RunRefreshTask(context.Background(), task.ID, syncsvc.RefreshDNSZoneType, target)
	return task, nil
}

func (r *Router) enqueueServerRefresh(serverID, serverName, createdBy string, skipHealthCheck bool) (domain.RefreshTask, error) {
	ctx := context.Background()
	task, err := r.store.CreateRefreshTask(ctx, syncsvc.RefreshServerType, map[string]any{
		"message":      "Agent 同步已排队",
		"resourceType": "server",
		"resourceId":   serverID,
		"resourceName": serverName,
		"serverId":     serverID,
		"serverName":   serverName,
	}, createdBy)
	if err != nil {
		return domain.RefreshTask{}, err
	}
	if server, serverErr := r.store.GetServer(ctx, serverID); serverErr == nil {
		r.sync.MarkAgentSyncQueued(ctx, server)
	}
	_ = r.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: syncsvc.RefreshServerType, TaskID: task.ID, Status: "queued", Message: "Agent 同步已排队"})
	go r.sync.RunRefreshTask(context.Background(), task.ID, syncsvc.RefreshServerType, &syncsvc.ServerTarget{ServerID: serverID, ServerName: serverName, SkipHealthCheck: skipHealthCheck})
	return task, nil
}

func (r *Router) enqueueDHCPScopeRefresh(serverID, scopeID, scopeExternalID, scopeName, createdBy string) (domain.RefreshTask, error) {
	ctx := context.Background()
	serverName := ""
	if server, err := r.store.GetServer(ctx, serverID); err == nil {
		serverName = server.Name
	}
	target := &syncsvc.DHCPScopeTarget{ServerID: serverID, ServerName: serverName, ScopeID: scopeID, ScopeExternalID: scopeExternalID, ScopeName: scopeName}
	running, err := r.isRefreshTargetRunning(ctx, syncsvc.RefreshDHCPScopeType, target)
	if err != nil {
		return domain.RefreshTask{}, errRuntimeLockUnavailable
	}
	if running {
		return domain.RefreshTask{}, errRefreshTargetRunning
	}
	task, err := r.store.CreateRefreshTask(ctx, syncsvc.RefreshDHCPScopeType, map[string]any{
		"message":      "DHCP 作用域刷新已排队",
		"resourceType": "dhcp.scope",
		"resourceId":   scopeID,
		"resourceName": scopeName,
		"serverId":     serverID,
		"serverName":   serverName,
	}, createdBy)
	if err != nil {
		r.logger.Warn("Create DHCP scope refresh task failed", "server", serverID, "scope", scopeExternalID, "error", err)
		_ = r.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: "runtime.updated", Status: "success", Message: "运行态已更新"})
		return domain.RefreshTask{}, err
	}
	_ = r.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: syncsvc.RefreshDHCPScopeType, TaskID: task.ID, Status: "queued", Message: "DHCP 作用域刷新已排队"})
	go r.sync.RunRefreshTask(context.Background(), task.ID, syncsvc.RefreshDHCPScopeType, target)
	return task, nil
}
