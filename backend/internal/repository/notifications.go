package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) ListNotificationChannels(ctx context.Context) ([]domain.NotificationChannel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(name, ''), enabled, password_reset_enabled, config, created_at, updated_at
		FROM notification_channels
		WHERE id='email'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.NotificationChannel, 0)
	for rows.Next() {
		var item domain.NotificationChannel
		if err := rows.Scan(&item.ID, &item.Name, &item.Enabled, &item.PasswordResetEnabled, &item.Config, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetNotificationChannel(ctx context.Context, id string) (domain.NotificationChannel, error) {
	var item domain.NotificationChannel
	err := s.pool.QueryRow(ctx, `
		SELECT id, COALESCE(name, ''), enabled, password_reset_enabled, config, created_at, updated_at
		FROM notification_channels WHERE id=$1 AND id='email'
	`, id).Scan(&item.ID, &item.Name, &item.Enabled, &item.PasswordResetEnabled, &item.Config, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotificationChannel{}, ErrNotFound
	}
	return item, err
}

func (s *Store) GetPasswordResetNotificationChannel(ctx context.Context) (domain.NotificationChannel, error) {
	var item domain.NotificationChannel
	err := s.pool.QueryRow(ctx, `
		SELECT id, COALESCE(name, ''), enabled, password_reset_enabled, config, created_at, updated_at
		FROM notification_channels
		WHERE id='email' AND password_reset_enabled=true
		LIMIT 1
	`).Scan(&item.ID, &item.Name, &item.Enabled, &item.PasswordResetEnabled, &item.Config, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotificationChannel{}, ErrNotFound
	}
	return item, err
}

func (s *Store) UpsertNotificationChannel(ctx context.Context, id string, enabled bool, passwordResetEnabled bool, config any) (domain.NotificationChannel, error) {
	configBytes, err := json.Marshal(config)
	if err != nil {
		return domain.NotificationChannel{}, err
	}
	var item domain.NotificationChannel
	err = s.pool.QueryRow(ctx, `
		INSERT INTO notification_channels(id, name, enabled, password_reset_enabled, config)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			name=COALESCE(NULLIF(notification_channels.name, ''), EXCLUDED.name),
			enabled=EXCLUDED.enabled,
			password_reset_enabled=EXCLUDED.password_reset_enabled,
			config=EXCLUDED.config,
			updated_at=now()
		RETURNING id, COALESCE(name, ''), enabled, password_reset_enabled, config, created_at, updated_at
	`, id, defaultNotificationChannelName(id), enabled, passwordResetEnabled, configBytes).Scan(&item.ID, &item.Name, &item.Enabled, &item.PasswordResetEnabled, &item.Config, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func defaultNotificationChannelName(id string) string {
	if id == "email" {
		return "邮件媒介"
	}
	return id
}
