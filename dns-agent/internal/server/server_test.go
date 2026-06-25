package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"zonelease/dns-agent/internal/config"
	"zonelease/dns-agent/internal/dns"
)

type fakeProvider struct {
	zone   string
	record dns.Record
	update dns.RecordUpdate
}

func (p *fakeProvider) ListZones(context.Context) ([]dns.Zone, error) {
	return nil, nil
}

func (p *fakeProvider) CreateZone(context.Context, dns.Zone) error {
	return nil
}

func (p *fakeProvider) DeleteZone(context.Context, string) error {
	return nil
}

func (p *fakeProvider) ListRecords(_ context.Context, zone string) ([]dns.Record, error) {
	p.zone = zone
	return []dns.Record{{Name: "www", Type: "A", Value: "10.10.10.10"}}, nil
}

func (p *fakeProvider) CreateRecord(_ context.Context, zone string, record dns.Record) (dns.CreateRecordResult, error) {
	p.zone = zone
	p.record = record
	return dns.CreateRecordResult{Created: true}, nil
}

func (p *fakeProvider) UpdateRecord(_ context.Context, zone string, update dns.RecordUpdate) (dns.CreateRecordResult, error) {
	p.zone = zone
	p.update = update
	return dns.CreateRecordResult{Created: true}, nil
}

func (p *fakeProvider) DeleteRecord(_ context.Context, zone string, record dns.Record) error {
	p.zone = zone
	p.record = record
	return nil
}

func TestCreateRecordBodyRouteUsesZoneFromRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dns/records/create", map[string]any{
		"zone":   "youtube.com",
		"record": map[string]any{"name": "www", "type": "A", "value": "10.10.10.10", "ttl": 3600},
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.zone != "youtube.com" {
		t.Fatalf("unexpected zone: %s", provider.zone)
	}
	if provider.record.Name != "www" || provider.record.Type != "A" || provider.record.Value != "10.10.10.10" {
		t.Fatalf("unexpected record: %+v", provider.record)
	}
}

func TestDeleteRecordBodyRouteUsesZoneFromRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dns/records/delete", map[string]any{
		"zone":   "youtube.com",
		"record": map[string]any{"name": "www", "type": "A", "value": "10.10.10.10", "createPtr": true},
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.zone != "youtube.com" {
		t.Fatalf("unexpected zone: %s", provider.zone)
	}
	if !provider.record.CreatePTR {
		t.Fatal("expected createPtr flag to be passed through")
	}
}

func TestUpdateRecordBodyRouteUsesZoneFromRequestBody(t *testing.T) {
	provider := &fakeProvider{}
	recorder := executeAgentRequest(t, provider, http.MethodPost, "/dns/records/update", map[string]any{
		"zone": "youtube.com",
		"update": map[string]any{
			"old": map[string]any{"name": "www", "type": "A", "value": "10.10.10.10"},
			"new": map[string]any{"name": "www", "type": "A", "value": "10.10.10.11"},
		},
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.zone != "youtube.com" {
		t.Fatalf("unexpected zone: %s", provider.zone)
	}
	if provider.update.New.Value != "10.10.10.11" {
		t.Fatalf("unexpected update: %+v", provider.update)
	}
}

func executeAgentRequest(t *testing.T, provider dns.Provider, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{AllowAnonymous: true}, provider, slog.Default())
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Routes().ServeHTTP(recorder, request)
	return recorder
}
