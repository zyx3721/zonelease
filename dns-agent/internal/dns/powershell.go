package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

type PowerShellProvider struct{}

var powerShellTimeoutSeconds int64 = 180

func NewPowerShellProvider() *PowerShellProvider {
	return &PowerShellProvider{}
}

func NewPowerShellProviderWithTimeout(timeout time.Duration) *PowerShellProvider {
	if timeout > 0 {
		setPowerShellTimeout(timeout)
	}
	return &PowerShellProvider{}
}

func (p *PowerShellProvider) ListZones(ctx context.Context) ([]Zone, error) {
	script := p.listZonesScript()
	var zones []Zone
	return zones, runJSON(ctx, script, &zones)
}

func (p *PowerShellProvider) listZonesScript() string {
	return `
Import-Module DnsServer -ErrorAction Stop
$systemZones = @("trustanchors", "0.in-addr.arpa", "127.in-addr.arpa", "255.in-addr.arpa")
$items = @(Get-DnsServerZone | Where-Object { $_.ZoneType -in @("Primary","Secondary","Stub") -and -not $systemZones.Contains(([string]$_.ZoneName).Trim(".").ToLowerInvariant()) } | ForEach-Object {
  [PSCustomObject]@{
    id = $_.ZoneName
    name = $_.ZoneName
    type = [string]$_.ZoneType
    reverse = [bool]$_.IsReverseLookupZone
    dynamicUpdate = if ($_.DynamicUpdate -eq "NonsecureAndSecure") { "Nonsecure" } else { [string]$_.DynamicUpdate }
    serverId = "local"
  }
})
ConvertTo-Json -InputObject $items -Depth 8
`
}

func (p *PowerShellProvider) CreateZone(ctx context.Context, zone Zone) error {
	if strings.TrimSpace(zone.Name) == "" {
		return fmt.Errorf("zone name is required")
	}
	return run(ctx, p.createZoneScript(zone))
}

