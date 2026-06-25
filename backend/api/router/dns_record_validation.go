package router

import (
	"context"
	"fmt"
	"net"
	"strings"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func (r *Router) validateDNSRecordCreate(ctx context.Context, record domain.DNSRecord) error {
	zone, err := r.store.GetDNSZone(ctx, record.ZoneID)
	if err != nil {
		return err
	}
	if zone.Reverse {
		return fmt.Errorf("反向区域不支持在此新建记录")
	}
	records, err := r.store.ListDNSRecordsByZone(ctx, record.ZoneID)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(record.Name)
	recordType := strings.ToUpper(strings.TrimSpace(record.Type))
	value := strings.TrimSpace(record.Value)
	recordLabel := dnsRecordLabel(name, recordType, value)
	for _, existing := range records {
		if !strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			continue
		}
		existingType := strings.ToUpper(strings.TrimSpace(existing.Type))
		if existingType == recordType && strings.EqualFold(strings.TrimSpace(existing.Value), value) {
			if recordType == "CNAME" {
				return fmt.Errorf("%s CNAME 类型记录已存在", name)
			}
			return fmt.Errorf("%s 记录已存在", recordLabel)
		}
		if existingType == "CNAME" && recordType == "CNAME" {
			return fmt.Errorf("%s CNAME 类型记录已存在", name)
		}
		if existingType == "CNAME" && recordType != "CNAME" {
			return fmt.Errorf("%s %s 类型记录无法创建，已存在 CNAME 类型记录", name, recordType)
		}
		if existingType != "CNAME" && recordType == "CNAME" {
			return fmt.Errorf("%s CNAME 类型记录无法创建，已存在其他类型记录", name)
		}
	}
	if recordType == "CNAME" && !isValidCNAMEValue(value) {
		return fmt.Errorf("%s CNAME 类型记录值必须是以 . 结尾的合法域名", name)
	}
	if recordType == "A" && !isValidARecordValue(value) {
		return fmt.Errorf("%s A 类型记录值必须是合法 IPv4 地址", name)
	}
	return nil
}

func (r *Router) dnsRecordCreateWarnings(ctx context.Context, record domain.DNSRecord, serverID string) ([]string, error) {
	if !strings.EqualFold(strings.TrimSpace(record.Type), "A") || !record.CreatePTR {
		return nil, nil
	}
	reverseZone, ok := ipv4ReverseZone(record.Value)
	if !ok {
		return []string{"记录值不是有效 IPv4 地址，无法创建 PTR 记录"}, nil
	}
	exists, err := r.store.DNSReverseZoneExists(ctx, serverID, reverseZone)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []string{"未找到参照的反向查找区域，无法创建 PTR 记录"}, nil
	}
	return nil, nil
}

