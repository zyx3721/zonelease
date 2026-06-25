package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) GetSystemBaseConfig(ctx context.Context) (domain.SystemBaseConfig, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT value FROM system_settings WHERE key='base'
	`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SystemBaseConfig{}, nil
	}
	if err != nil {
		return domain.SystemBaseConfig{}, err
	}
	var item domain.SystemBaseConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &item); err != nil {
			return domain.SystemBaseConfig{}, err
		}
	}
	return item, nil
}

func (s *Store) UpdateSystemBaseConfig(ctx context.Context, item domain.SystemBaseConfig) (domain.SystemBaseConfig, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return domain.SystemBaseConfig{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO system_settings(key, value, updated_at)
		VALUES('base', $1, now())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
	`, raw)
	if err != nil {
		return domain.SystemBaseConfig{}, err
	}
	return item, nil
}