func (p *PowerShellProvider) createZoneScript(zone Zone) string {
	dynamicUpdate := zone.DynamicUpdate
	if dynamicUpdate == "" {
		dynamicUpdate = "None"
	}
	zoneName := strings.Trim(strings.TrimSpace(zone.Name), ".")
	networkID := reverseZoneNetworkID(zoneName)
	script := fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$name = %s
$networkId = %s
$dynamicUpdate = %s
if ($dynamicUpdate -eq "Nonsecure") { $dynamicUpdate = "NonsecureAndSecure" }
if (%s) {
  try {
    Add-DnsServerPrimaryZone -NetworkId $networkId -ReplicationScope "Domain" -DynamicUpdate $dynamicUpdate -ErrorAction Stop | Out-Null
  } catch {
    Add-DnsServerPrimaryZone -NetworkId $networkId -ZoneFile ("$name.dns") -DynamicUpdate $dynamicUpdate -ErrorAction Stop | Out-Null
  }
} else {
  try {
    Add-DnsServerPrimaryZone -Name $name -ReplicationScope "Domain" -DynamicUpdate $dynamicUpdate -ErrorAction Stop | Out-Null
  } catch {
    Add-DnsServerPrimaryZone -Name $name -ZoneFile ("$name.dns") -DynamicUpdate $dynamicUpdate -ErrorAction Stop | Out-Null
  }
}
$created = Get-DnsServerZone -Name $name -ErrorAction SilentlyContinue
if ($null -eq $created) {
  throw ("DNS zone was not found after creation: " + $name)
}
`, psString(zoneName), psString(networkID), psString(dynamicUpdate), psBool(zone.Reverse))
	return script
}

func (p *PowerShellProvider) DeleteZone(ctx context.Context, name string) error {
	return run(ctx, fmt.Sprintf(`Import-Module DnsServer -ErrorAction Stop
Remove-DnsServerZone -Name %s -Force -ErrorAction Stop`, psString(name)))
}

func (p *PowerShellProvider) ListRecords(ctx context.Context, zone string) ([]Record, error) {
	script := p.listRecordsScript(zone)
	var records []Record
	return records, runJSON(ctx, script, &records)
}

func (p *PowerShellProvider) listRecordsScript(zone string) string {
	return fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$zoneName = %s
$now = (Get-Date).ToString("o")
function Get-RecordDataValue {
  param([object]$Data, [string[]]$Names)
  foreach ($name in $Names) {
    if ($null -eq $Data) { continue }
    $property = $Data.PSObject.Properties[$name]
    if ($null -ne $property -and $null -ne $property.Value) { return [string]$property.Value }
  }
  return ""
}
$items = New-Object System.Collections.ArrayList
foreach ($record in @(Get-DnsServerResourceRecord -ZoneName $zoneName -ErrorAction Stop)) {
  try {
    $recordData = $record.RecordData
    $type = [string]$record.RecordType
    $name = [string]$record.HostName
    if ([string]::IsNullOrEmpty($name) -or $name -eq ".") { $name = "@" }
    if ($type -eq "PTR" -and $zoneName -match "\.in-addr\.arpa$" -and $name -match "^\d+$") {
      $network = $zoneName -replace "\.in-addr\.arpa$", ""
      $octets = @($network.Split("."))
      [Array]::Reverse($octets)
      $name = (($octets + @($name)) -join ".") + "."
    }
    $value = switch ($type) {
      "A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }
      "AAAA" { Get-RecordDataValue -Data $recordData -Names @("IPv6Address", "IPAddress") }
      "CNAME" { Get-RecordDataValue -Data $recordData -Names @("HostNameAlias", "DomainName") }
      "MX" { "$($recordData.Preference) $(Get-RecordDataValue -Data $recordData -Names @("MailExchange", "DomainName"))" }
      "TXT" { ($recordData.DescriptiveText -join " ") }
      "PTR" { Get-RecordDataValue -Data $recordData -Names @("PtrDomainName", "DomainName") }
      "NS" { Get-RecordDataValue -Data $recordData -Names @("NameServer", "DomainName") }
      "SRV" { "$($recordData.Priority) $($recordData.Weight) $($recordData.Port) $(Get-RecordDataValue -Data $recordData -Names @("DomainName"))" }
      "SOA" { "$(Get-RecordDataValue -Data $recordData -Names @("PrimaryServer", "PrimaryServerName")) $(Get-RecordDataValue -Data $recordData -Names @("ResponsiblePerson", "ResponsiblePersonName")) $($recordData.SerialNumber)" }
      default { "" }
    }
    if (-not [string]::IsNullOrWhiteSpace($value)) {
      [void]$items.Add([PSCustomObject]@{
        id = "$zoneName|$type|$name|$value"
        zoneId = $zoneName
        name = $name
        type = $type
        value = $value
        ttl = [int][Math]::Round($record.TimeToLive.TotalSeconds)
        updatedAt = $now
      })
    }
  } catch {
    [Console]::Error.WriteLine("Skipped DNS record in zone " + $zoneName + ": " + $_.Exception.Message)
  }
}
ConvertTo-Json -InputObject $items -Depth 8
`, psString(zone))
}

func (p *PowerShellProvider) CreateRecord(ctx context.Context, zone string, record Record) (CreateRecordResult, error) {
	if err := validate(record); err != nil {
		return CreateRecordResult{}, err
	}
	if record.TTL <= 0 {
		record.TTL = 3600
	}
	var result CreateRecordResult
	return result, runJSON(ctx, p.createRecordScript(zone, record), &result)
}

