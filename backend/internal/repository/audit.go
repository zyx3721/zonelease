package repository

import (
	"context"
	"time"

	"zonelease/backend/internal/domain"
)

func (s *Store) ListAudit(ctx context.Context) ([]domain.AuditEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, ts, username, action, target, module, result, COALESCE(ip_address, ''), detail
		FROM audit_entries ORDER BY ts DESC LIMIT 500
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditEntry{}
	for rows.Next() {
		var item domain.AuditEntry
		var ts time.Time
		if err := rows.Scan(&item.ID, &ts, &item.User, &item.Action, &item.Target, &item.Module, &item.Result, &item.IPAddress, &item.Detail); err != nil {
			return nil, err
		}
		item.TS = ts.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) WriteAudit(ctx context.Context, userID, username, action, target, module, result, detail, ipAddress string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_entries(user_id, username, action, target, module, result, detail, ip_address)
		VALUES(NULLIF($1, '')::uuid, $2, $3, $4, $5, $6, $7, $8)
	`, userID, username, action, target, module, result, detail, ipAddress)
	return err
}
