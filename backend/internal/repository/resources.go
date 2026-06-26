package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) State(ctx context.Context) (domain.State, error) {
	state := domain.State{
		Servers:      []domain.Server{},
		Zones:        []domain.DNSZone{},
		Records:      []domain.DNSRecord{},
		Scopes:       []domain.DHCPScope{},
		Exclusions:   []domain.DHCPExclusion{},
		Leases:       []domain.DHCPLease{},
		Reservations: []domain.DHCPReservation{},
		Audit:        []domain.AuditEntry{},
	}
	var err error
	if state.Servers, err = s.ListServers(ctx); err != nil {
		return state, err
	}
	if state.Zones, err = s.ListDNSZones(ctx); err != nil {
		return state, err
	}
	if state.Records, err = s.ListDNSRecords(ctx); err != nil {
		return state, err
	}
	if state.Scopes, err = s.ListScopes(ctx); err != nil {
		return state, err
	}
	if state.Exclusions, err = s.ListExclusions(ctx); err != nil {
		return state, err
	}
	if state.Leases, err = s.ListLeases(ctx); err != nil {
		return state, err
	}
	if state.Reservations, err = s.ListReservations(ctx); err != nil {
		return state, err
	}
	if state.Audit, err = s.ListAudit(ctx); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Store) ListServers(ctx context.Context) ([]domain.Server, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, host, role, agent_url, api_key, tls_insecure, status, failure_count, COALESCE(last_checked, created_at)
		FROM servers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Server{}
	for rows.Next() {
		var item domain.Server
		var lastChecked time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Host, &item.Role, &item.AgentURL, &item.APIKey, &item.TLSInsecure, &item.Status, &item.FailureCount, &lastChecked); err != nil {
			return nil, err
		}
		item.LastChecked = lastChecked.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetServer(ctx context.Context, id string) (domain.Server, error) {
	var item domain.Server
	var lastChecked time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, host, role, agent_url, api_key, tls_insecure, status, failure_count, COALESCE(last_checked, created_at)
		FROM servers WHERE id=$1
	`, id).Scan(&item.ID, &item.Name, &item.Host, &item.Role, &item.AgentURL, &item.APIKey, &item.TLSInsecure, &item.Status, &item.FailureCount, &lastChecked)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	item.LastChecked = lastChecked.UTC().Format(time.RFC3339Nano)
	return item, err
}

func (s *Store) GetServerByName(ctx context.Context, name string) (domain.Server, error) {
	var item domain.Server
	var lastChecked time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, host, role, agent_url, api_key, tls_insecure, status, failure_count, COALESCE(last_checked, created_at)
		FROM servers WHERE name=$1
	`, name).Scan(&item.ID, &item.Name, &item.Host, &item.Role, &item.AgentURL, &item.APIKey, &item.TLSInsecure, &item.Status, &item.FailureCount, &lastChecked)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	item.LastChecked = lastChecked.UTC().Format(time.RFC3339Nano)
	return item, err
}

func (s *Store) GetServerByAgentURL(ctx context.Context, agentURL string) (domain.Server, error) {
	var item domain.Server
	var lastChecked time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, host, role, agent_url, api_key, tls_insecure, status, failure_count, COALESCE(last_checked, created_at)
		FROM servers WHERE agent_url=$1
	`, agentURL).Scan(&item.ID, &item.Name, &item.Host, &item.Role, &item.AgentURL, &item.APIKey, &item.TLSInsecure, &item.Status, &item.FailureCount, &lastChecked)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	item.LastChecked = lastChecked.UTC().Format(time.RFC3339Nano)
	return item, err
}

func (s *Store) CreateServer(ctx context.Context, item domain.Server) (domain.Server, error) {
	var lastChecked time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO servers(name, host, role, agent_url, api_key, tls_insecure, status, last_checked)
		VALUES($1, $2, $3, $4, $5, $6, 'Online', now())
		RETURNING id::text, status, last_checked
	`, item.Name, item.Host, item.Role, item.AgentURL, item.APIKey, item.TLSInsecure).Scan(&item.ID, &item.Status, &lastChecked)
	item.LastChecked = lastChecked.UTC().Format(time.RFC3339Nano)
	return item, err
}

