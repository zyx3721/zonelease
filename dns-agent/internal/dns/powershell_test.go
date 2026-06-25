package dns

import (
	"strings"
	"testing"
)

func TestPTRRecordRelativeName(t *testing.T) {
	if got := ptrRecordRelativeName("0.24.10.in-addr.arpa", "10.24.0.10."); got != "10" {
		t.Fatalf("unexpected relative name: %s", got)
	}
}

func TestPTRRecordRelativeNameKeepsNonMatchingName(t *testing.T) {
	if got := ptrRecordRelativeName("0.24.10.in-addr.arpa", "10.25.0.10."); got != "10.25.0.10" {
		t.Fatalf("unexpected non matching name: %s", got)
	}
}

func TestSyncableZoneNameRejectsTrustAnchors(t *testing.T) {
	for _, name := range []string{"TrustAnchors", "trustanchors", "0.in-addr.arpa", "127.in-addr.arpa", "255.in-addr.arpa"} {
		if syncableZoneName(name) {
			t.Fatalf("expected %s to be filtered", name)
		}
	}
	if !syncableZoneName("test.com") {
		t.Fatal("expected regular zone to be syncable")
	}
}

func TestListZonesScriptFiltersSystemZones(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listZonesScript()
	for _, name := range []string{"trustanchors", "0.in-addr.arpa", "127.in-addr.arpa", "255.in-addr.arpa"} {
		if !strings.Contains(script, name) {
			t.Fatalf("list zones script should filter %s", name)
		}
	}
}

func TestListRecordsScriptKeepsEnumeratingAfterUnsupportedRecords(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listRecordsScript("example.com")
	if !strings.Contains(script, `"SOA"`) || !strings.Contains(script, "Get-RecordDataValue") {
		t.Fatal("list records script should parse SOA records")
	}
	if !strings.Contains(script, `$items = New-Object System.Collections.ArrayList`) {
		t.Fatal("list records script should collect records explicitly")
	}
	if !strings.Contains(script, `"A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }`) {
		t.Fatal("list records script should support alternate A record data properties")
	}
	if strings.Contains(script, `if ([string]::IsNullOrWhiteSpace($value)) { return }`) {
		t.Fatal("list records script must not return from the whole script when one record has no value")
	}
	if !strings.Contains(script, `if (-not [string]::IsNullOrWhiteSpace($value))`) {
		t.Fatal("list records script should skip empty record values without stopping enumeration")
	}
}

func TestListRecordsScriptQuotesZoneName(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listRecordsScript("example'zone.com")
	if !strings.Contains(script, "$zoneName = 'example''zone.com'") {
		t.Fatalf("zone name was not quoted safely in script: %s", script)
	}
}

func TestCreateZoneScriptFallsBackToZoneFileAndVerifiesZone(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.createZoneScript(Zone{Name: "example.com", DynamicUpdate: "None"})
	if !strings.Contains(script, `Add-DnsServerPrimaryZone -Name $name -ReplicationScope "Domain"`) {
		t.Fatal("create zone script should try AD-integrated primary zone first")
	}
	if !strings.Contains(script, `Add-DnsServerPrimaryZone -Name $name -ZoneFile ("$name.dns")`) {
		t.Fatal("create zone script should fall back to file-backed primary zone")
	}
	if !strings.Contains(script, `Get-DnsServerZone -Name $name`) {
		t.Fatal("create zone script should verify the zone after creation")
	}
}

func TestCreateReverseZoneScriptUsesNetworkID(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.createZoneScript(Zone{Name: "1.168.192.in-addr.arpa", Reverse: true})
	if !strings.Contains(script, "$name = '1.168.192.in-addr.arpa'") {
		t.Fatalf("reverse zone name should stay complete for verification: %s", script)
	}
	if !strings.Contains(script, "$networkId = '1.168.192'") {
		t.Fatalf("reverse zone should pass network ID to -NetworkId: %s", script)
	}
}

func TestCreateRecordScriptCreatesAAndPTRInSingleScript(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.createRecordScript("example.com", Record{Name: "www", Type: "A", Value: "10.24.0.12", TTL: 3600, CreatePTR: true})
	for _, expected := range []string{
		"Add-DnsServerResourceRecordA",
		"Get-DnsServerZone -Name $reverseZone",
		"Add-DnsServerResourceRecordPtr",
		"未找到参照的反向查找区域，无法创建 PTR 记录",
		"ConvertTo-Json -InputObject $result",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("create record script should include %q: %s", expected, script)
		}
	}
}

func TestDeleteRecordScriptCanFallbackToUniqueSameNameRecord(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.deleteRecordScript("example.com", Record{Name: "www", Type: "A", Value: "10.24.0.12"}, true)
	if !strings.Contains(script, "$allowUniqueNameFallback = $true") {
		t.Fatalf("delete record script should enable unique name fallback: %s", script)
	}
	if !strings.Contains(script, "$records.Count -eq 1") {
		t.Fatalf("delete record script should only fallback for unique same-name records: %s", script)
	}
}

func TestDeleteRecordUsesUniqueSameNameFallback(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.deleteRecordScript("example.com", Record{Name: "www", Type: "A", Value: "10.24.0.12"}, true)
	if !strings.Contains(script, "$allowUniqueNameFallback = $true") {
		t.Fatalf("delete record script should enable unique name fallback: %s", script)
	}
}

func TestDeleteRecordScriptNormalizesDomainRecordValues(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.deleteRecordScript("0.24.10.in-addr.arpa", Record{Name: "10.24.0.10.", Type: "PTR", Value: "WWW.Test.COM."}, true)
	for _, expected := range []string{
		"function Normalize-DnsRecordValue",
		`"PTR" { return $text.TrimEnd(".").ToLowerInvariant() }`,
		"$recordValueForCompare",
		"$valueForCompare -eq $recordValueForCompare",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("delete record script should normalize domain values with %q: %s", expected, script)
		}
	}
}

func TestDeleteRecordScriptUsesRecordDataFallbackProperties(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.deleteRecordScript("example.com", Record{Name: "www", Type: "A", Value: "10.24.0.12"}, true)
	for _, expected := range []string{
		"function Get-RecordDataValue",
		`"A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }`,
		`"AAAA" { Get-RecordDataValue -Data $recordData -Names @("IPv6Address", "IPAddress") }`,
		`"CNAME" { Get-RecordDataValue -Data $recordData -Names @("HostNameAlias", "DomainName") }`,
		`"PTR" { Get-RecordDataValue -Data $recordData -Names @("PtrDomainName", "DomainName") }`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("delete record script should use robust record data extraction with %q: %s", expected, script)
		}
	}
}

func TestUpdateRecordScriptDeletesAndCreatesInSingleScript(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.updateRecordScript("example.com", RecordUpdate{
		Old: Record{Name: "www", Type: "A", Value: "10.24.0.12", TTL: 3600},
		New: Record{Name: "www", Type: "A", Value: "10.24.0.13", TTL: 3600},
	})
	for _, expected := range []string{
		"Remove-DnsServerResourceRecord",
		"Add-DnsServerResourceRecordA",
		`"A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }`,
		"$recordValueForCompare",
		"$target",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("update record script should update in one PowerShell script with %q: %s", expected, script)
		}
	}
}

func TestReverseZoneNetworkID(t *testing.T) {
	if got := reverseZoneNetworkID("1.168.192.in-addr.arpa"); got != "1.168.192" {
		t.Fatalf("unexpected network ID: %s", got)
	}
	if got := reverseZoneNetworkID("example.com"); got != "example.com" {
		t.Fatalf("unexpected forward zone value: %s", got)
	}
}
