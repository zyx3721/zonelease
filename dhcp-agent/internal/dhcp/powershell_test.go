package dhcp

import (
	"strings"
	"testing"
)

func TestListScopesScriptSkipsInvalidScopeRows(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listScopesScript()
	if !strings.Contains(script, `$items = New-Object System.Collections.ArrayList`) {
		t.Fatal("list scopes script should collect scopes explicitly")
	}
	if !strings.Contains(script, `Skipped DHCP scope`) {
		t.Fatal("list scopes script should skip invalid scope rows")
	}
	if !strings.Contains(script, `Get-LeaseDurationSeconds`) {
		t.Fatal("list scopes script should handle missing lease duration")
	}
	if !strings.Contains(script, `10675199`) || !strings.Contains(script, `return -1`) {
		t.Fatal("list scopes script should recognize TimeSpan MaxValue lease duration")
	}
	if !strings.Contains(script, `Convert-MaskToPrefix`) || !strings.Contains(script, `subnet = "$scopeId/$prefix"`) {
		t.Fatal("list scopes script should return CIDR prefix subnet values")
	}
}

func TestGetScopeScriptReadsSingleScope(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.getScopeScript("10.24.0.0")
	if !strings.Contains(script, `Get-DhcpServerv4Scope -ScopeId '10.24.0.0'`) {
		t.Fatalf("get scope script should target one scope: %s", script)
	}
	if strings.Contains(script, `foreach ($scope in @(Get-DhcpServerv4Scope`) {
		t.Fatalf("get scope script should not enumerate all scopes: %s", script)
	}
	if !strings.Contains(script, `subnet = "$scopeId/$prefix"`) {
		t.Fatalf("get scope script should return CIDR prefix subnet: %s", script)
	}
	if !strings.Contains(script, `10675199`) || !strings.Contains(script, `return -1`) {
		t.Fatalf("get scope script should recognize TimeSpan MaxValue lease duration: %s", script)
	}
}

func TestListLeasesScriptSkipsInvalidLeaseRows(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listLeasesScript("10.24.0.0")
	if !strings.Contains(script, `Get-IsoTime`) {
		t.Fatal("list leases script should handle missing lease expiry time")
	}
	if !strings.Contains(script, `Skipped DHCP lease in scope`) {
		t.Fatal("list leases script should skip invalid lease rows")
	}
	if !strings.Contains(script, `if ([string]::IsNullOrWhiteSpace($ip)) { continue }`) {
		t.Fatal("list leases script should skip rows without an IP address")
	}
	if !strings.Contains(script, `Get-MacValue $lease.ClientId`) {
		t.Fatal("list leases script should normalize client id separators")
	}
	if !strings.Contains(script, `Get-LeaseState $lease.AddressState`) || !strings.Contains(script, `inactivereservation`) {
		t.Fatal("list leases script should normalize reserved inactive states")
	}
}

func TestListReservationsScriptSkipsInvalidReservationRows(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listReservationsScript("10.24.0.0")
	if !strings.Contains(script, `Skipped DHCP reservation in scope`) {
		t.Fatal("list reservations script should skip invalid reservation rows")
	}
	if !strings.Contains(script, `Get-TextValue $reservation.Description`) {
		t.Fatal("list reservations script should normalize nullable description")
	}
	if !strings.Contains(script, `Get-ReservationDetails`) {
		t.Fatal("list reservations script should fetch reservation details when list fields are empty")
	}
	if !strings.Contains(script, `Get-MacValue $reservation.ClientId`) {
		t.Fatal("list reservations script should normalize client id separators")
	}
}

func TestListScopeDetailsScriptAggregatesScopeDetails(t *testing.T) {
	provider := NewPowerShellProvider()
	script := provider.listScopeDetailsScript("10.24.0.0")
	for _, want := range []string{
		`Get-DhcpServerv4ExclusionRange`,
		`Get-DhcpServerv4Lease`,
		`Get-DhcpServerv4Reservation`,
		`exclusions = $exclusions`,
		`leases = $leases`,
		`reservations = $reservations`,
		`Get-MacValue $lease.ClientId`,
		`Get-MacValue $reservation.ClientId`,
		`Get-LeaseState $lease.AddressState`,
		`inactivereservation`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("details script should contain %q: %s", want, script)
		}
	}
}

func TestReleaseLeaseRemovesByIPAddress(t *testing.T) {
	script := releaseLeaseScript("10.24.0.20")
	if strings.Contains(script, `Remove-DhcpServerv4Lease -ScopeId`) {
		t.Fatalf("release lease should not combine scope id and ip address: %s", script)
	}
	if !strings.Contains(script, `Remove-DhcpServerv4Lease -IPAddress '10.24.0.20' -Confirm:$false -ErrorAction Stop`) {
		t.Fatalf("release lease should use ip address parameter set: %s", script)
	}
}

