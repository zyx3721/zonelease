package dhcp

import (
	"bytes"
	"context"
	"encoding/binary"
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

func (p *PowerShellProvider) Probe(ctx context.Context) error {
	return run(ctx, `
Import-Module DhcpServer -ErrorAction Stop
Get-DhcpServerSetting -ErrorAction Stop | Out-Null
`)
}

func (p *PowerShellProvider) ListScopes(ctx context.Context) ([]Scope, error) {
	script := p.listScopesScript()
	var scopes []Scope
	return scopes, runJSON(ctx, script, &scopes)
}

func (p *PowerShellProvider) listScopesScript() string {
	return `
Import-Module DhcpServer -ErrorAction Stop
function Get-TextValue {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  return [string]$Value
}
function Get-LeaseDurationSeconds {
  param([object]$Value)
  if ($null -eq $Value) { return 0 }
  try { return [int][Math]::Round($Value.TotalSeconds) } catch { return 0 }
}
function Get-DefaultGateway {
  param([object]$ScopeId)
  try {
    $value = Get-DhcpServerv4OptionValue -ScopeId $ScopeId -OptionId 3 -ErrorAction Stop
    if ($null -eq $value -or $null -eq $value.Value) { return "" }
    $values = @($value.Value)
    if ($values.Count -eq 0) { return "" }
    return Get-TextValue $values[0]
  } catch { return "" }
}
$items = New-Object System.Collections.ArrayList
foreach ($scope in @(Get-DhcpServerv4Scope -ErrorAction Stop)) {
  try {
    $leaseDurationSeconds = Get-LeaseDurationSeconds $scope.LeaseDuration
    $scopeId = Get-TextValue $scope.ScopeId
    [void]$items.Add([PSCustomObject]@{
      id = $scopeId
      name = Get-TextValue $scope.Name
      description = Get-TextValue $scope.Description
      subnet = "$scopeId/$(Get-TextValue $scope.SubnetMask)"
      defaultGateway = Get-DefaultGateway $scope.ScopeId
      startRange = Get-TextValue $scope.StartRange
      endRange = Get-TextValue $scope.EndRange
      leaseDurationHours = if ($leaseDurationSeconds -gt 0) { [int][Math]::Ceiling($leaseDurationSeconds / 3600) } else { 0 }
      leaseDurationSeconds = $leaseDurationSeconds
      state = if ($scope.State -eq "Active") { "Active" } else { "Inactive" }
      serverId = "local"
    })
  } catch {
    [Console]::Error.WriteLine("Skipped DHCP scope: " + $_.Exception.Message)
  }
}
ConvertTo-Json -InputObject $items -Depth 8
`
}

func (p *PowerShellProvider) CreateScope(ctx context.Context, scope Scope) (Scope, error) {
	scope, script, err := p.createScopeScript(scope)
	if err != nil {
		return Scope{}, err
	}
	return scope, run(ctx, script)
}

func (p *PowerShellProvider) createScopeScript(scope Scope) (Scope, string, error) {
	scope, _, subnetMask, err := normalizeScope(scope)
	if err != nil {
		return Scope{}, "", err
	}
	leaseDurationArg := fmt.Sprintf("-LeaseDuration (New-TimeSpan -Hours %d) ", scope.LeaseDurationHours)
	if scope.LeaseDurationSeconds == -1 {
		leaseDurationArg = ""
	}
	descriptionArg := ""
	if strings.TrimSpace(scope.Description) != "" {
		descriptionArg = "-Description " + psString(scope.Description) + " "
	}
	defaultGatewayScript := ""
	if strings.TrimSpace(scope.DefaultGateway) != "" {
		defaultGatewayScript = fmt.Sprintf("\nSet-DhcpServerv4OptionValue -ScopeId %s -Router %s -ErrorAction Stop", psString(scope.ID), psString(scope.DefaultGateway))
	}
	script := fmt.Sprintf(`
Import-Module DhcpServer -ErrorAction Stop
Add-DhcpServerv4Scope -Name %s -StartRange %s -EndRange %s -SubnetMask %s %s%s-State Active -ErrorAction Stop | Out-Null
%s
`, psString(scope.Name), psString(scope.StartRange), psString(scope.EndRange), psString(subnetMask), descriptionArg, leaseDurationArg, defaultGatewayScript)
	return scope, script, nil
}

func (p *PowerShellProvider) UpdateScope(ctx context.Context, scope Scope) (Scope, error) {
	scope, scopeID, err := normalizeScopeUpdate(scope)
	if err != nil {
		return Scope{}, err
	}
	changed := scopeChangedSet(scope.ChangedFields)
	if len(changed) == 0 {
		changed["name"] = true
		changed["description"] = true
		changed["gateway"] = true
		changed["lease"] = true
		changed["range"] = true
		changed["state"] = true
	}
	args := []string{"-ScopeId " + psString(scopeID)}
	if changed["name"] {
		args = append(args, "-Name "+psString(scope.Name))
	}
	if changed["description"] {
		args = append(args, "-Description "+psString(scope.Description))
	}
	if changed["lease"] {
		if scope.LeaseDurationSeconds == -1 {
			args = append(args, "-LeaseDuration (New-TimeSpan -Seconds 4294967295)")
		} else {
			leaseSeconds := scope.LeaseDurationSeconds
			if leaseSeconds <= 0 {
				leaseSeconds = scope.LeaseDurationHours * 3600
			}
			args = append(args, fmt.Sprintf("-LeaseDuration (New-TimeSpan -Seconds %d)", leaseSeconds))
		}
	}
	if changed["state"] {
		args = append(args, "-State "+psString(scope.State))
	}
	if changed["range"] && scope.StartRange != "" && scope.EndRange != "" {
		args = append(args, "-StartRange "+psString(scope.StartRange), "-EndRange "+psString(scope.EndRange))
	}
	commands := []string{"Import-Module DhcpServer -ErrorAction Stop"}
	if len(args) > 1 {
		commands = append(commands, "Set-DhcpServerv4Scope "+strings.Join(args, " ")+" -ErrorAction Stop")
	}
	if changed["gateway"] {
		commands = append(commands, "Set-DhcpServerv4OptionValue -ScopeId "+psString(scopeID)+" -Router "+psString(scope.DefaultGateway)+" -ErrorAction Stop")
	}
	if len(commands) == 1 {
		return scope, nil
	}
	script := strings.Join(commands, "\n")
	return scope, run(ctx, script)
}

func scopeChangedSet(fields []string) map[string]bool {
	result := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(strings.ToLower(field))
		if field != "" {
			result[field] = true
		}
	}
	return result
}

