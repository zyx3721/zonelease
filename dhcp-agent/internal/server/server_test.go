package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"zonelease/dhcp-agent/internal/config"
	"zonelease/dhcp-agent/internal/dhcp"
)

type fakeProvider struct {
	scopeID     string
	scope       dhcp.Scope
	leaseIP     string
	reservation dhcp.Reservation
	update      dhcp.ReservationUpdate
	exclusion   dhcp.Exclusion
	active      bool
	probed      bool
}

func (p *fakeProvider) Probe(context.Context) error {
	p.probed = true
	return nil
}

func (p *fakeProvider) ListScopes(context.Context) ([]dhcp.Scope, error) {
	return []dhcp.Scope{{ID: "10.24.0.0", Name: "Office"}}, nil
}

func (p *fakeProvider) CreateScope(_ context.Context, scope dhcp.Scope) (dhcp.Scope, error) {
	p.scopeID = scope.ID
	p.scope = scope
	return scope, nil
}

func (p *fakeProvider) UpdateScope(_ context.Context, scope dhcp.Scope) (dhcp.Scope, error) {
	p.scopeID = scope.ID
	p.scope = scope
	return scope, nil
}

func (p *fakeProvider) SetScopeState(_ context.Context, scopeID string, active bool) error {
	p.scopeID = scopeID
	p.active = active
	return nil
}

func (p *fakeProvider) DeleteScope(_ context.Context, scopeID string) error {
	p.scopeID = scopeID
	return nil
}

func (p *fakeProvider) ListExclusions(_ context.Context, scopeID string) ([]dhcp.Exclusion, error) {
	p.scopeID = scopeID
	return []dhcp.Exclusion{{ID: scopeID + "|10.24.0.30|10.24.0.40", ScopeID: scopeID, StartIP: "10.24.0.30", EndIP: "10.24.0.40"}}, nil
}

func (p *fakeProvider) CreateExclusion(_ context.Context, exclusion dhcp.Exclusion) (dhcp.Exclusion, error) {
	p.exclusion = exclusion
	return exclusion, nil
}

func (p *fakeProvider) DeleteExclusion(_ context.Context, scopeID, startIP, endIP string) error {
	p.exclusion = dhcp.Exclusion{ScopeID: scopeID, StartIP: startIP, EndIP: endIP}
	return nil
}

func (p *fakeProvider) ListLeases(_ context.Context, scopeID string) ([]dhcp.Lease, error) {
	p.scopeID = scopeID
	return []dhcp.Lease{{ID: scopeID + "|10.24.0.10", ScopeID: scopeID, IP: "10.24.0.10"}}, nil
}

func (p *fakeProvider) ReleaseLease(_ context.Context, scopeID, ip string) error {
	p.scopeID = scopeID
	p.leaseIP = ip
	return nil
}

func (p *fakeProvider) ListReservations(_ context.Context, scopeID string) ([]dhcp.Reservation, error) {
	p.scopeID = scopeID
	return []dhcp.Reservation{{ID: scopeID + "|10.24.0.20", ScopeID: scopeID, IP: "10.24.0.20"}}, nil
}

func (p *fakeProvider) CreateReservation(_ context.Context, reservation dhcp.Reservation) (dhcp.Reservation, error) {
	p.reservation = reservation
	return reservation, nil
}

func (p *fakeProvider) UpdateReservation(_ context.Context, update dhcp.ReservationUpdate) (dhcp.Reservation, error) {
	p.update = update
	p.reservation = update.New
	return update.New, nil
}

func (p *fakeProvider) DeleteReservation(_ context.Context, scopeID, ip string) error {
	p.scopeID = scopeID
	p.leaseIP = ip
	return nil
}

func TestListLeasesRouteUsesScopeIDFromPath(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodGet, "/dhcp/scopes/10.24.0.0/leases", nil, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != "10.24.0.0" {
		t.Fatalf("unexpected scope id: %s", provider.scopeID)
	}
}

