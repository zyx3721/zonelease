package sync

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func annotateDNSPTRRecords(serverID, zoneName string, records []domain.DNSRecord) []domain.DNSRecord {
	result := make([]domain.DNSRecord, len(records))
	copy(result, records)
	ptrKeys := map[string]bool{}
	for _, record := range result {
		if !strings.EqualFold(strings.TrimSpace(record.Type), "PTR") {
			continue
		}
		ptrKeys[dnsPTRMatchKey(record.ZoneID, record.Name, record.Value)] = true
	}
	for i := range result {
		if !strings.EqualFold(strings.TrimSpace(result[i].Type), "A") {
			continue
		}
		ptrRecord, ok := dnsPTRRecordForARecord(serverID, zoneName, result[i])
		if !ok {
			continue
		}
		result[i].CreatePTR = ptrKeys[dnsPTRMatchKey(ptrRecord.ZoneID, ptrRecord.Name, ptrRecord.Value)]
	}
	return result
}

func dnsPTRRecordForARecord(serverID, zoneName string, record domain.DNSRecord) (domain.DNSRecord, bool) {
	ip := net.ParseIP(strings.TrimSpace(record.Value)).To4()
	if ip == nil {
		return domain.DNSRecord{}, false
	}
	reverseZone := fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
	value := dnsPTRRecordFQDN(zoneName, record.Name)
	return domain.DNSRecord{
		ID:     repository.DNSRecordID(serverID, reverseZone, "PTR", strings.TrimSpace(record.Value)+".", value),
		ZoneID: repository.DNSZoneID(serverID, reverseZone),
		Name:   strings.TrimSpace(record.Value) + ".",
		Type:   "PTR",
		Value:  value,
		TTL:    record.TTL,
	}, true
}

func dnsPTRRecordFQDN(zoneName, recordName string) string {
	name := strings.TrimSpace(recordName)
	zone := strings.Trim(strings.TrimSpace(zoneName), ".")
	var fqdn string
	if name == "" || name == "@" {
		fqdn = zone
	} else {
		fqdn = strings.Trim(name, ".") + "." + zone
	}
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	return fqdn
}

func dnsPTRMatchKey(zoneID, name, value string) string {
	return strings.ToLower(strings.TrimSpace(zoneID)) + "|" + strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) annotateDNSRecordsFromReverseZones(ctx context.Context, server domain.Server, zoneName string, records []domain.DNSRecord) []domain.DNSRecord {
	reverseZones := map[string]struct{}{}
	for _, record := range records {
		ptrRecord, ok := dnsPTRRecordForARecord(server.ID, zoneName, record)
		if !ok {
			continue
		}
		if _, reverseZoneName, ok := repository.DecodeDNSZoneID(ptrRecord.ZoneID); ok {
			reverseZones[reverseZoneName] = struct{}{}
		}
	}
	reverseZones = s.existingDNSReverseZones(ctx, server.ID, reverseZones)
	if len(reverseZones) == 0 {
		return records
	}
	combined := make([]domain.DNSRecord, 0, len(records))
	combined = append(combined, records...)
	for reverseZoneName := range reverseZones {
		reverseZone := domain.DNSZone{
			ID:       repository.DNSZoneID(server.ID, reverseZoneName),
			Name:     reverseZoneName,
			Type:     "Primary",
			Reverse:  true,
			ServerID: server.ID,
		}
		reverseRecords, err := s.fetchDNSZoneRecords(ctx, server, reverseZone)
		if err != nil {
			continue
		}
		_ = s.store.ReplaceDNSZoneRecords(ctx, reverseZone, reverseRecords)
		combined = append(combined, reverseRecords...)
	}
	return annotateDNSPTRRecords(server.ID, zoneName, combined)[:len(records)]
}

func (s *Service) existingDNSReverseZones(ctx context.Context, serverID string, candidates map[string]struct{}) map[string]struct{} {
	if len(candidates) == 0 {
		return candidates
	}
	existing := map[string]struct{}{}
	zones, err := s.store.ListDNSZones(ctx)
	if err != nil {
		return existing
	}
	for _, zone := range zones {
		if zone.ServerID != serverID || !zone.Reverse {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(zone.Name))
		if _, ok := candidates[name]; ok {
			existing[name] = struct{}{}
		}
	}
	return existing
}

func (s *Service) fetchDNSZoneRecords(ctx context.Context, server domain.Server, zone domain.DNSZone) ([]domain.DNSRecord, error) {
	if zone.ID == "" {
		zone.ID = repository.DNSZoneID(server.ID, zone.Name)
	}
	zone.ServerID = server.ID
	var records []domain.DNSRecord
	if err := s.fetchDNSRecordsFromAgent(ctx, server, zone.Name, &records); err != nil {
		_ = s.store.MarkDNSZoneSyncError(ctx, zone.ID, err.Error())
		return nil, err
	}
	for i := range records {
		records[i].ZoneID = zone.ID
		records[i].ID = repository.DNSRecordID(server.ID, zone.Name, records[i].Type, records[i].Name, records[i].Value)
	}
	return records, nil
}

func (s *Service) fetchDNSRecordsFromAgent(ctx context.Context, server domain.Server, zoneName string, records *[]domain.DNSRecord) error {
	queryPath := "/dns/records/query"
	body := map[string]string{"zone": zoneName}
	if err := s.agent.Post(ctx, server.AgentURL, server.APIKey, queryPath, body, records, server.TLSInsecure); err != nil {
		if !agentCanFallbackToPathRecordQuery(err) {
			return err
		}
		path := "/dns/zones/" + url.PathEscape(zoneName) + "/records"
		return s.agent.Get(ctx, server.AgentURL, server.APIKey, path, records, server.TLSInsecure)
	}
	return nil
}

func agentCanFallbackToPathRecordQuery(err error) bool {
	return agentReturnedNotFound(err) || agentReturnedLegacyJSONParserUnavailable(err)
}

func agentReturnedNotFound(err error) bool {
	var statusErr agent.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound
}

func agentReturnedLegacyJSONParserUnavailable(err error) bool {
	var statusErr agent.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	if statusErr.StatusCode != http.StatusInternalServerError {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(statusErr.Message))
	return strings.Contains(message, "system.web.extensions") &&
		strings.Contains(message, "json request body parsing")
}
