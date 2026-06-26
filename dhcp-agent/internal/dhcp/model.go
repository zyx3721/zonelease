package dhcp

import "context"

type Scope struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Subnet               string   `json:"subnet"`
	DefaultGateway       string   `json:"defaultGateway,omitempty"`
	StartRange           string   `json:"startRange"`
	EndRange             string   `json:"endRange"`
	LeaseDurationHours   int      `json:"leaseDurationHours"`
	LeaseDurationSeconds int      `json:"leaseDurationSeconds,omitempty"`
	State                string   `json:"state"`
	ServerID             string   `json:"serverId"`
	OldStartRange        string   `json:"oldStartRange,omitempty"`
	OldEndRange          string   `json:"oldEndRange,omitempty"`
	ChangedFields        []string `json:"changedFields,omitempty"`
}

type Exclusion struct {
	ID      string `json:"id"`
	ScopeID string `json:"scopeId"`
	StartIP string `json:"startIp"`
	EndIP   string `json:"endIp"`
}

type Lease struct {
	ID        string `json:"id"`
	ScopeID   string `json:"scopeId"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	State     string `json:"state"`
	ExpiresAt string `json:"expiresAt"`
}

type Reservation struct {
	ID          string `json:"id"`
	ScopeID     string `json:"scopeId"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ScopeState struct {
	ScopeID string `json:"scopeId"`
	Active  bool   `json:"active"`
}

type LeaseRef struct {
	ScopeID string `json:"scopeId"`
	IP      string `json:"ip"`
}

type ExclusionRef struct {
	ScopeID string `json:"scopeId"`
	StartIP string `json:"startIp"`
	EndIP   string `json:"endIp"`
}

type ReservationUpdate struct {
	Old Reservation `json:"old"`
	New Reservation `json:"new"`
}

type Provider interface {
	Probe(context.Context) error
	ListScopes(context.Context) ([]Scope, error)
	CreateScope(context.Context, Scope) (Scope, error)
	UpdateScope(context.Context, Scope) (Scope, error)
	SetScopeState(context.Context, string, bool) error
	DeleteScope(context.Context, string) error
	ListExclusions(context.Context, string) ([]Exclusion, error)
	CreateExclusion(context.Context, Exclusion) (Exclusion, error)
	DeleteExclusion(context.Context, string, string, string) error
	ListLeases(context.Context, string) ([]Lease, error)
	ReleaseLease(context.Context, string, string) error
	ListReservations(context.Context, string) ([]Reservation, error)
	CreateReservation(context.Context, Reservation) (Reservation, error)
	UpdateReservation(context.Context, ReservationUpdate) (Reservation, error)
	DeleteReservation(context.Context, string, string) error
}