func (p *PowerShellProvider) SetScopeState(ctx context.Context, scopeID string, active bool) error {
	scopeID = strings.TrimSpace(scopeID)
	if net.ParseIP(scopeID).To4() == nil {
		return fmt.Errorf("scope id must be an IPv4 address")
	}
	state := "Inactive"
	if active {
		state = "Active"
	}
	return run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Set-DhcpServerv4Scope -ScopeId %s -State %s -ErrorAction Stop`, psString(scopeID), psString(state)))
}

func (p *PowerShellProvider) DeleteScope(ctx context.Context, scopeID string) error {
	scopeID = strings.TrimSpace(scopeID)
	if net.ParseIP(scopeID).To4() == nil {
		return fmt.Errorf("scope id must be an IPv4 address")
	}
	return run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Remove-DhcpServerv4Scope -ScopeId %s -Force -ErrorAction Stop`, psString(scopeID)))
}

func (p *PowerShellProvider) ListExclusions(ctx context.Context, scopeID string) ([]Exclusion, error) {
	script := p.listExclusionsScript(scopeID)
	var exclusions []Exclusion
	return exclusions, runJSON(ctx, script, &exclusions)
}

func (p *PowerShellProvider) listExclusionsScript(scopeID string) string {
	return fmt.Sprintf(`
Import-Module DhcpServer -ErrorAction Stop
$scopeId = %s
function Get-TextValue {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  return [string]$Value
}
$items = New-Object System.Collections.ArrayList
foreach ($range in @(Get-DhcpServerv4ExclusionRange -ScopeId $scopeId -ErrorAction Stop)) {
  try {
    $startIp = Get-TextValue $range.StartRange
    $endIp = Get-TextValue $range.EndRange
    if ([string]::IsNullOrWhiteSpace($startIp) -or [string]::IsNullOrWhiteSpace($endIp)) { continue }
    [void]$items.Add([PSCustomObject]@{
      id = "$scopeId|$startIp|$endIp"
      scopeId = $scopeId
      startIp = $startIp
      endIp = $endIp
    })
  } catch {
    [Console]::Error.WriteLine("Skipped DHCP exclusion in scope " + $scopeId + ": " + $_.Exception.Message)
  }
}
ConvertTo-Json -InputObject $items -Depth 8
`, psString(scopeID))
}