func TestValidateIPv4RangeInScope(t *testing.T) {
	if err := validateIPv4RangeInScope("10.24.0.0", "255.255.255.0", "10.24.0.10", "10.24.0.20"); err != nil {
		t.Fatalf("expected range to be valid: %v", err)
	}
	if err := validateIPv4RangeInScope("10.24.0.0", "255.255.255.0", "10.24.1.10", "10.24.1.20"); err == nil {
		t.Fatal("expected range outside subnet to fail")
	}
	if err := validateIPv4RangeInScope("10.24.0.0", "255.255.255.0", "10.24.0.20", "10.24.0.10"); err == nil {
		t.Fatal("expected descending range to fail")
	}
}

func TestCreateScopeRejectsInvalidRangeBeforePowerShell(t *testing.T) {
	provider := NewPowerShellProvider()
	_, err := provider.CreateScope(t.Context(), Scope{
		Name:           "Office",
		Subnet:         "10.24.0.0/24",
		DefaultGateway: "10.24.1.1",
		StartRange:     "10.24.1.10",
		EndRange:       "10.24.1.20",
	})
	if err == nil || !strings.Contains(err.Error(), "must belong to the scope subnet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateScopeScriptIncludesDescription(t *testing.T) {
	provider := NewPowerShellProvider()
	scope, script, err := provider.createScopeScript(Scope{
		Name:                 "Office",
		Description:          "Office clients",
		Subnet:               "10.24.0.0/24",
		DefaultGateway:       "10.24.0.1",
		StartRange:           "10.24.0.10",
		EndRange:             "10.24.0.20",
		LeaseDurationSeconds: 180120,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(script, `-Description 'Office clients'`) {
		t.Fatalf("create scope script should include description: %s", script)
	}
	if !strings.Contains(script, `Set-DhcpServerv4OptionValue -ScopeId '10.24.0.0' -Router '10.24.0.1'`) {
		t.Fatalf("create scope script should include default gateway: %s", script)
	}
	if !strings.Contains(script, `-LeaseDuration (New-TimeSpan -Seconds 180120)`) {
		t.Fatalf("create scope script should use second-level lease duration: %s", script)
	}
	if strings.Contains(script, `New-TimeSpan -Hours`) {
		t.Fatalf("create scope script should not use hour-level lease duration: %s", script)
	}
	if scope.Subnet != "10.24.0.0/24" {
		t.Fatalf("create scope should normalize subnet to CIDR prefix, got %q", scope.Subnet)
	}
}

func TestCreateScopeScriptUsesOption51ForUnlimitedLease(t *testing.T) {
	provider := NewPowerShellProvider()
	_, script, err := provider.createScopeScript(Scope{
		Name:                 "Office",
		Subnet:               "10.24.0.0/24",
		DefaultGateway:       "10.24.0.1",
		StartRange:           "10.24.0.10",
		EndRange:             "10.24.0.20",
		LeaseDurationSeconds: -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(script, `New-TimeSpan -Seconds 4294967295`) {
		t.Fatalf("unlimited lease should not use LeaseDuration TimeSpan: %s", script)
	}
	if !strings.Contains(script, `Set-DhcpServerv4OptionValue -ScopeId '10.24.0.0' -OptionId 51 -Value 4294967295 -ErrorAction Stop`) {
		t.Fatalf("unlimited lease should use DHCP option 51: %s", script)
	}
	if strings.Count(script, `Import-Module DhcpServer -ErrorAction Stop`) != 1 {
		t.Fatalf("create scope should run as one PowerShell script: %s", script)
	}
}

func TestUpdateScopeScriptUsesOption51ForUnlimitedLease(t *testing.T) {
	provider := NewPowerShellProvider()
	_, script, err := provider.updateScopeScript(Scope{
		ID:                   "10.24.0.0",
		Name:                 "Office",
		Description:          "Office clients",
		Subnet:               "10.24.0.0/24",
		DefaultGateway:       "10.24.0.1",
		StartRange:           "10.24.0.10",
		EndRange:             "10.24.0.20",
		LeaseDurationSeconds: -1,
		State:                "Active",
		ChangedFields:        []string{"name", "description", "gateway", "lease", "range"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(script, `New-TimeSpan -Seconds 4294967295`) {
		t.Fatalf("unlimited lease should not use LeaseDuration TimeSpan: %s", script)
	}
	for _, want := range []string{
		`Set-DhcpServerv4Scope -ScopeId '10.24.0.0' -Name 'Office' -Description 'Office clients' -StartRange '10.24.0.10' -EndRange '10.24.0.20' -ErrorAction Stop`,
		`Set-DhcpServerv4OptionValue -ScopeId '10.24.0.0' -OptionId 51 -Value 4294967295 -ErrorAction Stop`,
		`Set-DhcpServerv4OptionValue -ScopeId '10.24.0.0' -Router '10.24.0.1' -ErrorAction Stop`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("update scope script should contain %q: %s", want, script)
		}
	}
	if strings.Count(script, `Import-Module DhcpServer -ErrorAction Stop`) != 1 {
		t.Fatalf("update scope should run as one PowerShell script: %s", script)
	}
}

func TestCreateScopeRejectsMissingLeaseDurationSeconds(t *testing.T) {
	provider := NewPowerShellProvider()
	_, _, err := provider.createScopeScript(Scope{
		Name:           "Office",
		Subnet:         "10.24.0.0/24",
		DefaultGateway: "10.24.0.1",
		StartRange:     "10.24.0.10",
		EndRange:       "10.24.0.20",
	})
	if err == nil || !strings.Contains(err.Error(), "lease duration seconds") {
		t.Fatalf("expected missing lease duration seconds to fail, got %v", err)
	}
}

func TestParseScopeSubnetRejectsDottedMask(t *testing.T) {
	_, _, err := parseScopeSubnet("10.24.0.0/255.255.255.0")
	if err == nil || !strings.Contains(err.Error(), "valid IPv4 CIDR") {
		t.Fatalf("expected dotted mask subnet to fail, got %v", err)
	}
}

func TestReservationValidationRejectsInvalidIP(t *testing.T) {
	provider := NewPowerShellProvider()
	_, err := provider.CreateReservation(t.Context(), Reservation{
		ScopeID: "10.24.0.0",
		IP:      "not-ip",
		MAC:     "00-11-22-33-44-55",
		Name:    "client-1",
	})
	if err == nil || !strings.Contains(err.Error(), "reservation ip must be an IPv4 address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateReservationScriptSetsDHCPType(t *testing.T) {
	script := createReservationScript(Reservation{
		ScopeID: "10.24.0.0",
		IP:      "10.24.0.20",
		MAC:     "001122334455",
		Name:    "client-1",
	})
	if !strings.Contains(script, `Add-DhcpServerv4Reservation -ScopeId '10.24.0.0' -IPAddress '10.24.0.20' -ClientId '001122334455' -Name 'client-1' -Description '' -Type 'dhcp' -ErrorAction Stop`) {
		t.Fatalf("create reservation script should set dhcp type: %s", script)
	}
}

func TestUpdateReservationUsesSetReservation(t *testing.T) {
	script := updateReservationScript(
		Reservation{ScopeID: "10.24.0.0", IP: "10.24.0.20"},
		Reservation{ScopeID: "10.24.0.0", IP: "10.24.0.21", MAC: "001122334455", Name: "client-1"},
	)
	if strings.Contains(script, `Remove-DhcpServerv4Reservation`) || strings.Contains(script, `Add-DhcpServerv4Reservation`) {
		t.Fatalf("reservation update should not delete and recreate: %s", script)
	}
	if !strings.Contains(script, `Set-DhcpServerv4Reservation -IPAddress '10.24.0.21' -Name 'client-1' -Description '' -Type 'dhcp' -ErrorAction Stop`) {
		t.Fatalf("reservation update should use Set-DhcpServerv4Reservation with dhcp type: %s", script)
	}
}

func TestDeleteReservationRemovesByIPAddress(t *testing.T) {
	script := deleteReservationScript("10.24.0.20")
	if strings.Contains(script, `Remove-DhcpServerv4Reservation -ScopeId`) {
		t.Fatalf("reservation delete should not combine scope id and ip address: %s", script)
	}
	if !strings.Contains(script, `Remove-DhcpServerv4Reservation -IPAddress '10.24.0.20' -Confirm:$false -ErrorAction Stop`) {
		t.Fatalf("reservation delete should use ip address parameter set: %s", script)
	}
}

func TestPowerShellCommandUsesUTF8Output(t *testing.T) {
	script := powerShellCommandScript("C:\\Temp\\zonelease-dhcp.ps1")
	if !strings.Contains(script, `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8`) {
		t.Fatalf("powershell command should force UTF-8 output: %s", script)
	}
}

func TestClassifyPowerShellError(t *testing.T) {
	cases := map[string]string{
		"Import-Module : The specified module 'DhcpServer' was not loaded": "DHCP PowerShell module is unavailable",
		"Access is denied":     "administrator privileges are required",
		"Scope already exists": "DHCP resource already exists",
		"Cannot find scope":    "DHCP resource was not found",
	}
	for input, want := range cases {
		if got := classifyPowerShellError(input); got != want {
			t.Fatalf("classifyPowerShellError(%q) = %q, want %q", input, got, want)
		}
	}
}
