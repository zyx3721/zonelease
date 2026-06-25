package router

import (
	"testing"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func TestIPv4ReverseZone(t *testing.T) {
	zone, ok := ipv4ReverseZone("10.10.10.10")
	if !ok {
		t.Fatal("expected IPv4 reverse zone")
	}
	if zone != "10.10.10.in-addr.arpa" {
		t.Fatalf("unexpected reverse zone: %s", zone)
	}
}

func TestIPv4ReverseZoneRejectsInvalidIP(t *testing.T) {
	if zone, ok := ipv4ReverseZone("not-an-ip"); ok || zone != "" {
		t.Fatalf("expected invalid IP to be rejected, got zone=%q ok=%v", zone, ok)
	}
}

func TestDNSRecordLabel(t *testing.T) {
	label := dnsRecordLabel("www", "A", "10.10.10.10")
	if label != "www A 10.10.10.10" {
		t.Fatalf("unexpected label: %s", label)
	}
}

func TestPTRRecordForARecord(t *testing.T) {
	record := domain.DNSRecord{Name: "www", Type: "A", Value: "10.24.0.10", TTL: 3600}
	ptr, ok := ptrRecordForARecord("server-1", "test.com", record)
	if !ok {
		t.Fatal("expected PTR record")
	}
	if ptr.ZoneID != repository.DNSZoneID("server-1", "0.24.10.in-addr.arpa") {
		t.Fatalf("unexpected zone id: %s", ptr.ZoneID)
	}
	if ptr.Name != "10.24.0.10." {
		t.Fatalf("unexpected ptr name: %s", ptr.Name)
	}
	if ptr.Value != "www.test.com." {
		t.Fatalf("unexpected ptr value: %s", ptr.Value)
	}
	if ptr.Type != "PTR" || ptr.TTL != 3600 {
		t.Fatalf("unexpected ptr record: %+v", ptr)
	}
}

func TestPTRRecordFQDNForRootRecord(t *testing.T) {
	if got := ptrRecordFQDN("test.com", "@"); got != "test.com." {
		t.Fatalf("unexpected fqdn: %s", got)
	}
}

func TestValidateDNSRecordUpdateAllowsNoChange(t *testing.T) {
	router := &Router{}
	record := domain.DNSRecord{
		ID:        "record-1",
		ZoneID:    "zone-1",
		Name:      "www",
		Type:      "A",
		Value:     "10.10.10.10",
		TTL:       3600,
		CreatePTR: true,
	}
	if err := router.validateDNSRecordUpdate(t.Context(), record, record); err != nil {
		t.Fatalf("expected no-change update to pass, got %v", err)
	}
}

func TestDNSRecordAgentNameConvertsReversePTRDisplayName(t *testing.T) {
	if got := dnsRecordAgentName("0.24.10.in-addr.arpa", "PTR", "10.24.0.10."); got != "10" {
		t.Fatalf("unexpected agent name: %s", got)
	}
}

func TestDNSRecordAgentNameKeepsNonMatchingReversePTRName(t *testing.T) {
	if got := dnsRecordAgentName("0.24.10.in-addr.arpa", "PTR", "10.25.0.10."); got != "10.25.0.10." {
		t.Fatalf("unexpected agent name: %s", got)
	}
}

func TestDNSRecordAgentNameKeepsForwardRecordName(t *testing.T) {
	if got := dnsRecordAgentName("test.com", "A", "www"); got != "www" {
		t.Fatalf("unexpected agent name: %s", got)
	}
}

func TestIsValidARecordValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "ipv4", value: "10.10.10.10", want: true},
		{name: "domain", value: "example.com", want: false},
		{name: "ipv6", value: "2001:db8::1", want: false},
		{name: "out of range", value: "999.10.10.10", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidARecordValue(tc.value); got != tc.want {
				t.Fatalf("isValidARecordValue(%q)=%v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestIsValidCNAMEValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "fqdn", value: "target.example.com.", want: true},
		{name: "missing trailing dot", value: "target.example.com", want: false},
		{name: "ip address", value: "10.10.10.10", want: false},
		{name: "invalid label edge", value: "-target.example.com.", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidCNAMEValue(tc.value); got != tc.want {
				t.Fatalf("isValidCNAMEValue(%q)=%v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestDNSRecordManageableInReverseZoneAllowsOnlyPTR(t *testing.T) {
	if !dnsRecordManageableInZone("0.24.10.in-addr.arpa", "PTR") {
		t.Fatal("expected PTR to be manageable in reverse zone")
	}
	if dnsRecordManageableInZone("0.24.10.in-addr.arpa", "A") {
		t.Fatal("expected A to be blocked in reverse zone")
	}
}

func TestDNSRecordManageableInForwardZoneAllowsAAndCNAME(t *testing.T) {
	if !dnsRecordManageableInZone("example.com", "A") {
		t.Fatal("expected A to be manageable in forward zone")
	}
	if !dnsRecordManageableInZone("example.com", "CNAME") {
		t.Fatal("expected CNAME to be manageable in forward zone")
	}
	if dnsRecordManageableInZone("example.com", "PTR") {
		t.Fatal("expected PTR to be blocked in forward zone")
	}
}

func TestShouldRefreshDeletedARecordPTRZone(t *testing.T) {
	tests := []struct {
		name              string
		hasRelatedPTR     bool
		createPTR         bool
		reverseZoneExists bool
		want              bool
	}{
		{
			name:          "actual ptr snapshot exists",
			hasRelatedPTR: true,
			want:          true,
		},
		{
			name:              "marked ptr and reverse zone exists",
			createPTR:         true,
			reverseZoneExists: true,
			want:              true,
		},
		{
			name:      "marked ptr but reverse zone missing",
			createPTR: true,
			want:      false,
		},
		{
			name: "no ptr marker and no snapshot",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRefreshDeletedARecordPTRZone(tt.hasRelatedPTR, tt.createPTR, tt.reverseZoneExists)
			if got != tt.want {
				t.Fatalf("shouldRefreshDeletedARecordPTRZone()=%v, want %v", got, tt.want)
			}
		})
	}
}
