package sync

import (
	"net/http"
	"testing"

	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func TestIsReverseDNSZoneName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "0.24.10.in-addr.arpa", want: true},
		{name: "example.com", want: false},
		{name: "0.0.0.0.0.0.0.0.ip6.arpa", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReverseDNSZoneName(tc.name); got != tc.want {
				t.Fatalf("isReverseDNSZoneName(%q)=%v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestAnnotateDNSPTRRecordsMarksARecord(t *testing.T) {
	serverID := "server-1"
	zoneName := "test.com"
	reverseZoneID := repository.DNSZoneID(serverID, "0.24.10.in-addr.arpa")
	records := []domain.DNSRecord{
		{
			ID:     repository.DNSRecordID(serverID, zoneName, "A", "www", "10.24.0.10"),
			ZoneID: repository.DNSZoneID(serverID, zoneName),
			Name:   "www",
			Type:   "A",
			Value:  "10.24.0.10",
			TTL:    3600,
		},
		{
			ID:     repository.DNSRecordID(serverID, "0.24.10.in-addr.arpa", "PTR", "10.24.0.10.", "www.test.com."),
			ZoneID: reverseZoneID,
			Name:   "10.24.0.10.",
			Type:   "PTR",
			Value:  "www.test.com.",
			TTL:    3600,
		},
	}
	annotated := annotateDNSPTRRecords(serverID, zoneName, records)
	if !annotated[0].CreatePTR {
		t.Fatal("expected A record createPtr to be true")
	}
	if annotated[1].CreatePTR {
		t.Fatal("expected PTR record createPtr to stay false")
	}
}

func TestAnnotateDNSPTRRecordsKeepsARecordFalseWithoutPTR(t *testing.T) {
	serverID := "server-1"
	zoneName := "test.com"
	records := []domain.DNSRecord{
		{
			ID:     repository.DNSRecordID(serverID, zoneName, "A", "www", "10.24.0.10"),
			ZoneID: repository.DNSZoneID(serverID, zoneName),
			Name:   "www",
			Type:   "A",
			Value:  "10.24.0.10",
			TTL:    3600,
		},
	}
	annotated := annotateDNSPTRRecords(serverID, zoneName, records)
	if annotated[0].CreatePTR {
		t.Fatal("expected A record createPtr to stay false")
	}
}

func TestDNSPTRRecordForARecordUsesExpectedReverseZone(t *testing.T) {
	ptr, ok := dnsPTRRecordForARecord("server-1", "test.com", domain.DNSRecord{
		Name:  "www",
		Type:  "A",
		Value: "10.24.0.10",
		TTL:   3600,
	})
	if !ok {
		t.Fatal("expected ptr record")
	}
	if ptr.ZoneID != repository.DNSZoneID("server-1", "0.24.10.in-addr.arpa") {
		t.Fatalf("unexpected zone id: %s", ptr.ZoneID)
	}
	if ptr.Name != "10.24.0.10." || ptr.Value != "www.test.com." {
		t.Fatalf("unexpected ptr record: %+v", ptr)
	}
}

func TestFilterSyncableDNSZonesRejectsTrustAnchors(t *testing.T) {
	zones := filterSyncableDNSZones([]domain.DNSZone{
		{Name: "test.com"},
		{Name: "TrustAnchors"},
		{Name: "trustanchors"},
		{Name: "0.in-addr.arpa"},
		{Name: "127.in-addr.arpa"},
		{Name: "255.in-addr.arpa"},
		{Name: "0.24.10.in-addr.arpa"},
	})
	if len(zones) != 2 {
		t.Fatalf("expected 2 syncable zones, got %d: %+v", len(zones), zones)
	}
	if zones[0].Name != "test.com" || zones[1].Name != "0.24.10.in-addr.arpa" {
		t.Fatalf("unexpected zones: %+v", zones)
	}
}

func TestAgentCanFallbackToPathRecordQueryForLegacyJSONParserError(t *testing.T) {
	err := agent.HTTPStatusError{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Code:       "INTERNAL_ERROR",
		Message:    "JSON request body parsing requires .NET System.Web.Extensions. Install/enable .NET Framework 3.5/4.x on this server.",
	}
	if !agentCanFallbackToPathRecordQuery(err) {
		t.Fatal("expected legacy JSON parser error to allow path query fallback")
	}
}