func (p *PowerShellProvider) CreateExclusion(ctx context.Context, exclusion Exclusion) (Exclusion, error) {
	exclusion, err := normalizeExclusion(exclusion)
	if err != nil {
		return Exclusion{}, err
	}
	script := fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Add-DhcpServerv4ExclusionRange -ScopeId %s -StartRange %s -EndRange %s -ErrorAction Stop | Out-Null`, psString(exclusion.ScopeID), psString(exclusion.StartIP), psString(exclusion.EndIP))
	return exclusion, run(ctx, script)
}

func (p *PowerShellProvider) DeleteExclusion(ctx context.Context, scopeID, startIP, endIP string) error {
	exclusion, err := normalizeExclusion(Exclusion{ScopeID: scopeID, StartIP: startIP, EndIP: endIP})
	if err != nil {
		return err
	}
	return run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Remove-DhcpServerv4ExclusionRange -ScopeId %s -StartRange %s -EndRange %s -Confirm:$false -ErrorAction Stop`, psString(exclusion.ScopeID), psString(exclusion.StartIP), psString(exclusion.EndIP)))
}

func (p *PowerShellProvider) ListLeases(ctx context.Context, scopeID string) ([]Lease, error) {
	script := p.listLeasesScript(scopeID)
	var leases []Lease
	return leases, runJSON(ctx, script, &leases)
}

func (p *PowerShellProvider) listLeasesScript(scopeID string) string {
	return fmt.Sprintf(`
Import-Module DhcpServer -ErrorAction Stop
$scopeId = %s
function Get-TextValue {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  return [string]$Value
}
function Get-IsoTime {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  try { return $Value.ToString("o") } catch { return "" }
}
$items = New-Object System.Collections.ArrayList
foreach ($lease in @(Get-DhcpServerv4Lease -ScopeId $scopeId -ErrorAction Stop)) {
  try {
    $ip = Get-TextValue $lease.IPAddress
    if ([string]::IsNullOrWhiteSpace($ip)) { continue }
    [void]$items.Add([PSCustomObject]@{
      id = "$scopeId|$ip"
      scopeId = $scopeId
      ip = $ip
      mac = Get-TextValue $lease.ClientId
      hostname = Get-TextValue $lease.HostName
      state = Get-TextValue $lease.AddressState
      expiresAt = Get-IsoTime $lease.LeaseExpiryTime
    })
  } catch {
    [Console]::Error.WriteLine("Skipped DHCP lease in scope " + $scopeId + ": " + $_.Exception.Message)
  }
}
ConvertTo-Json -InputObject $items -Depth 8
`, psString(scopeID))
}

func (p *PowerShellProvider) ReleaseLease(ctx context.Context, scopeID, ip string) error {
	scopeID = strings.TrimSpace(scopeID)
	ip = strings.TrimSpace(ip)
	if net.ParseIP(scopeID).To4() == nil {
		return fmt.Errorf("scope id must be an IPv4 address")
	}
	if net.ParseIP(ip).To4() == nil {
		return fmt.Errorf("lease ip must be an IPv4 address")
	}
	return run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Remove-DhcpServerv4Lease -ScopeId %s -IPAddress %s -Confirm:$false -ErrorAction Stop`, psString(scopeID), psString(ip)))
}

func (p *PowerShellProvider) ListReservations(ctx context.Context, scopeID string) ([]Reservation, error) {
	script := p.listReservationsScript(scopeID)
	var reservations []Reservation
	return reservations, runJSON(ctx, script, &reservations)
}

func (p *PowerShellProvider) listReservationsScript(scopeID string) string {
	return fmt.Sprintf(`
Import-Module DhcpServer -ErrorAction Stop
$scopeId = %s
function Get-TextValue {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  return [string]$Value
}
function Get-ReservationDetails {
  param([string]$ScopeId, [string]$IP)
  try {
    $items = @(Get-DhcpServerv4Reservation -ScopeId $ScopeId -IPAddress $IP -ErrorAction Stop)
    if ($items.Count -gt 0) { return $items[0] }
  } catch {}
  return $null
}
$items = New-Object System.Collections.ArrayList
foreach ($reservation in @(Get-DhcpServerv4Reservation -ScopeId $scopeId -ErrorAction Stop)) {
  try {
    $ip = Get-TextValue $reservation.IPAddress
    if ([string]::IsNullOrWhiteSpace($ip)) { continue }
    $name = Get-TextValue $reservation.Name
    $description = Get-TextValue $reservation.Description
    if ([string]::IsNullOrWhiteSpace($name) -or [string]::IsNullOrWhiteSpace($description)) {
      $details = Get-ReservationDetails -ScopeId $scopeId -IP $ip
      if ($null -ne $details) {
        if ([string]::IsNullOrWhiteSpace($name)) { $name = Get-TextValue $details.Name }
        if ([string]::IsNullOrWhiteSpace($description)) { $description = Get-TextValue $details.Description }
      }
    }
    [void]$items.Add([PSCustomObject]@{
      id = "$scopeId|$ip"
      scopeId = $scopeId
      ip = $ip
      mac = Get-TextValue $reservation.ClientId
      name = $name
      description = $description
    })
  } catch {
    [Console]::Error.WriteLine("Skipped DHCP reservation in scope " + $scopeId + ": " + $_.Exception.Message)
  }
}
ConvertTo-Json -InputObject $items -Depth 8
`, psString(scopeID))
}

func (p *PowerShellProvider) CreateReservation(ctx context.Context, reservation Reservation) (Reservation, error) {
	reservation, err := normalizeReservation(reservation)
	if err != nil {
		return Reservation{}, err
	}
	err = run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Add-DhcpServerv4Reservation -ScopeId %s -IPAddress %s -ClientId %s -Name %s -Description %s -ErrorAction Stop | Out-Null`,
		psString(reservation.ScopeID), psString(reservation.IP), psString(reservation.MAC), psString(reservation.Name), psString(reservation.Description)))
	if err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

