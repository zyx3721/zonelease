package router

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

type dnsRecordUpdateRequest struct {
	Value     string `json:"value"`
	CreatePTR *bool  `json:"createPtr,omitempty"`
}

type agentRecordPayload struct {
	Zone   string           `json:"zone"`
	Record domain.DNSRecord `json:"record"`
}

type agentRecordUpdatePayload struct {
	Zone   string `json:"zone"`
	Update struct {
		Old domain.DNSRecord `json:"old"`
		New domain.DNSRecord `json:"new"`
	} `json:"update"`
}

func (r *Router) createDNSRecord(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	var body domain.DNSRecord
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.ZoneID) == "" || strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Type) == "" || strings.TrimSpace(body.Value) == "" {
		writeError(w, http.StatusBadRequest, "invalid_record", "记录参数不完整")
		return
	}
	serverID, zoneName, ok := repository.DecodeDNSZoneID(body.ZoneID)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_zone_id", "DNS 区域标识无效")
		return
	}
	server, err := r.store.GetServer(req.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	primaryRefresh := dnsZoneRefreshTarget(serverID, body.ZoneID, zoneName)
	finishPrimary := r.refresh.begin(primaryRefresh)
	defer finishPrimary()
	if err := r.validateDNSRecordCreate(req.Context(), body); err != nil {
		writeError(w, http.StatusConflict, "dns_record_conflict", err.Error())
		return
	}
	preflightWarnings, err := r.dnsRecordCreateWarnings(req.Context(), body, serverID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dns_record_warning_failed", "检查 DNS 记录警告失败")
		return
	}
	var agentResult struct {
		Created  bool              `json:"created"`
		PTR      *domain.DNSRecord `json:"ptr,omitempty"`
		Warnings []string          `json:"warnings"`
	}
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	agentBody := body
	if len(preflightWarnings) > 0 {
		agentBody.CreatePTR = false
	}
	if err := r.createDNSRecordOnAgent(agentCtx, server, zoneName, agentBody, &agentResult); err != nil {
		writeError(w, http.StatusBadGateway, "agent_create_record_failed", "Agent 创建 DNS 记录失败："+err.Error())
		return
	}
	agentResult.Warnings = normalizeDNSRecordAgentWarnings(agentResult.Warnings)
	body.ID = repository.DNSRecordID(serverID, zoneName, body.Type, body.Name, body.Value)
	body, _ = r.store.UpsertDNSRecord(req.Context(), body)
	relatedRecords := []domain.DNSRecord{}
	if len(preflightWarnings) == 0 && len(agentResult.Warnings) == 0 && strings.EqualFold(body.Type, "A") && body.CreatePTR {
		if ptrRecord, ok := ptrRecordForARecord(serverID, zoneName, body); ok {
			ptrRecord, _ = r.store.UpsertDNSRecord(req.Context(), ptrRecord)
			relatedRecords = append(relatedRecords, ptrRecord)
		}
	}
	r.writeAudit(req, "Created DNS record", body.Name, "DNS", "success", map[string]any{
		"zone":   zoneName,
		"record": body.Name,
		"type":   body.Type,
		"value":  body.Value,
		"ttl":    body.TTL,
	})
	r.refresh.markDirty(primaryRefresh)
	for _, related := range relatedRecords {
		if _, relatedZoneName, ok := repository.DecodeDNSZoneID(related.ZoneID); ok {
			r.refresh.markDirty(dnsZoneRefreshTarget(serverID, related.ZoneID, relatedZoneName))
		}
	}
	warnings := append(preflightWarnings, agentResult.Warnings...)
	writeJSON(w, http.StatusCreated, domain.DNSRecordCreateResponse{DNSRecord: body, RelatedRecords: relatedRecords, Warnings: warnings})
}