func (p *PowerShellProvider) createRecordScript(zone string, record Record) string {
	ptrScript := ""
	if strings.EqualFold(record.Type, "A") && record.CreatePTR {
		ip := net.ParseIP(record.Value).To4()
		if ip == nil {
			ptrScript = `$result.warnings += "记录值不是有效 IPv4 地址"`
		} else {
			fqdn := strings.TrimSpace(record.Name)
			if fqdn == "@" || fqdn == "" {
				fqdn = zone
			} else {
				fqdn += "." + zone
			}
			if !strings.HasSuffix(fqdn, ".") {
				fqdn += "."
			}
			reverseZone := fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
			ptrName := fmt.Sprintf("%d", ip[3])
			ptrScript = fmt.Sprintf(`
$reverseZone = %s
$ptrName = %s
$fqdn = %s
$zone = Get-DnsServerZone -Name $reverseZone -ErrorAction SilentlyContinue
if ($null -eq $zone) {
  $result.warnings += "未找到参照的反向查找区域，无法创建 PTR 记录"
} else {
  $existing = @(Get-DnsServerResourceRecord -ZoneName $reverseZone -Name $ptrName -RRType PTR -ErrorAction SilentlyContinue | Where-Object { $_.RecordData.PtrDomainName -eq $fqdn })
  if ($existing.Count -eq 0) {
    Add-DnsServerResourceRecordPtr -ZoneName $reverseZone -Name $ptrName -PtrDomainName $fqdn -ErrorAction Stop | Out-Null
  }
  $result.ptr = [PSCustomObject]@{ name = %s; type = "PTR"; value = $fqdn; ttl = %d }
}
`, psString(reverseZone), psString(ptrName), psString(fqdn), psString(strings.TrimSpace(record.Value)+"."), record.TTL)
		}
	}
	return fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$zoneName = %s
$name = %s
if ($name -eq "@") { $name = "" }
$ttl = New-TimeSpan -Seconds %d
$result = [PSCustomObject]@{ created = $true; warnings = @(); ptr = $null }
switch (%s) {
%s
  default { throw "Unsupported record type" }
}
%s
if ($result.warnings.Count -eq 0) {
  $result.PSObject.Properties.Remove("warnings")
}
if ($null -eq $result.ptr) {
  $result.PSObject.Properties.Remove("ptr")
}
ConvertTo-Json -InputObject $result -Depth 8
`, psString(zone), psString(recordNameForWrite(zone, record)), record.TTL, psString(strings.ToUpper(record.Type)), addRecordPowerShellCommand(record, "  "), ptrScript)
}

func (p *PowerShellProvider) createPTRBestEffort(ctx context.Context, zone string, record Record) (*Record, string) {
	ip := net.ParseIP(record.Value).To4()
	if ip == nil {
		return nil, "记录值不是有效 IPv4 地址"
	}
	fqdn := strings.TrimSpace(record.Name)
	if fqdn == "@" || fqdn == "" {
		fqdn = zone
	} else {
		fqdn += "." + zone
	}
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	reverseZone := fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
	ptrName := fmt.Sprintf("%d", ip[3])
	script := fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$reverseZone = %s
$ptrName = %s
$fqdn = %s
$zone = Get-DnsServerZone -Name $reverseZone -ErrorAction SilentlyContinue
if ($null -eq $zone) {
  throw ("找不到参照的反向查找区域 " + $reverseZone)
}
$existing = @(Get-DnsServerResourceRecord -ZoneName $reverseZone -Name $ptrName -RRType PTR -ErrorAction SilentlyContinue | Where-Object { $_.RecordData.PtrDomainName -eq $fqdn })
if ($existing.Count -eq 0) {
  Add-DnsServerResourceRecordPtr -ZoneName $reverseZone -Name $ptrName -PtrDomainName $fqdn -ErrorAction Stop | Out-Null
}
`, psString(reverseZone), psString(ptrName), psString(fqdn))
	if err := run(ctx, script); err != nil {
		return nil, "未找到参照的反向查找区域，无法创建 PTR 记录"
	}
	return &Record{Name: strings.TrimSpace(record.Value) + ".", Type: "PTR", Value: fqdn, TTL: record.TTL}, ""
}

func (p *PowerShellProvider) DeleteRecord(ctx context.Context, zone string, record Record) error {
	if err := validate(record); err != nil {
		return err
	}
	script := p.deleteRecordScript(zone, record, true)
	if err := run(ctx, script); err != nil {
		return err
	}
	if strings.EqualFold(record.Type, "A") && record.CreatePTR {
		_ = p.deletePTRBestEffort(ctx, zone, record)
	}
	return nil
}