func (p *PowerShellProvider) UpdateReservation(ctx context.Context, update ReservationUpdate) (Reservation, error) {
	oldReservation, err := normalizeReservation(update.Old)
	if err != nil {
		return Reservation{}, fmt.Errorf("old reservation: %w", err)
	}
	newReservation, err := normalizeReservation(update.New)
	if err != nil {
		return Reservation{}, fmt.Errorf("new reservation: %w", err)
	}
	script := fmt.Sprintf(`
Import-Module DhcpServer -ErrorAction Stop
Remove-DhcpServerv4Reservation -ScopeId %s -IPAddress %s -Confirm:$false -ErrorAction Stop
Add-DhcpServerv4Reservation -ScopeId %s -IPAddress %s -ClientId %s -Name %s -Description %s -ErrorAction Stop | Out-Null
`, psString(oldReservation.ScopeID), psString(oldReservation.IP), psString(newReservation.ScopeID), psString(newReservation.IP), psString(newReservation.MAC), psString(newReservation.Name), psString(newReservation.Description))
	if err := run(ctx, script); err != nil {
		return Reservation{}, err
	}
	return newReservation, nil
}

func (p *PowerShellProvider) DeleteReservation(ctx context.Context, scopeID, ip string) error {
	scopeID = strings.TrimSpace(scopeID)
	ip = strings.TrimSpace(ip)
	if net.ParseIP(scopeID).To4() == nil {
		return fmt.Errorf("reservation scope id must be an IPv4 address")
	}
	if net.ParseIP(ip).To4() == nil {
		return fmt.Errorf("reservation ip must be an IPv4 address")
	}
	return run(ctx, fmt.Sprintf(`Import-Module DhcpServer -ErrorAction Stop
Remove-DhcpServerv4Reservation -ScopeId %s -IPAddress %s -Confirm:$false -ErrorAction Stop`, psString(scopeID), psString(ip)))
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
	file, err := os.CreateTemp("", "zonelease-dhcp-*.ps1")
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
		msg = classifyPowerShellError(msg)
		return nil, fmt.Errorf("powershell failed: %s", msg)
	}
	return stdout.Bytes(), nil
}

func classifyPowerShellError(message string) string {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "dhcpserver") && (strings.Contains(lower, "module") || strings.Contains(lower, "not loaded") || strings.Contains(lower, "was not loaded")):
		return "DHCP PowerShell module is unavailable"
	case strings.Contains(lower, "access is denied") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "permission"):
		return "administrator privileges are required"
	case strings.Contains(lower, "service") && (strings.Contains(lower, "not running") || strings.Contains(lower, "stopped")):
		return "DHCP Server service is not running"
	case strings.Contains(lower, "already exists") || strings.Contains(lower, "already present"):
		return "DHCP resource already exists"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") || strings.Contains(lower, "cannot find"):
		return "DHCP resource was not found"
	default:
		return trimmed
	}
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

