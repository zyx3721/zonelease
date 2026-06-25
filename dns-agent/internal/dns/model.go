package dns

import "context"

type Zone struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Reverse       bool   `json:"reverse"`
	DynamicUpdate string `json:"dynamicUpdate"`
	ServerID      string `json:"serverId"`
}

type Record struct {
	ID        string `json:"id"`
	ZoneID    string `json:"zoneId"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	TTL       int    `json:"ttl"`
	CreatePTR bool   `json:"createPtr,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type CreateRecordResult struct {
	Created  bool     `json:"created"`
	PTR      *Record  `json:"ptr,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type RecordUpdate struct {
	Old Record `json:"old"`
	New Record `json:"new"`
}

type Provider interface {
	ListZones(context.Context) ([]Zone, error)
	CreateZone(context.Context, Zone) error
	DeleteZone(context.Context, string) error
	ListRecords(context.Context, string) ([]Record, error)
	CreateRecord(context.Context, string, Record) (CreateRecordResult, error)
	UpdateRecord(context.Context, string, RecordUpdate) (CreateRecordResult, error)
	DeleteRecord(context.Context, string, Record) error
}