func (r *Router) deleteDNSRecord(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dns/records/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	serverID, zoneName, recordType, recordName, recordValue, ok := repository.DecodeDNSRecordID(id)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_record_id", "DNS 记录标识无效")
		return
	}
	if !dnsRecordManageableInZone(zoneName, recordType) {
		writeError(w, http.StatusBadRequest, "unsupported_record_type", dnsRecordManageRestrictionMessage(zoneName, "删除"))
		return
	}
	server, err := r.store.GetServer(req.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	primaryRefresh := dnsZoneRefreshTarget(serverID, repository.DNSZoneID(serverID, zoneName), zoneName)
	finishPrimary := r.refresh.begin(primaryRefresh)
	defer finishPrimary()
	oldRecord, err := r.store.GetDNSRecord(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "record_not_found", "DNS 记录不存在")
		return
	}
	deleteRelatedPTR := false
	relatedPTR, hasRelatedPTR := r.relatedPTRRecordForDNSRecord(req.Context(), serverID, zoneName, oldRecord)
	if strings.EqualFold(recordType, "A") && (oldRecord.CreatePTR || hasRelatedPTR) {
		deleteRelatedPTR = true
	}
	agentRecord := domain.DNSRecord{
		Name:      dnsRecordAgentName(zoneName, recordType, recordName),
		Type:      recordType,
		Value:     recordValue,
		TTL:       oldRecord.TTL,
		CreatePTR: deleteRelatedPTR,
	}
	var ignored map[string]any
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	if err := r.deleteDNSRecordOnAgent(agentCtx, server, zoneName, agentRecord, &ignored); err != nil {
		writeError(w, http.StatusBadGateway, "agent_delete_record_failed", "Agent 删除 DNS 记录失败："+err.Error())
		return
	}
	r.writeAudit(req, "Deleted DNS record", recordName, "DNS", "success", map[string]any{
		"zone":   zoneName,
		"record": recordName,
		"type":   recordType,
		"value":  recordValue,
	})
	_ = r.store.DeleteDNSRecord(req.Context(), id)
	if strings.EqualFold(recordType, "A") {
		ptrRecord := relatedPTR
		reverseZoneExists := false
		if !hasRelatedPTR && oldRecord.CreatePTR {
			if expectedPTR, ok := ptrRecordForARecord(serverID, zoneName, oldRecord); ok {
				ptrRecord = expectedPTR
				if _, relatedZoneName, ok := repository.DecodeDNSZoneID(expectedPTR.ZoneID); ok {
					exists, err := r.store.DNSReverseZoneExists(req.Context(), serverID, relatedZoneName)
					reverseZoneExists = err == nil && exists
				}
			}
		}
		if hasRelatedPTR {
			_ = r.store.DeleteDNSRecord(req.Context(), ptrRecord.ID)
		}
		if shouldRefreshDeletedARecordPTRZone(hasRelatedPTR, oldRecord.CreatePTR, reverseZoneExists) {
			if _, relatedZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
				r.refresh.markDirty(dnsZoneRefreshTarget(serverID, ptrRecord.ZoneID, relatedZoneName))
			}
		}
	}
	r.refresh.markDirty(primaryRefresh)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) updateDNSRecord(w http.ResponseWriter, req *http.Request) {
	if !r.ensurePermission(w, req, "dns.manage") {
		return
	}
	id := pathID(req.URL.Path, "/api/dns/records/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	serverID, zoneName, recordType, recordName, _, ok := repository.DecodeDNSRecordID(id)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_record_id", "DNS 记录标识无效")
		return
	}
	if !dnsRecordManageableInZone(zoneName, recordType) {
		writeError(w, http.StatusBadRequest, "unsupported_record_type", dnsRecordManageRestrictionMessage(zoneName, "编辑"))
		return
	}
	var body dnsRecordUpdateRequest
	if !decode(w, req, &body) {
		return
	}
	newValue := strings.TrimSpace(body.Value)
	if newValue == "" {
		writeError(w, http.StatusBadRequest, "invalid_record", "记录值不能为空")
		return
	}
	server, err := r.store.GetServer(req.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "服务器不存在")
		return
	}
	primaryRefresh := dnsZoneRefreshTarget(serverID, repository.DNSZoneID(serverID, zoneName), zoneName)
	finishPrimary := r.refresh.begin(primaryRefresh)
	defer finishPrimary()
	oldRecord, err := r.store.GetDNSRecord(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "record_not_found", "DNS 记录不存在")
		return
	}
	newRecord := oldRecord
	newRecord.Value = newValue
	if body.CreatePTR != nil && (strings.EqualFold(recordType, "A") || strings.EqualFold(recordType, "AAAA")) {
		newRecord.CreatePTR = *body.CreatePTR
	}
	newRecord.ID = repository.DNSRecordID(serverID, zoneName, recordType, recordName, newValue)
	if err := r.validateDNSRecordUpdate(req.Context(), oldRecord, newRecord); err != nil {
		writeError(w, http.StatusConflict, "dns_record_conflict", err.Error())
		return
	}
	valueChanged := !strings.EqualFold(strings.TrimSpace(oldRecord.Value), strings.TrimSpace(newRecord.Value))
	ptrChanged := oldRecord.CreatePTR != newRecord.CreatePTR
	agentCtx, cancel := context.WithTimeout(req.Context(), r.agentOperationTimeout(req.Context()))
	defer cancel()
	var ignored map[string]any
	shouldDeleteOldPtr := strings.EqualFold(recordType, "A") && oldRecord.CreatePTR
	shouldCreateNewPtr := strings.EqualFold(recordType, "A") && newRecord.CreatePTR
	ptrWarnings, err := r.dnsRecordCreateWarnings(req.Context(), newRecord, serverID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dns_record_warning_failed", "检查 DNS 记录警告失败")
		return
	}
	if len(ptrWarnings) > 0 {
		shouldCreateNewPtr = false
	}
	var agentResult struct {
		Created  bool              `json:"created"`
		PTR      *domain.DNSRecord `json:"ptr,omitempty"`
		Warnings []string          `json:"warnings"`
	}
	if valueChanged {
		agentRecordName := dnsRecordAgentName(zoneName, recordType, recordName)
		agentOldRecord := oldRecord
		agentOldRecord.Name = agentRecordName
		agentOldRecord.CreatePTR = shouldDeleteOldPtr
		agentNewRecord := newRecord
		agentNewRecord.Name = agentRecordName
		agentNewRecord.CreatePTR = shouldCreateNewPtr
		if err := r.updateDNSRecordOnAgent(agentCtx, server, zoneName, agentOldRecord, agentNewRecord, &agentResult); err != nil {
			writeError(w, http.StatusBadGateway, "agent_update_record_failed", "Agent 更新 DNS 记录失败："+err.Error())
			return
		}
		agentResult.Warnings = normalizeDNSRecordAgentWarnings(agentResult.Warnings)
	}
	if !valueChanged && ptrChanged && shouldCreateNewPtr {
		if ptrRecord, ok := ptrRecordForARecord(serverID, zoneName, newRecord); ok {
			if _, reverseZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
				relatedRefresh := dnsZoneRefreshTarget(serverID, ptrRecord.ZoneID, reverseZoneName)
				finishRelated := r.refresh.begin(relatedRefresh)
				defer finishRelated()
				var ptrResult map[string]any
				ptrAgentRecord := ptrRecord
				ptrAgentRecord.Name = dnsRecordAgentName(reverseZoneName, "PTR", ptrRecord.Name)
				if err := r.createDNSRecordOnAgent(agentCtx, server, reverseZoneName, ptrAgentRecord, &ptrResult); err != nil {
					agentResult.Warnings = append(agentResult.Warnings, "PTR 记录创建失败："+err.Error())
				}
			}
		}
	}
	if !valueChanged && ptrChanged && shouldDeleteOldPtr {
		if ptrRecord, ok := ptrRecordForARecord(serverID, zoneName, oldRecord); ok {
			if _, reverseZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
				relatedRefresh := dnsZoneRefreshTarget(serverID, ptrRecord.ZoneID, reverseZoneName)
				finishRelated := r.refresh.begin(relatedRefresh)
				defer finishRelated()
				ptrAgentRecord := ptrRecord
				ptrAgentRecord.Name = dnsRecordAgentName(reverseZoneName, "PTR", ptrRecord.Name)
				if err := r.deleteDNSRecordOnAgent(agentCtx, server, reverseZoneName, ptrAgentRecord, &ignored); err != nil {
					agentResult.Warnings = append(agentResult.Warnings, "PTR 记录删除失败："+err.Error())
				}
			}
		}
	}
	reverseRefreshZones := map[string]string{}
	if shouldDeleteOldPtr && (valueChanged || ptrChanged) {
		if ptrRecord, ok := ptrRecordForARecord(serverID, zoneName, oldRecord); ok {
			_ = r.store.DeleteDNSRecord(req.Context(), ptrRecord.ID)
			if _, relatedZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
				reverseRefreshZones[ptrRecord.ZoneID] = relatedZoneName
			}
		}
	}
	if valueChanged {
		_ = r.store.DeleteDNSRecord(req.Context(), oldRecord.ID)
	}
	newRecord, _ = r.store.UpsertDNSRecord(req.Context(), newRecord)
	relatedRecords := []domain.DNSRecord{}
	if len(agentResult.Warnings) == 0 && shouldCreateNewPtr {
		if ptrRecord, ok := ptrRecordForARecord(serverID, zoneName, newRecord); ok {
			ptrRecord, _ = r.store.UpsertDNSRecord(req.Context(), ptrRecord)
			relatedRecords = append(relatedRecords, ptrRecord)
			if _, relatedZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
				reverseRefreshZones[ptrRecord.ZoneID] = relatedZoneName
			}
		}
	}
	r.writeAudit(req, "Updated DNS record", recordName, "DNS", "success", map[string]any{
		"zone":     zoneName,
		"record":   recordName,
		"type":     recordType,
		"oldValue": oldRecord.Value,
		"newValue": newValue,
	})
	r.refresh.markDirty(primaryRefresh)
	for zoneID, relatedZoneName := range reverseRefreshZones {
		r.refresh.markDirty(dnsZoneRefreshTarget(serverID, zoneID, relatedZoneName))
	}
	warnings := append(ptrWarnings, agentResult.Warnings...)
	writeJSON(w, http.StatusOK, domain.DNSRecordCreateResponse{DNSRecord: newRecord, RelatedRecords: relatedRecords, Warnings: warnings})
}

