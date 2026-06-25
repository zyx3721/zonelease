package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/strutil"
)

func (s *Store) ListDNSZones(ctx context.Context) ([]domain.DNSZone, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, type, reverse, dynamic_update, server_id::text, last_synced_at, sync_status, last_error
		FROM dns_zones ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DNSZone{}
	for rows.Next() {
		var item domain.DNSZone
		var lastSyncedAt *time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Reverse, &item.DynamicUpdate, &item.ServerID, &lastSyncedAt, &item.SyncStatus, &item.LastError); err != nil {
			return nil, err
		}
		if lastSyncedAt != nil {
			item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetDNSZone(ctx context.Context, id string) (domain.DNSZone, error) {
	var item domain.DNSZone
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, type, reverse, dynamic_update, server_id::text, last_synced_at, sync_status, last_error
		FROM dns_zones WHERE id=$1
	`, id).Scan(&item.ID, &item.Name, &item.Type, &item.Reverse, &item.DynamicUpdate, &item.ServerID, &lastSyncedAt, &item.SyncStatus, &item.LastError)
	if err == pgx.ErrNoRows {
		return item, ErrNotFound
	}
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, err
}

func (s *Store) DNSReverseZoneExists(ctx context.Context, serverID, name string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dns_zones
			WHERE server_id=$1 AND lower(name)=lower($2) AND reverse=true
		)
	`, serverID, name).Scan(&exists)
	return exists, err
}

func (s *Store) ListDNSRecords(ctx context.Context) ([]domain.DNSRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, zone_id, name, type, value, ttl, create_ptr, updated_at, last_synced_at
		FROM dns_records ORDER BY name, type, value
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DNSRecord{}
	for rows.Next() {
		item, err := scanDNSRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListDNSRecordsByZone(ctx context.Context, zoneID string) ([]domain.DNSRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, zone_id, name, type, value, ttl, create_ptr, updated_at, last_synced_at
		FROM dns_records WHERE zone_id=$1 ORDER BY name, type, value
	`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DNSRecord{}
	for rows.Next() {
		item, err := scanDNSRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetDNSRecord(ctx context.Context, id string) (domain.DNSRecord, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, zone_id, name, type, value, ttl, create_ptr, updated_at, last_synced_at
		FROM dns_records WHERE id=$1
	`, id)
	item, err := scanDNSRecord(row)
	if err == pgx.ErrNoRows {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Store) UpsertDNSZone(ctx context.Context, item domain.DNSZone) (domain.DNSZone, error) {
	if item.ID == "" {
		item.ID = DNSZoneID(item.ServerID, item.Name)
	}
	var lastSyncedAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dns_zones(id, name, type, reverse, dynamic_update, server_id, sync_status, last_synced_at, last_error)
		VALUES($1, $2, $3, $4, $5, $6, 'synced', now(), '')
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			type=EXCLUDED.type,
			reverse=EXCLUDED.reverse,
			dynamic_update=EXCLUDED.dynamic_update,
			server_id=EXCLUDED.server_id,
			sync_status='synced',
			last_synced_at=now(),
			last_error='',
			updated_at=now()
		RETURNING last_synced_at
	`, item.ID, item.Name, strutil.FirstNonEmpty(item.Type, "Primary"), item.Reverse, strutil.FirstNonEmpty(item.DynamicUpdate, "None"), item.ServerID).Scan(&lastSyncedAt)
	item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	item.SyncStatus = "synced"
	item.LastError = ""
	return item, err
}

func (s *Store) ReplaceDNSZones(ctx context.Context, serverID string, zones []domain.DNSZone) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keep := make([]string, 0, len(zones))
	for _, zone := range zones {
		zone.ServerID = serverID
		if zone.ID == "" {
			zone.ID = DNSZoneID(serverID, zone.Name)
		}
		if _, err := upsertDNSZone(ctx, tx, zone); err != nil {
			return err
		}
		keep = append(keep, zone.ID)
	}
	if len(keep) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM dns_zones WHERE server_id=$1`, serverID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM dns_zones WHERE server_id=$1 AND NOT (id = ANY($2))`, serverID, keep); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteDNSZone(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dns_zones WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReplaceDNSZoneRecords(ctx context.Context, zone domain.DNSZone, records []domain.DNSRecord) error {
	if zone.ID == "" {
		zone.ID = DNSZoneID(zone.ServerID, zone.Name)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := upsertDNSZone(ctx, tx, zone); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dns_records WHERE zone_id=$1`, zone.ID); err != nil {
		return err
	}
	for _, record := range records {
		record.ZoneID = zone.ID
		if record.ID == "" {
			record.ID = DNSRecordID(zone.ServerID, zone.Name, record.Type, record.Name, record.Value)
		}
		if record.TTL <= 0 {
			record.TTL = 3600
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO dns_records(id, zone_id, name, type, value, ttl, create_ptr, last_synced_at, updated_at)
			VALUES($1, $2, $3, $4, $5, $6, $7, now(), now())
		`, record.ID, record.ZoneID, record.Name, record.Type, record.Value, record.TTL, record.CreatePTR); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) UpsertDNSRecord(ctx context.Context, record domain.DNSRecord) (domain.DNSRecord, error) {
	if record.TTL <= 0 {
		record.TTL = 3600
	}
	var updatedAt time.Time
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dns_records(id, zone_id, name, type, value, ttl, create_ptr, last_synced_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			zone_id=EXCLUDED.zone_id,
			name=EXCLUDED.name,
			type=EXCLUDED.type,
			value=EXCLUDED.value,
			ttl=EXCLUDED.ttl,
			create_ptr=EXCLUDED.create_ptr,
			last_synced_at=now(),
			updated_at=now()
		RETURNING updated_at, last_synced_at
	`, record.ID, record.ZoneID, record.Name, record.Type, record.Value, record.TTL, record.CreatePTR).Scan(&updatedAt, &lastSyncedAt)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	if lastSyncedAt != nil {
		record.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return record, err
}

func (s *Store) DeleteDNSRecord(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dns_records WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type dnsZoneExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func upsertDNSZone(ctx context.Context, exec dnsZoneExecutor, item domain.DNSZone) (pgconn.CommandTag, error) {
	return exec.Exec(ctx, `
		INSERT INTO dns_zones(id, name, type, reverse, dynamic_update, server_id, sync_status, last_synced_at, last_error)
		VALUES($1, $2, $3, $4, $5, $6, 'synced', now(), '')
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			type=EXCLUDED.type,
			reverse=EXCLUDED.reverse,
			dynamic_update=EXCLUDED.dynamic_update,
			server_id=EXCLUDED.server_id,
			sync_status='synced',
			last_synced_at=now(),
			last_error='',
			updated_at=now()
	`, item.ID, item.Name, strutil.FirstNonEmpty(item.Type, "Primary"), item.Reverse, strutil.FirstNonEmpty(item.DynamicUpdate, "None"), item.ServerID)
}

func (s *Store) MarkDNSZoneSyncError(ctx context.Context, zoneID, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dns_zones SET sync_status='failed', last_error=$2, updated_at=now() WHERE id=$1
	`, zoneID, message)
	return err
}

type dnsRecordScanner interface {
	Scan(dest ...any) error
}

func scanDNSRecord(row dnsRecordScanner) (domain.DNSRecord, error) {
	var item domain.DNSRecord
	var updatedAt time.Time
	var lastSyncedAt *time.Time
	if err := row.Scan(&item.ID, &item.ZoneID, &item.Name, &item.Type, &item.Value, &item.TTL, &item.CreatePTR, &updatedAt, &lastSyncedAt); err != nil {
		return item, err
	}
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, nil
}