func parseScopeSubnet(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("scope subnet is required")
	}
	if !strings.Contains(value, "/") {
		if net.ParseIP(value).To4() == nil {
			return "", "", fmt.Errorf("scope subnet must be an IPv4 address or CIDR")
		}
		return value, "255.255.255.0", nil
	}
	ip, network, err := net.ParseCIDR(value)
	if err != nil || ip.To4() == nil {
		return "", "", fmt.Errorf("scope subnet must be a valid IPv4 CIDR")
	}
	mask := net.IP(network.Mask).To4()
	if mask == nil {
		return "", "", fmt.Errorf("scope subnet mask must be IPv4")
	}
	return ip.String(), mask.String(), nil
}

func normalizeScope(scope Scope) (Scope, string, string, error) {
	scope.Name = strings.TrimSpace(scope.Name)
	scope.Description = strings.TrimSpace(scope.Description)
	scope.Subnet = strings.TrimSpace(scope.Subnet)
	scope.DefaultGateway = strings.TrimSpace(scope.DefaultGateway)
	scope.StartRange = strings.TrimSpace(scope.StartRange)
	scope.EndRange = strings.TrimSpace(scope.EndRange)
	scope.State = strings.TrimSpace(scope.State)
	if scope.Name == "" || scope.Subnet == "" {
		return Scope{}, "", "", fmt.Errorf("scope name and subnet are required")
	}
	scopeID, subnetMask, err := parseScopeSubnet(scope.Subnet)
	if err != nil {
		return Scope{}, "", "", err
	}
	if scope.StartRange == "" || scope.EndRange == "" {
		return Scope{}, "", "", fmt.Errorf("scope start and end ranges are required")
	}
	if net.ParseIP(scope.StartRange).To4() == nil || net.ParseIP(scope.EndRange).To4() == nil {
		return Scope{}, "", "", fmt.Errorf("scope start and end ranges must be IPv4 addresses")
	}
	if net.ParseIP(scopeID).To4() == nil {
		return Scope{}, "", "", fmt.Errorf("scope subnet must start with an IPv4 address")
	}
	if err := validateIPv4RangeInScope(scopeID, subnetMask, scope.StartRange, scope.EndRange); err != nil {
		return Scope{}, "", "", err
	}
	if scope.DefaultGateway == "" {
		return Scope{}, "", "", fmt.Errorf("default gateway is required")
	}
	if net.ParseIP(scope.DefaultGateway).To4() == nil {
		return Scope{}, "", "", fmt.Errorf("default gateway must be an IPv4 address")
	}
	if err := validateIPv4InScope(scopeID, subnetMask, scope.DefaultGateway, "default gateway"); err != nil {
		return Scope{}, "", "", err
	}
	if scope.LeaseDurationSeconds == -1 {
		scope.LeaseDurationHours = 0
	} else if scope.LeaseDurationHours <= 0 {
		scope.LeaseDurationHours = 24
	}
	if scope.State == "" {
		scope.State = "Active"
	}
	scope.ID = scopeID
	scope.Subnet = scopeID + "/" + subnetMask
	scope.ServerID = "local"
	return scope, scopeID, subnetMask, nil
}

func normalizeScopeUpdate(scope Scope) (Scope, string, error) {
	scope.ID = strings.TrimSpace(scope.ID)
	scope.Name = strings.TrimSpace(scope.Name)
	scope.Description = strings.TrimSpace(scope.Description)
	scope.Subnet = strings.TrimSpace(scope.Subnet)
	scope.DefaultGateway = strings.TrimSpace(scope.DefaultGateway)
	scope.State = strings.TrimSpace(scope.State)
	if scope.Name == "" {
		return Scope{}, "", fmt.Errorf("scope name is required")
	}
	scopeID := scope.ID
	if scopeID == "" {
		parsedScopeID, _, err := parseScopeSubnet(scope.Subnet)
		if err != nil {
			return Scope{}, "", err
		}
		scopeID = parsedScopeID
	}
	if net.ParseIP(scopeID).To4() == nil {
		return Scope{}, "", fmt.Errorf("scope id must be an IPv4 address")
	}
	if scope.DefaultGateway == "" {
		return Scope{}, "", fmt.Errorf("default gateway is required")
	}
	if net.ParseIP(scope.DefaultGateway).To4() == nil {
		return Scope{}, "", fmt.Errorf("default gateway must be an IPv4 address")
	}
	if scope.Subnet != "" {
		parsedScopeID, subnetMask, err := parseScopeSubnet(scope.Subnet)
		if err != nil {
			return Scope{}, "", err
		}
		if err := validateIPv4InScope(parsedScopeID, subnetMask, scope.DefaultGateway, "default gateway"); err != nil {
			return Scope{}, "", err
		}
	}
	if scope.LeaseDurationSeconds == -1 {
		scope.LeaseDurationHours = 0
	} else if scope.LeaseDurationHours <= 0 {
		scope.LeaseDurationHours = 24
	}
	if scope.State == "" {
		scope.State = "Active"
	}
	scope.ID = scopeID
	scope.ServerID = "local"
	return scope, scopeID, nil
}