func dnsZoneRefreshTarget(serverID, zoneID, zoneName string) operationRefreshTarget {
	return operationRefreshTarget{
		Kind:     operationRefreshDNSZone,
		ServerID: serverID,
		ZoneID:   zoneID,
		ZoneName: zoneName,
	}
}

func normalizeDNSRecordAgentWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return warnings
	}
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		switch strings.TrimSpace(warning) {
		case "PTR_REVERSE_ZONE_NOT_FOUND":
			result = append(result, "未找到参照的反向查找区域，无法创建 PTR 记录")
		default:
			result = append(result, warning)
		}
	}
	return result
}

func shouldRefreshDeletedARecordPTRZone(hasRelatedPTR, createPTR, reverseZoneExists bool) bool {
	return hasRelatedPTR || (createPTR && reverseZoneExists)
}

func (r *Router) relatedPTRRecordForDNSRecord(ctx context.Context, serverID, zoneName string, record domain.DNSRecord) (domain.DNSRecord, bool) {
	expected, ok := ptrRecordForARecord(serverID, zoneName, record)
	if !ok {
		return domain.DNSRecord{}, false
	}
	records, err := r.store.ListDNSRecordsByZone(ctx, expected.ZoneID)
	if err != nil {
		return domain.DNSRecord{}, false
	}
	for _, item := range records {
		if !strings.EqualFold(strings.TrimSpace(item.Type), "PTR") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(expected.Name)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Value), strings.TrimSpace(expected.Value)) {
			continue
		}
		return item, true
	}
	return domain.DNSRecord{}, false
}

