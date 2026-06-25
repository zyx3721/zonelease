package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) CreateNotification(ctx context.Context, level, title, message, sourceType, sourceID string, metadata any) (domain.Notification, error) {
	bytes, err := json.Marshal(metadata)
	if err != nil {
		return domain.Notification{}, err
	}
	var item domain.Notification
	err = s.pool.QueryRow(ctx, `
		INSERT INTO notifications(level, title, message, source_type, source_id, metadata)
		VALUES($1, $2, $3, $4, $5, $6)
		RETURNING id::text, level, title, message, source_type, source_id, metadata, read_at, dismissed_at, created_at, updated_at
	`, level, title, message, sourceType, sourceID, bytes).Scan(&item.ID, &item.Level, &item.Title, &item.Message, &item.SourceType, &item.SourceID, &item.Metadata, &item.ReadAt, &item.DismissedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) CreateNotificationIfUnreadMissing(ctx context.Context, level, title, message, sourceType, sourceID string, metadata any) (domain.Notification, bool, error) {
	var existingID string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text
		FROM notifications
		WHERE source_type=$1 AND source_id=$2 AND read_at IS NULL AND dismissed_at IS NULL
		LIMIT 1
	`, sourceType, sourceID).Scan(&existingID)
	if err == nil {
		return domain.Notification{ID: existingID}, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Notification{}, false, err
	}
	item, err := s.CreateNotification(ctx, level, title, message, sourceType, sourceID, metadata)
	return item, true, err
}

func (s *Store) ListNotifications(ctx context.Context, limit int) ([]domain.Notification, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications WHERE dismissed_at IS NULL AND source_type <> 'refresh_task'
	`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, level, title, message, source_type, source_id, metadata, read_at, dismissed_at, created_at, updated_at
		FROM notifications
		WHERE dismissed_at IS NULL AND source_type <> 'refresh_task'
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Notification, 0)
	for rows.Next() {
		var item domain.Notification
		if err := rows.Scan(&item.ID, &item.Level, &item.Title, &item.Message, &item.SourceType, &item.SourceID, &item.Metadata, &item.ReadAt, &item.DismissedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) CountUnreadNotifications(ctx context.Context) (int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications WHERE read_at IS NULL AND dismissed_at IS NULL
			AND source_type <> 'refresh_task'
	`).Scan(&total)
	return total, err
}

func (s *Store) MarkNotificationRead(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET read_at=COALESCE(read_at, now()), updated_at=now()
		WHERE id=$1 AND dismissed_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET read_at=COALESCE(read_at, now()), updated_at=now()
		WHERE dismissed_at IS NULL
	`)
	return err
}

func (s *Store) DismissNotifications(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET dismissed_at=COALESCE(dismissed_at, now()), read_at=COALESCE(read_at, now()), updated_at=now()
		WHERE dismissed_at IS NULL
	`)
	return err
}

func (s *Store) DismissNotificationsBySource(ctx context.Context, sourceType, sourceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET dismissed_at=COALESCE(dismissed_at, now()), read_at=COALESCE(read_at, now()), updated_at=now()
		WHERE source_type=$1 AND source_id=$2 AND dismissed_at IS NULL
	`, sourceType, sourceID)
	return err
}