func normalizeReservation(reservation Reservation) (Reservation, error) {
	reservation.ScopeID = strings.TrimSpace(reservation.ScopeID)
	reservation.IP = strings.TrimSpace(reservation.IP)
	reservation.MAC = strings.TrimSpace(reservation.MAC)
	reservation.Name = strings.TrimSpace(reservation.Name)
	reservation.Description = strings.TrimSpace(reservation.Description)
	if reservation.ScopeID == "" || reservation.IP == "" || reservation.MAC == "" {
		return Reservation{}, fmt.Errorf("reservation scope id, ip and mac are required")
	}
	if net.ParseIP(reservation.ScopeID).To4() == nil {
		return Reservation{}, fmt.Errorf("reservation scope id must be an IPv4 address")
	}
	if net.ParseIP(reservation.IP).To4() == nil {
		return Reservation{}, fmt.Errorf("reservation ip must be an IPv4 address")
	}
	reservation.ID = reservation.ScopeID + "|" + reservation.IP
	return reservation, nil
}

func normalizeExclusion(exclusion Exclusion) (Exclusion, error) {
	exclusion.ScopeID = strings.TrimSpace(exclusion.ScopeID)
	exclusion.StartIP = strings.TrimSpace(exclusion.StartIP)
	exclusion.EndIP = strings.TrimSpace(exclusion.EndIP)
	if net.ParseIP(exclusion.ScopeID).To4() == nil {
		return Exclusion{}, fmt.Errorf("scope id must be an IPv4 address")
	}
	if net.ParseIP(exclusion.StartIP).To4() == nil || net.ParseIP(exclusion.EndIP).To4() == nil {
		return Exclusion{}, fmt.Errorf("exclusion range must use IPv4 addresses")
	}
	exclusion.ID = exclusion.ScopeID + "|" + exclusion.StartIP + "|" + exclusion.EndIP
	return exclusion, nil
}

func validateIPv4RangeInScope(scopeID, subnetMask, startRange, endRange string) error {
	scopeIP := net.ParseIP(strings.TrimSpace(scopeID)).To4()
	maskIP := net.ParseIP(strings.TrimSpace(subnetMask)).To4()
	startIP := net.ParseIP(strings.TrimSpace(startRange)).To4()
	endIP := net.ParseIP(strings.TrimSpace(endRange)).To4()
	if scopeIP == nil || maskIP == nil || startIP == nil || endIP == nil {
		return fmt.Errorf("scope and range values must be IPv4 addresses")
	}
	mask := net.IPMask(maskIP)
	network := scopeIP.Mask(mask)
	if !startIP.Mask(mask).Equal(network) || !endIP.Mask(mask).Equal(network) {
		return fmt.Errorf("scope start and end ranges must belong to the scope subnet")
	}
	if ipv4ToUint32(startIP) > ipv4ToUint32(endIP) {
		return fmt.Errorf("scope start range must not be greater than end range")
	}
	return nil
}

func validateIPv4InScope(scopeID, subnetMask, value, label string) error {
	scopeIP := net.ParseIP(strings.TrimSpace(scopeID)).To4()
	maskIP := net.ParseIP(strings.TrimSpace(subnetMask)).To4()
	valueIP := net.ParseIP(strings.TrimSpace(value)).To4()
	if scopeIP == nil || maskIP == nil || valueIP == nil {
		return fmt.Errorf("%s and scope values must be IPv4 addresses", label)
	}
	mask := net.IPMask(maskIP)
	network := scopeIP.Mask(mask)
	if !valueIP.Mask(mask).Equal(network) {
		return fmt.Errorf("%s must belong to the scope subnet", label)
	}
	networkUint := binary.BigEndian.Uint32(network)
	valueUint := binary.BigEndian.Uint32(valueIP)
	broadcastUint := networkUint | ^binary.BigEndian.Uint32(maskIP)
	if valueUint == networkUint || valueUint == broadcastUint {
		return fmt.Errorf("%s cannot be the subnet or broadcast address", label)
	}
	return nil
}

func ipv4ToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}
