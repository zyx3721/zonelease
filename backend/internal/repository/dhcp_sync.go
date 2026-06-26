package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/strutil"
)

func (s *Store) ReplaceDHCPScopes(ctx context.Context, serverID string, scopes []domain.DHCPScope) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keepScopeIDs := []string{}
	for _, scope := range scopes {
		scope.ServerID = serverID
		scope.ExternalID = strutil.FirstNonEmpty(scope.ExternalID, scope.ID, scope.Subnet, scope.Name)
		if scope.State == "" {
			scope.State = "Active"
		}
		normalizeScopeLeaseDuration(&scope)
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO dhcp_scopes(name, description, subnet, default_gateway, start_range, end_range, lease_duration_hours, lease_duration_seconds, state, server_id, external_id, last_synced_at, sync_status, last_error)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), 'synced', '')
			ON CONFLICT (server_id, external_id) WHERE external_id <> '' DO UPDATE SET
				name=EXCLUDED.name,
				description=EXCLUDED.description,
				subnet=EXCLUDED.subnet,
				default_gateway=EXCLUDED.default_gateway,
				start_range=EXCLUDED.start_range,
				end_range=EXCLUDED.end_range,
				lease_duration_hours=EXCLUDED.lease_duration_hours,
				lease_duration_seconds=EXCLUDED.lease_duration_seconds,
				state=EXCLUDED.state,
				last_synced_at=now(),
				sync_status='synced',
				last_error='',
				updated_at=now()
			RETURNING id::text
		`, scope.Name, scope.Description, scope.Subnet, scope.DefaultGateway, scope.StartRange, scope.EndRange, scope.LeaseDurationHours, scope.LeaseDurationSeconds, scope.State, serverID, scope.ExternalID).Scan(&id); err != nil {
			return err
		}
		keepScopeIDs = append(keepScopeIDs, id)
	}

	if len(keepScopeIDs) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM dhcp_scopes WHERE server_id=$1`, serverID); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM dhcp_scopes WHERE server_id=$1 AND NOT (id::text = ANY($2))`, serverID, keepScopeIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReplaceDHCPScopeSnapshot(ctx context.Context, serverID string, scope domain.DHCPScope, exclusions []domain.DHCPExclusion, leases []domain.DHCPLease, reservations []domain.DHCPReservation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	scope.ExternalID = strutil.FirstNonEmpty(scope.ExternalID, scope.ID, scope.Subnet, scope.Name)
	if scope.State == "" {
		scope.State = "Active"
	}
	normalizeScopeLeaseDuration(&scope)
	var scopeID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO dhcp_scopes(name, description, subnet, default_gateway, start_range, end_range, lease_duration_hours, lease_duration_seconds, state, server_id, external_id, last_synced_at, sync_status, last_error)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), 'synced', '')
		ON CONFLICT (server_id, external_id) WHERE external_id <> '' DO UPDATE SET
			name=EXCLUDED.name,
			description=EXCLUDED.description,
			subnet=EXCLUDED.subnet,
			default_gateway=EXCLUDED.default_gateway,
			start_range=EXCLUDED.start_range,
			end_range=EXCLUDED.end_range,
			lease_duration_hours=EXCLUDED.lease_duration_hours,
			lease_duration_seconds=EXCLUDED.lease_duration_seconds,
			state=EXCLUDED.state,
			last_synced_at=now(),
			sync_status='synced',
			last_error='',
			updated_at=now()
		RETURNING id::text
	`, scope.Name, scope.Description, scope.Subnet, scope.DefaultGateway, scope.StartRange, scope.EndRange, scope.LeaseDurationHours, scope.LeaseDurationSeconds, scope.State, serverID, scope.ExternalID).Scan(&scopeID); err != nil {
		return err
	}

	keepExclusionIDs := []string{}
	for _, exclusion := range exclusions {
		exclusion.ScopeID = scopeID
		exclusion.ExternalID = strutil.FirstNonEmpty(exclusion.ExternalID, exclusionExternalID(exclusion))
		if err := tx.QueryRow(ctx, `
			INSERT INTO dhcp_exclusions(scope_id, start_ip, end_ip, external_id, last_synced_at)
			VALUES($1, $2, $3, $4, now())
			ON CONFLICT (scope_id, start_ip, end_ip) DO UPDATE SET
				external_id=EXCLUDED.external_id,
				last_synced_at=now(),
				updated_at=now()
			RETURNING id::text
		`, scopeID, exclusion.StartIP, exclusion.EndIP, exclusion.ExternalID).Scan(&exclusion.ID); err != nil {
			return err
		}
		keepExclusionIDs = append(keepExclusionIDs, exclusion.ID)
	}

	keepLeaseIDs := []string{}
	for _, lease := range leases {
		externalID := strutil.FirstNonEmpty(lease.ExternalID, lease.ID, lease.IP)
		expiresAt, err := parseTimeOrNow(lease.ExpiresAt)
		if err != nil {
			expiresAt = time.Now().UTC()
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO dhcp_leases(scope_id, ip, mac, hostname, state, expires_at, external_id, last_synced_at)
			VALUES($1, $2, $3, $4, $5, $6, $7, now())
			ON CONFLICT (scope_id, ip) DO UPDATE SET
				mac=EXCLUDED.mac,
				hostname=EXCLUDED.hostname,
				state=EXCLUDED.state,
				expires_at=EXCLUDED.expires_at,
				external_id=EXCLUDED.external_id,
				last_synced_at=now()
			RETURNING id::text
		`, scopeID, lease.IP, lease.MAC, lease.Hostname, strutil.FirstNonEmpty(lease.State, "Active"), expiresAt, externalID).Scan(&lease.ID); err != nil {
			return err
		}
		keepLeaseIDs = append(keepLeaseIDs, lease.ID)
	}
	keepReservationIDs := []string{}
	for _, reservation := range reservations {
		externalID := strutil.FirstNonEmpty(reservation.ExternalID, reservation.ID, reservation.IP)
		if err := tx.QueryRow(ctx, `
			INSERT INTO dhcp_reservations(scope_id, ip, mac, name, description, external_id, last_synced_at)
			VALUES($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (scope_id, ip) DO UPDATE SET
				mac=EXCLUDED.mac,
				name=EXCLUDED.name,
				description=EXCLUDED.description,
				external_id=EXCLUDED.external_id,
				last_synced_at=now()
			RETURNING id::text
		`, scopeID, reservation.IP, reservation.MAC, reservation.Name, reservation.Description, externalID).Scan(&reservation.ID); err != nil {
			return err
		}
		keepReservationIDs = append(keepReservationIDs, reservation.ID)
	}
	if err := deleteStaleDHCPScopeRows(ctx, tx, scopeID, keepExclusionIDs, keepLeaseIDs, keepReservationIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteDHCPScopeByExternalID(ctx context.Context, serverID, externalID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dhcp_scopes WHERE server_id=$1 AND external_id=$2`, serverID, externalID)
	return err
}

func deleteStaleDHCPRows(ctx context.Context, exec dhcpExecutor, serverID string, keepScopeIDs, keepLeaseIDs, keepReservationIDs []string) error {
	if len(keepScopeIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_scopes WHERE server_id=$1`, serverID); err != nil {
			return err
		}
		return nil
	}
	if _, err := exec.Exec(ctx, `DELETE FROM dhcp_exclusions WHERE scope_id::text = ANY($1)`, keepScopeIDs); err != nil {
		return err
	}
	if len(keepLeaseIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_leases WHERE scope_id::text = ANY($1)`, keepScopeIDs); err != nil {
			return err
		}
	} else if _, err := exec.Exec(ctx, `DELETE FROM dhcp_leases WHERE scope_id::text = ANY($1) AND NOT (id::text = ANY($2))`, keepScopeIDs, keepLeaseIDs); err != nil {
		return err
	}
	if len(keepReservationIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_reservations WHERE scope_id::text = ANY($1)`, keepScopeIDs); err != nil {
			return err
		}
	} else if _, err := exec.Exec(ctx, `DELETE FROM dhcp_reservations WHERE scope_id::text = ANY($1) AND NOT (id::text = ANY($2))`, keepScopeIDs, keepReservationIDs); err != nil {
		return err
	}
	if _, err := exec.Exec(ctx, `DELETE FROM dhcp_scopes WHERE server_id=$1 AND NOT (id::text = ANY($2))`, serverID, keepScopeIDs); err != nil {
		return err
	}
	return nil
}

func deleteStaleDHCPScopeRows(ctx context.Context, exec dhcpExecutor, scopeID string, keepExclusionIDs, keepLeaseIDs, keepReservationIDs []string) error {
	if len(keepExclusionIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_exclusions WHERE scope_id=$1`, scopeID); err != nil {
			return err
		}
	} else if _, err := exec.Exec(ctx, `DELETE FROM dhcp_exclusions WHERE scope_id=$1 AND NOT (id::text = ANY($2))`, scopeID, keepExclusionIDs); err != nil {
		return err
	}
	if len(keepLeaseIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_leases WHERE scope_id=$1`, scopeID); err != nil {
			return err
		}
	} else if _, err := exec.Exec(ctx, `DELETE FROM dhcp_leases WHERE scope_id=$1 AND NOT (id::text = ANY($2))`, scopeID, keepLeaseIDs); err != nil {
		return err
	}
	if len(keepReservationIDs) == 0 {
		if _, err := exec.Exec(ctx, `DELETE FROM dhcp_reservations WHERE scope_id=$1`, scopeID); err != nil {
			return err
		}
	} else if _, err := exec.Exec(ctx, `DELETE FROM dhcp_reservations WHERE scope_id=$1 AND NOT (id::text = ANY($2))`, scopeID, keepReservationIDs); err != nil {
		return err
	}
	return nil
}

type dhcpExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func parseTimeOrNow(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func exclusionExternalID(item domain.DHCPExclusion) string {
	if item.ExternalID != "" {
		return item.ExternalID
	}
	return item.StartIP + "-" + item.EndIP
}