func (s *Store) DeleteServer(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM servers WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ServerHealthUpdate struct {
	PreviousStatus string
	Status         string
	FailureCount   int
}

func (u ServerHealthUpdate) BecameOffline() bool {
	return u.Status == "Offline" && u.PreviousStatus != "Offline"
}

func (s *Store) UpdateServerHealth(ctx context.Context, id, status string, offlineFailureLimit int) (ServerHealthUpdate, error) {
	if offlineFailureLimit <= 0 {
		offlineFailureLimit = 3
	}
	var result ServerHealthUpdate
	if status == "Online" {
		err := s.pool.QueryRow(ctx, `
			WITH previous AS (
				SELECT status FROM servers WHERE id=$1
			), updated AS (
				UPDATE servers
				SET status='Online', failure_count=0, last_checked=now(), updated_at=now()
				FROM previous
				WHERE servers.id=$1
				RETURNING previous.status AS previous_status, servers.status, servers.failure_count
			)
			SELECT previous_status, status, failure_count FROM updated
		`, id).Scan(&result.PreviousStatus, &result.Status, &result.FailureCount)
		if errors.Is(err, pgx.ErrNoRows) {
			return result, ErrNotFound
		}
		return result, err
	}
	err := s.pool.QueryRow(ctx, `
		WITH previous AS (
			SELECT status FROM servers WHERE id=$1
		), updated AS (
			UPDATE servers
			SET failure_count=failure_count+1,
				status=CASE WHEN failure_count + 1 >= $3 THEN $2 ELSE servers.status END,
				last_checked=now(),
				updated_at=now()
			FROM previous
			WHERE servers.id=$1
			RETURNING previous.status AS previous_status, servers.status, servers.failure_count
		)
		SELECT previous_status, status, failure_count FROM updated
	`, id, status, offlineFailureLimit).Scan(&result.PreviousStatus, &result.Status, &result.FailureCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	return result, err
}

func (s *Store) ListScopes(ctx context.Context) ([]domain.DHCPScope, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, description, subnet, default_gateway, start_range, end_range, lease_duration_hours, lease_duration_seconds, state, server_id::text, external_id, last_synced_at, sync_status, last_error
		FROM dhcp_scopes ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DHCPScope{}
	for rows.Next() {
		var item domain.DHCPScope
		var lastSyncedAt *time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Subnet, &item.DefaultGateway, &item.StartRange, &item.EndRange, &item.LeaseDurationHours, &item.LeaseDurationSeconds, &item.State, &item.ServerID, &item.ExternalID, &lastSyncedAt, &item.SyncStatus, &item.LastError); err != nil {
			return nil, err
		}
		normalizeScopeLeaseDuration(&item)
		if lastSyncedAt != nil {
			item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetScope(ctx context.Context, id string) (domain.DHCPScope, error) {
	var item domain.DHCPScope
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, description, subnet, default_gateway, start_range, end_range, lease_duration_hours, lease_duration_seconds, state, server_id::text, external_id, last_synced_at, sync_status, last_error
		FROM dhcp_scopes WHERE id=$1
	`, id).Scan(&item.ID, &item.Name, &item.Description, &item.Subnet, &item.DefaultGateway, &item.StartRange, &item.EndRange, &item.LeaseDurationHours, &item.LeaseDurationSeconds, &item.State, &item.ServerID, &item.ExternalID, &lastSyncedAt, &item.SyncStatus, &item.LastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	normalizeScopeLeaseDuration(&item)
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, err
}

func (s *Store) CreateScope(ctx context.Context, item domain.DHCPScope) (domain.DHCPScope, error) {
	if item.State == "" {
		item.State = "Active"
	}
	normalizeScopeLeaseDuration(&item)
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dhcp_scopes(name, description, subnet, default_gateway, start_range, end_range, lease_duration_hours, lease_duration_seconds, state, server_id, external_id, last_synced_at, sync_status, last_error)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), 'synced', '')
		RETURNING id::text
	`, item.Name, item.Description, item.Subnet, item.DefaultGateway, item.StartRange, item.EndRange, item.LeaseDurationHours, item.LeaseDurationSeconds, item.State, item.ServerID, item.ExternalID).Scan(&item.ID)
	return item, err
}

func (s *Store) UpdateScope(ctx context.Context, item domain.DHCPScope) (domain.DHCPScope, error) {
	normalizeScopeLeaseDuration(&item)
	cmd, err := s.pool.Exec(ctx, `
		UPDATE dhcp_scopes
		SET name=$2,
			description=$3,
			subnet=$4,
			default_gateway=$5,
			start_range=$6,
			end_range=$7,
			lease_duration_hours=$8,
			lease_duration_seconds=$9,
			state=$10,
			external_id=$11,
			updated_at=now()
		WHERE id=$1
	`, item.ID, item.Name, item.Description, item.Subnet, item.DefaultGateway, item.StartRange, item.EndRange, item.LeaseDurationHours, item.LeaseDurationSeconds, item.State, item.ExternalID)
	if err != nil {
		return item, err
	}
	if cmd.RowsAffected() == 0 {
		return item, ErrNotFound
	}
	return s.GetScope(ctx, item.ID)
}

func normalizeScopeLeaseDuration(item *domain.DHCPScope) {
	if item.LeaseDurationSeconds == -1 {
		item.LeaseDurationHours = 0
		return
	}
	if item.LeaseDurationSeconds <= 0 && item.LeaseDurationHours > 0 {
		item.LeaseDurationSeconds = item.LeaseDurationHours * 3600
	}
	if item.LeaseDurationHours <= 0 && item.LeaseDurationSeconds > 0 {
		item.LeaseDurationHours = (item.LeaseDurationSeconds + 3599) / 3600
	}
	if item.LeaseDurationHours <= 0 {
		item.LeaseDurationHours = 24
	}
	if item.LeaseDurationSeconds <= 0 {
		item.LeaseDurationSeconds = item.LeaseDurationHours * 3600
	}
}

func (s *Store) ToggleScope(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE dhcp_scopes
		SET state = CASE WHEN state='Active' THEN 'Inactive' ELSE 'Active' END, updated_at=now()
		WHERE id=$1
	`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteScope(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dhcp_scopes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListExclusions(ctx context.Context) ([]domain.DHCPExclusion, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, scope_id::text, start_ip, end_ip, external_id, last_synced_at FROM dhcp_exclusions ORDER BY start_ip, end_ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DHCPExclusion{}
	for rows.Next() {
		var item domain.DHCPExclusion
		var lastSyncedAt *time.Time
		if err := rows.Scan(&item.ID, &item.ScopeID, &item.StartIP, &item.EndIP, &item.ExternalID, &lastSyncedAt); err != nil {
			return nil, err
		}
		if lastSyncedAt != nil {
			item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetExclusion(ctx context.Context, id string) (domain.DHCPExclusion, error) {
	var item domain.DHCPExclusion
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, scope_id::text, start_ip, end_ip, external_id, last_synced_at
		FROM dhcp_exclusions WHERE id=$1
	`, id).Scan(&item.ID, &item.ScopeID, &item.StartIP, &item.EndIP, &item.ExternalID, &lastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, err
}

func (s *Store) CreateExclusion(ctx context.Context, item domain.DHCPExclusion) (domain.DHCPExclusion, error) {
	item.ExternalID = exclusionExternalID(item)
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dhcp_exclusions(scope_id, start_ip, end_ip, external_id, last_synced_at)
		VALUES($1, $2, $3, $4, now())
		ON CONFLICT (scope_id, start_ip, end_ip) DO UPDATE SET
			external_id=EXCLUDED.external_id,
			last_synced_at=now(),
			updated_at=now()
		RETURNING id::text
	`, item.ScopeID, item.StartIP, item.EndIP, item.ExternalID).Scan(&item.ID)
	return item, err
}

func (s *Store) DeleteExclusion(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dhcp_exclusions WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListLeases(ctx context.Context) ([]domain.DHCPLease, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, scope_id::text, ip, mac, hostname, state, expires_at, external_id, last_synced_at FROM dhcp_leases ORDER BY ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DHCPLease{}
	for rows.Next() {
		var item domain.DHCPLease
		var expiresAt time.Time
		var lastSyncedAt *time.Time
		if err := rows.Scan(&item.ID, &item.ScopeID, &item.IP, &item.MAC, &item.Hostname, &item.State, &expiresAt, &item.ExternalID, &lastSyncedAt); err != nil {
			return nil, err
		}
		item.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
		if lastSyncedAt != nil {
			item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetLease(ctx context.Context, id string) (domain.DHCPLease, error) {
	var item domain.DHCPLease
	var expiresAt time.Time
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, scope_id::text, ip, mac, hostname, state, expires_at, external_id, last_synced_at
		FROM dhcp_leases WHERE id=$1
	`, id).Scan(&item.ID, &item.ScopeID, &item.IP, &item.MAC, &item.Hostname, &item.State, &expiresAt, &item.ExternalID, &lastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	item.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, err
}

func (s *Store) DeleteLease(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dhcp_leases WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkLeaseReservedInactive(ctx context.Context, scopeID, ip string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dhcp_leases
		SET state='ReservedInactive',
			expires_at='9999-12-31 23:59:59+00'::timestamptz
		WHERE scope_id=$1 AND ip=$2
	`, scopeID, ip)
	return err
}

func (s *Store) MarkLeaseReservedInactiveWithHostname(ctx context.Context, scopeID, ip, hostname string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dhcp_leases
		SET hostname=$3,
			state='ReservedInactive',
			expires_at='9999-12-31 23:59:59+00'::timestamptz
		WHERE scope_id=$1 AND ip=$2
	`, scopeID, ip, hostname)
	return err
}

func (s *Store) UpdateLeaseHostnameByScopeIP(ctx context.Context, scopeID, ip, hostname string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dhcp_leases
		SET hostname=$3
		WHERE scope_id=$1 AND ip=$2
	`, scopeID, ip, hostname)
	return err
}

func (s *Store) DeleteLeaseByScopeIP(ctx context.Context, scopeID, ip string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dhcp_leases WHERE scope_id=$1 AND ip=$2`, scopeID, ip)
	return err
}

func (s *Store) ListReservations(ctx context.Context) ([]domain.DHCPReservation, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, scope_id::text, ip, mac, name, description, external_id, last_synced_at FROM dhcp_reservations ORDER BY ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DHCPReservation{}
	for rows.Next() {
		var item domain.DHCPReservation
		var lastSyncedAt *time.Time
		if err := rows.Scan(&item.ID, &item.ScopeID, &item.IP, &item.MAC, &item.Name, &item.Description, &item.ExternalID, &lastSyncedAt); err != nil {
			return nil, err
		}
		if lastSyncedAt != nil {
			item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetReservation(ctx context.Context, id string) (domain.DHCPReservation, error) {
	var item domain.DHCPReservation
	var lastSyncedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, scope_id::text, ip, mac, name, description, external_id, last_synced_at
		FROM dhcp_reservations WHERE id=$1
	`, id).Scan(&item.ID, &item.ScopeID, &item.IP, &item.MAC, &item.Name, &item.Description, &item.ExternalID, &lastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	if lastSyncedAt != nil {
		item.LastSyncedAt = lastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return item, err
}

func (s *Store) CreateReservation(ctx context.Context, item domain.DHCPReservation) (domain.DHCPReservation, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dhcp_reservations(scope_id, ip, mac, name, description)
		VALUES($1, $2, $3, $4, $5)
		RETURNING id::text
	`, item.ScopeID, item.IP, item.MAC, item.Name, item.Description).Scan(&item.ID)
	return item, err
}

func (s *Store) UpdateReservation(ctx context.Context, item domain.DHCPReservation) (domain.DHCPReservation, error) {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE dhcp_reservations
		SET ip=$2,
			mac=$3,
			name=$4,
			description=$5,
			external_id=$6
		WHERE id=$1
	`, item.ID, item.IP, item.MAC, item.Name, item.Description, item.ExternalID)
	if err != nil {
		return item, err
	}
	if cmd.RowsAffected() == 0 {
		return item, ErrNotFound
	}
	return s.GetReservation(ctx, item.ID)
}

func (s *Store) DeleteReservation(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM dhcp_reservations WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
