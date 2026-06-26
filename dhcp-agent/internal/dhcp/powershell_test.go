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
	_, script, err := provider.createScopeScript(Scope{
		Name:           "Office",
		Description:    "Office clients",
		Subnet:         "10.24.0.0/24",
		DefaultGateway: "10.24.0.1",
		StartRange:     "10.24.0.10",
		EndRange:       "10.24.0.20",
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