func (p *PowerShellProvider) deleteRecordScript(zone string, record Record, allowUniqueNameFallback bool) string {
	name := record.Name
	if name == "@" {
		name = ""
	}
	if strings.EqualFold(record.Type, "PTR") {
		name = ptrRecordRelativeName(zone, name)
	}
	return fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$recordType = %s
$recordValue = %s
$allowUniqueNameFallback = %s
function Get-RecordDataValue {
  param([object]$Data, [string[]]$Names)
  foreach ($name in $Names) {
    if ($null -eq $Data) { continue }
    $property = $Data.PSObject.Properties[$name]
    if ($null -ne $property -and $null -ne $property.Value) { return [string]$property.Value }
  }
  return ""
}
function Normalize-DnsRecordValue {
  param([string]$RecordType, [object]$Value)
  if ($null -eq $Value) { return "" }
  $text = ([string]$Value).Trim()
  switch ($RecordType) {
    "CNAME" { return $text.TrimEnd(".").ToLowerInvariant() }
    "PTR" { return $text.TrimEnd(".").ToLowerInvariant() }
    "NS" { return $text.TrimEnd(".").ToLowerInvariant() }
    default { return $text }
  }
}
$recordValueForCompare = Normalize-DnsRecordValue -RecordType $recordType -Value $recordValue
$records = @(Get-DnsServerResourceRecord -ZoneName %s -Name %s -RRType $recordType -ErrorAction Stop)
$target = @($records | Where-Object {
  $recordData = $_.RecordData
  $value = switch ($recordType) {
    "A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }
    "AAAA" { Get-RecordDataValue -Data $recordData -Names @("IPv6Address", "IPAddress") }
    "CNAME" { Get-RecordDataValue -Data $recordData -Names @("HostNameAlias", "DomainName") }
    "MX" { "$($recordData.Preference) $(Get-RecordDataValue -Data $recordData -Names @("MailExchange", "DomainName"))" }
    "TXT" { ($recordData.DescriptiveText -join " ") }
    "PTR" { Get-RecordDataValue -Data $recordData -Names @("PtrDomainName", "DomainName") }
    "NS" { Get-RecordDataValue -Data $recordData -Names @("NameServer", "DomainName") }
    "SRV" { "$($recordData.Priority) $($recordData.Weight) $($recordData.Port) $(Get-RecordDataValue -Data $recordData -Names @("DomainName"))" }
    default { "" }
  }
  $valueForCompare = Normalize-DnsRecordValue -RecordType $recordType -Value $value
  $valueForCompare -eq $recordValueForCompare
} | Select-Object -First 1)[0]
if ($null -eq $target -and $allowUniqueNameFallback -and $records.Count -eq 1) {
  $target = $records[0]
}
if ($null -eq $target) { throw "DNS record not found" }
Remove-DnsServerResourceRecord -ZoneName %s -InputObject $target -Force -ErrorAction Stop
`, psString(strings.ToUpper(record.Type)), psString(record.Value), psBool(allowUniqueNameFallback), psString(zone), psString(name), psString(zone))
}

func (p *PowerShellProvider) deletePTRBestEffort(ctx context.Context, zone string, record Record) error {
	ip := net.ParseIP(record.Value).To4()
	if ip == nil {
		return nil
	}
	fqdn := strings.TrimSpace(record.Name)
	if fqdn == "@" || fqdn == "" {
		fqdn = zone
	} else {
		fqdn += "." + zone
	}
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	reverseZone := fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
	ptrName := fmt.Sprintf("%d", ip[3])
	script := fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$reverseZone = %s
$ptrName = %s
$fqdn = %s
$zone = Get-DnsServerZone -Name $reverseZone -ErrorAction SilentlyContinue
if ($null -eq $zone) { return }
$records = @(Get-DnsServerResourceRecord -ZoneName $reverseZone -Name $ptrName -RRType PTR -ErrorAction SilentlyContinue)
$targets = @($records | Where-Object { $_.RecordData.PtrDomainName -eq $fqdn })
foreach ($target in $targets) {
  Remove-DnsServerResourceRecord -ZoneName $reverseZone -InputObject $target -Force -ErrorAction Stop
}
`, psString(reverseZone), psString(ptrName), psString(fqdn))
	return run(ctx, script)
}

func ptrRecordRelativeName(zone, name string) string {
	name = strings.Trim(strings.TrimSpace(name), ".")
	ip := net.ParseIP(name).To4()
	if ip == nil {
		return name
	}
	network := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".in-addr.arpa")
	parts := strings.Split(network, ".")
	if len(parts) == 0 {
		return name
	}
	if len(parts) >= net.IPv4len {
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

func reverseZoneNetworkID(name string) string {
	value := strings.Trim(strings.TrimSpace(name), ".")
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, ".in-addr.arpa") {
		return strings.TrimSuffix(value, ".in-addr.arpa")
	}
	return value
}