func (r *Router) validateDNSRecordUpdate(ctx context.Context, oldRecord, newRecord domain.DNSRecord) error {
	if oldRecord.ZoneID != newRecord.ZoneID ||
		!strings.EqualFold(oldRecord.Name, newRecord.Name) ||
		!strings.EqualFold(oldRecord.Type, newRecord.Type) {
		return fmt.Errorf("DNS 记录编辑仅支持修改值")
	}
	name := strings.TrimSpace(oldRecord.Name)
	recordType := strings.ToUpper(strings.TrimSpace(oldRecord.Type))
	oldValue := strings.TrimSpace(oldRecord.Value)
	newValue := strings.TrimSpace(newRecord.Value)
	if strings.EqualFold(oldValue, newValue) && oldRecord.CreatePTR == newRecord.CreatePTR {
		return nil
	}
	if recordType == "CNAME" && !isValidCNAMEValue(newValue) {
		return fmt.Errorf("%s CNAME 类型记录值必须是以 . 结尾的合法域名", name)
	}
	if recordType == "PTR" && !isValidCNAMEValue(newValue) {
		return fmt.Errorf("%s PTR 类型记录值必须是以 . 结尾的合法域名", name)
	}
	if recordType == "A" && !isValidARecordValue(newValue) {
		return fmt.Errorf("%s A 类型记录值必须是合法 IPv4 地址", name)
	}
	records, err := r.store.ListDNSRecordsByZone(ctx, oldRecord.ZoneID)
	if err != nil {
		return err
	}
	for _, existing := range records {
		if existing.ID == oldRecord.ID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			continue
		}
		existingType := strings.ToUpper(strings.TrimSpace(existing.Type))
		if existingType == recordType && strings.EqualFold(strings.TrimSpace(existing.Value), newValue) {
			if recordType == "CNAME" {
				return fmt.Errorf("%s CNAME 类型记录已存在", name)
			}
			return fmt.Errorf("%s 记录已存在", dnsRecordLabel(name, recordType, newValue))
		}
		if recordType == "CNAME" && existingType != "CNAME" {
			return fmt.Errorf("%s CNAME 类型记录无法更新，已存在其他类型记录", name)
		}
		if recordType != "CNAME" && existingType == "CNAME" {
			return fmt.Errorf("%s %s %s 记录无法更新，已存在 CNAME 记录", name, recordType, newValue)
		}
	}
	return nil
}

func dnsRecordLabel(name, recordType, value string) string {
	return strings.TrimSpace(strings.Join([]string{name, recordType, value}, " "))
}

func isValidARecordValue(value string) bool {
	return net.ParseIP(strings.TrimSpace(value)).To4() != nil
}

func isValidCNAMEValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, ".") {
		return false
	}
	name := strings.TrimSuffix(value, ".")
	if name == "" || len(name) > 253 {
		return false
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if !isValidDNSLabel(label) {
			return false
		}
	}
	return true
}

func isValidDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	for i, char := range label {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-'
		if !valid {
			return false
		}
		if (i == 0 || i == len(label)-1) && char == '-' {
			return false
		}
	}
	return true
}

func ipv4ReverseZone(value string) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(value)).To4()
	if ip == nil {
		return "", false
	}
	return fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0]), true
}

func ptrRecordFQDN(zoneName, recordName string) string {
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

func dnsRecordAgentName(zoneName, recordType, recordName string) string {
	name := strings.TrimSpace(recordName)
	if !strings.EqualFold(recordType, "PTR") {
		return name
	}
	zone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zoneName), "."))
	if !strings.HasSuffix(zone, ".in-addr.arpa") {
		return name
	}
	ip := net.ParseIP(strings.TrimSuffix(name, ".")).To4()
	if ip == nil {
		return name
	}
	network := strings.TrimSuffix(zone, ".in-addr.arpa")
	parts := strings.Split(network, ".")
	if len(parts) == 0 || len(parts) >= net.IPv4len {
		return name
	}
	expected := make([]byte, 0, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		value := net.ParseIP("0.0.0." + parts[i]).To4()
		if value == nil {
			return name
		}
		expected = append(expected, value[3])
	}
	for i, value := range expected {
		if ip[i] != value {
			return name
		}
	}
	return fmt.Sprintf("%d", ip[len(expected)])
}

func ptrRecordForARecord(serverID, zoneName string, record domain.DNSRecord) (domain.DNSRecord, bool) {
	reverseZone, ok := ipv4ReverseZone(record.Value)
	if !ok {
		return domain.DNSRecord{}, false
	}
	ptrName := strings.TrimSpace(record.Value) + "."
	value := ptrRecordFQDN(zoneName, record.Name)
	ptr := domain.DNSRecord{
		ID:     repository.DNSRecordID(serverID, reverseZone, "PTR", ptrName, value),
		ZoneID: repository.DNSZoneID(serverID, reverseZone),
		Name:   ptrName,
		Type:   "PTR",
		Value:  value,
		TTL:    record.TTL,
	}
	if ptr.TTL <= 0 {
		ptr.TTL = 3600
	}
	return ptr, true
}