func (r *Router) createDNSRecordOnAgent(ctx context.Context, server domain.Server, zoneName string, record domain.DNSRecord, dst any) error {
	body := agentRecordPayload{Zone: zoneName, Record: record}
	if err := r.agent.Post(ctx, server.AgentURL, server.APIKey, "/dns/records/create", body, dst, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			return err
		}
		path := "/dns/zones/" + url.PathEscape(zoneName) + "/records"
		return r.agent.Post(ctx, server.AgentURL, server.APIKey, path, record, dst, server.TLSInsecure)
	}
	return nil
}

func (r *Router) deleteDNSRecordOnAgent(ctx context.Context, server domain.Server, zoneName string, record domain.DNSRecord, dst any) error {
	body := agentRecordPayload{Zone: zoneName, Record: record}
	if err := r.agent.Post(ctx, server.AgentURL, server.APIKey, "/dns/records/delete", body, dst, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			return err
		}
		path := "/dns/zones/" + url.PathEscape(zoneName) + "/records/" + url.PathEscape(record.Type) + "/" + url.PathEscape(record.Name) + "?value=" + url.QueryEscape(record.Value)
		if record.CreatePTR {
			path += "&createPtr=true"
		}
		return r.agent.Delete(ctx, server.AgentURL, server.APIKey, path, dst, server.TLSInsecure)
	}
	return nil
}