func recordNameForWrite(zone string, record Record) string {
	if strings.EqualFold(record.Type, "PTR") {
		return ptrRecordRelativeName(zone, record.Name)
	}
	return record.Name
}

func addRecordPowerShellCommand(record Record, indent string) string {
	value := psString(record.Value)
	lines := []string{
		fmt.Sprintf(`"A" { Add-DnsServerResourceRecordA -ZoneName $zoneName -Name $name -IPv4Address %s -TimeToLive $ttl -ErrorAction Stop | Out-Null }`, value),
		fmt.Sprintf(`"AAAA" { Add-DnsServerResourceRecordAAAA -ZoneName $zoneName -Name $name -IPv6Address %s -TimeToLive $ttl -ErrorAction Stop | Out-Null }`, value),
		fmt.Sprintf(`"CNAME" { Add-DnsServerResourceRecordCName -ZoneName $zoneName -Name $name -HostNameAlias %s -TimeToLive $ttl -ErrorAction Stop | Out-Null }`, value),
		fmt.Sprintf(`"PTR" { Add-DnsServerResourceRecordPtr -ZoneName $zoneName -Name $name -PtrDomainName %s -TimeToLive $ttl -ErrorAction Stop | Out-Null }`, value),
		fmt.Sprintf(`"TXT" { Add-DnsServerResourceRecord -Txt -ZoneName $zoneName -Name $name -DescriptiveText %s -TimeToLive $ttl -ErrorAction Stop | Out-Null }`, value),
	}
	return indent + strings.Join(lines, "\n"+indent)
}

func (p *PowerShellProvider) UpdateRecord(ctx context.Context, zone string, update RecordUpdate) (CreateRecordResult, error) {
	if err := validate(update.Old); err != nil {
		return CreateRecordResult{}, err
	}
	if err := validate(update.New); err != nil {
		return CreateRecordResult{}, err
	}
	if !strings.EqualFold(update.Old.Name, update.New.Name) || !strings.EqualFold(update.Old.Type, update.New.Type) {
		return CreateRecordResult{}, fmt.Errorf("record update only supports value changes")
	}
	if update.New.TTL <= 0 {
		update.New.TTL = update.Old.TTL
	}
	if update.New.TTL <= 0 {
		update.New.TTL = 3600
	}
	if err := run(ctx, p.updateRecordScript(zone, update)); err != nil {
		return CreateRecordResult{}, err
	}
	if strings.EqualFold(update.Old.Type, "A") && update.Old.CreatePTR {
		_ = p.deletePTRBestEffort(ctx, zone, update.Old)
	}
	result := CreateRecordResult{Created: true}
	if strings.EqualFold(update.New.Type, "A") && update.New.CreatePTR {
		ptrRecord, warning := p.createPTRBestEffort(ctx, zone, update.New)
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		} else if ptrRecord != nil {
			result.PTR = ptrRecord
		}
	}
	return result, nil
}