func TestProbeRouteUsesProviderProbe(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodGet, "/dhcp/probe", nil, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !provider.probed {
		t.Fatal("expected probe to call provider probe")
	}
}

func TestReleaseLeaseRouteUsesScopeAndIPFromPath(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodDelete, "/dhcp/scopes/10.24.0.0/leases/10.24.0.10", nil, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != "10.24.0.0" || provider.leaseIP != "10.24.0.10" {
		t.Fatalf("unexpected release target: %s %s", provider.scopeID, provider.leaseIP)
	}
}

func TestCreateReservationRouteUsesRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	body := dhcp.Reservation{ScopeID: "10.24.0.0", IP: "10.24.0.20", MAC: "00-11-22-33-44-55", Name: "client-1"}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dhcp/reservations", body, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.reservation.IP != body.IP || provider.reservation.ScopeID != body.ScopeID {
		t.Fatalf("unexpected reservation: %+v", provider.reservation)
	}
}

func TestUpdateScopeRouteUsesPathScopeID(t *testing.T) {
	provider := &fakeProvider{}
	body := dhcp.Scope{
		Name: "Office", Description: "Office clients", Subnet: "10.24.0.0/24",
		StartRange: "10.24.0.10", EndRange: "10.24.0.20", LeaseDurationHours: 24,
		ChangedFields: []string{"description"},
	}
	recorder := executeAgentRequest(t, provider, http.MethodPut, "/dhcp/scopes/10.24.0.0", body, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != "10.24.0.0" || provider.scope.ID != "10.24.0.0" {
		t.Fatalf("unexpected scope update: %+v", provider.scope)
	}
	if provider.scope.Description != body.Description || len(provider.scope.ChangedFields) != 1 || provider.scope.ChangedFields[0] != "description" {
		t.Fatalf("unexpected scope update body: %+v", provider.scope)
	}
}

func TestReleaseLeaseBodyRouteUsesRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	body := dhcp.LeaseRef{ScopeID: "10.24.0.0", IP: "10.24.0.10"}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dhcp/leases/release", body, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != body.ScopeID || provider.leaseIP != body.IP {
		t.Fatalf("unexpected lease release: %s %s", provider.scopeID, provider.leaseIP)
	}
}

func TestUpdateReservationBodyRouteUsesRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	body := dhcp.ReservationUpdate{
		Old: dhcp.Reservation{ScopeID: "10.24.0.0", IP: "10.24.0.20", MAC: "00-11-22-33-44-55", Name: "old"},
		New: dhcp.Reservation{ScopeID: "10.24.0.0", IP: "10.24.0.21", MAC: "00-11-22-33-44-66", Name: "new"},
	}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dhcp/reservations/update", body, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.update.New.IP != body.New.IP || provider.update.Old.IP != body.Old.IP {
		t.Fatalf("unexpected reservation update: %+v", provider.update)
	}
}

func TestDeleteReservationBodyRouteUsesRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	body := dhcp.LeaseRef{ScopeID: "10.24.0.0", IP: "10.24.0.20"}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dhcp/reservations/delete", body, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != body.ScopeID || provider.leaseIP != body.IP {
		t.Fatalf("unexpected reservation delete: %s %s", provider.scopeID, provider.leaseIP)
	}
}

func TestActivateScopeRouteUsesPathScopeID(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dhcp/scopes/10.24.0.0/activate", nil, "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if provider.scopeID != "10.24.0.0" || !provider.active {
		t.Fatalf("unexpected scope state call: %s active=%v", provider.scopeID, provider.active)
	}
}

func TestMiddlewareRejectsInvalidAPIKey(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodGet, "/dhcp/scopes", nil, "wrong")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", recorder.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Success || body.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func executeAgentRequest(t *testing.T, provider *fakeProvider, method, path string, body any, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	recorder := httptest.NewRecorder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	New(config.Config{APIKey: "secret"}, provider, logger).Routes().ServeHTTP(recorder, req)
	return recorder
}