func (r *Router) updateDNSRecordOnAgent(ctx context.Context, server domain.Server, zoneName string, oldRecord, newRecord domain.DNSRecord, dst any) error {
	body := agentRecordUpdatePayload{Zone: zoneName}
	body.Update.Old = oldRecord
	body.Update.New = newRecord
	if err := r.agent.Post(ctx, server.AgentURL, server.APIKey, "/dns/records/update", body, dst, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			return err
		}
		var ignored map[string]any
		if err := r.deleteDNSRecordOnAgent(ctx, server, zoneName, oldRecord, &ignored); err != nil {
			return err
		}
		if err := r.createDNSRecordOnAgent(ctx, server, zoneName, newRecord, dst); err != nil {
			_ = r.createDNSRecordOnAgent(context.Background(), server, zoneName, oldRecord, &ignored)
			return err
		}
	}
	return nil
}

func (r *Router) fetchDNSRecordsFromAgent(ctx context.Context, server domain.Server, zoneName string) ([]domain.DNSRecord, error) {
	var records []domain.DNSRecord
	if err := r.agent.Post(ctx, server.AgentURL, server.APIKey, "/dns/records/query", map[string]string{"zone": zoneName}, &records, server.TLSInsecure); err != nil {
		if !agentReturnedNotFound(err) {
			return nil, err
		}
		path := "/dns/zones/" + url.PathEscape(zoneName) + "/records"
		if err := r.agent.Get(ctx, server.AgentURL, server.APIKey, path, &records, server.TLSInsecure); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func agentReturnedNotFound(err error) bool {
	var statusErr agent.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound
}

func dnsRecordManageableInZone(zoneName, recordType string) bool {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))
	if dnsZoneNameIsReverse(zoneName) {
		return recordType == "PTR"
	}
	return recordType == "A" || recordType == "CNAME"
}

func dnsRecordManageRestrictionMessage(zoneName, action string) string {
	if dnsZoneNameIsReverse(zoneName) {
		return "反向区域仅支持" + action + " PTR 记录"
	}
	return "正向区域仅支持" + action + " A 和 CNAME 记录"
}

func dnsZoneNameIsReverse(zoneName string) bool {
	return strings.HasSuffix(strings.ToLower(strings.Trim(strings.TrimSpace(zoneName), ".")), ".in-addr.arpa")
}