func (p *PowerShellProvider) updateRecordScript(zone string, update RecordUpdate) string {
	oldName := recordNameForWrite(zone, update.Old)
	if oldName == "@" {
		oldName = ""
	}
	newName := recordNameForWrite(zone, update.New)
	if newName == "@" {
		newName = ""
	}
	return fmt.Sprintf(`
Import-Module DnsServer -ErrorAction Stop
$zoneName = %s
$recordType = %s
$recordValue = %s
$allowUniqueNameFallback = $true
$name = %s
$ttl = New-TimeSpan -Seconds %d
function Get-RecordDataValue {
  param([object]$Data, [string[]]$Names)
  foreach ($name in $Names) {
    if ($null -eq $Data) { continue }
    $property = $Data.PSObject.Properties[$name]
    if ($null -ne $property -and $null -ne $property.Value) { return [string]$property.Value }
  }
  return ""
}
function Normalize-DnsRecordValue {
  param([string]$RecordType, [object]$Value)
  if ($null -eq $Value) { return "" }
  $text = ([string]$Value).Trim()
  switch ($RecordType) {
    "CNAME" { return $text.TrimEnd(".").ToLowerInvariant() }
    "PTR" { return $text.TrimEnd(".").ToLowerInvariant() }
    "NS" { return $text.TrimEnd(".").ToLowerInvariant() }
    default { return $text }
  }
}
$recordValueForCompare = Normalize-DnsRecordValue -RecordType $recordType -Value $recordValue
$records = @(Get-DnsServerResourceRecord -ZoneName $zoneName -Name %s -RRType $recordType -ErrorAction Stop)
$target = @($records | Where-Object {
  $recordData = $_.RecordData
  $value = switch ($recordType) {
    "A" { Get-RecordDataValue -Data $recordData -Names @("IPv4Address", "IPAddress") }
    "AAAA" { Get-RecordDataValue -Data $recordData -Names @("IPv6Address", "IPAddress") }
    "CNAME" { Get-RecordDataValue -Data $recordData -Names @("HostNameAlias", "DomainName") }
    "MX" { "$($recordData.Preference) $(Get-RecordDataValue -Data $recordData -Names @("MailExchange", "DomainName"))" }
    "TXT" { ($recordData.DescriptiveText -join " ") }
    "PTR" { Get-RecordDataValue -Data $recordData -Names @("PtrDomainName", "DomainName") }
    "NS" { Get-RecordDataValue -Data $recordData -Names @("NameServer", "DomainName") }
    "SRV" { "$($recordData.Priority) $($recordData.Weight) $($recordData.Port) $(Get-RecordDataValue -Data $recordData -Names @("DomainName"))" }
    default { "" }
  }
  $valueForCompare = Normalize-DnsRecordValue -RecordType $recordType -Value $value
  $valueForCompare -eq $recordValueForCompare
} | Select-Object -First 1)[0]
if ($null -eq $target -and $allowUniqueNameFallback -and $records.Count -eq 1) {
  $target = $records[0]
}
if ($null -eq $target) { throw "DNS record not found" }
Remove-DnsServerResourceRecord -ZoneName $zoneName -InputObject $target -Force -ErrorAction Stop
switch ($recordType) {
%s
  default { throw "Unsupported record type" }
}
`, psString(zone), psString(strings.ToUpper(update.Old.Type)), psString(update.Old.Value), psString(newName), update.New.TTL, psString(oldName), addRecordPowerShellCommand(update.New, "  "))
}

func validate(record Record) error {
	if strings.TrimSpace(record.Name) == "" || strings.TrimSpace(record.Type) == "" || strings.TrimSpace(record.Value) == "" {
		return fmt.Errorf("record name, type and value are required")
	}
	if strings.EqualFold(record.Type, "A") && net.ParseIP(record.Value).To4() == nil {
		return fmt.Errorf("invalid IPv4 address")
	}
	if strings.EqualFold(record.Type, "AAAA") && net.ParseIP(record.Value).To16() == nil {
		return fmt.Errorf("invalid IPv6 address")
	}
	return nil
}

func runJSON(ctx context.Context, script string, dst any) error {
	out, err := runOutput(ctx, script)
	if err != nil {
		return err
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		out = []byte("[]")
	}
	return json.Unmarshal(out, dst)
}

func run(ctx context.Context, script string) error {
	_, err := runOutput(ctx, script)
	return err
}

func runOutput(ctx context.Context, script string) ([]byte, error) {
	timeout := powerShellTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	file, err := os.CreateTemp("", "zonelease-dns-*.ps1")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := file.WriteString(script); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("powershell timed out after %s", timeout)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, fmt.Errorf("powershell canceled")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("powershell failed: %s", msg)
	}
	return stdout.Bytes(), nil
}

func setPowerShellTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	atomic.StoreInt64(&powerShellTimeoutSeconds, int64(timeout/time.Second))
}

func powerShellTimeout() time.Duration {
	seconds := atomic.LoadInt64(&powerShellTimeoutSeconds)
	if seconds <= 0 {
		seconds = 180
	}
	return time.Duration(seconds) * time.Second
}

func psString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func psBool(value bool) string {
	if value {
		return "$true"
	}
	return "$false"
}
