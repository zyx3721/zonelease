package sync

import (
	"strings"

	"zonelease/backend/internal/domain"
)

func filterSyncableDNSZones(zones []domain.DNSZone) []domain.DNSZone {
	filtered := zones[:0]
	for _, zone := range zones {
		if syncableDNSZoneName(zone.Name) {
			filtered = append(filtered, zone)
		}
	}
	return filtered
}

func syncableDNSZoneName(name string) bool {
	value := strings.TrimSpace(name)
	if value == "" {
		return false
	}
	return !isSystemDNSZone(value)
}

func isSystemDNSZone(name string) bool {
	value := strings.Trim(strings.TrimSpace(name), ".")
	switch strings.ToLower(value) {
	case "trustanchors", "0.in-addr.arpa", "127.in-addr.arpa", "255.in-addr.arpa":
		return true
	default:
		return false
	}
}
