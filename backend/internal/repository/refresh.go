package repository

import (
	"context"
	"encoding/json"

	"zonelease/backend/internal/domain"
)

func (s *Store) CreateRefreshTask(ctx context.Context, taskType string, payload any, createdBy string) (domain.RefreshTask, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return domain.RefreshTask{}, err
	}
	var task domain.RefreshTask
	err = s.pool.QueryRow(ctx, `
		INSERT INTO refresh_tasks(type, status, payload, created_by)
		VALUES($1, 'queued', $2, NULLIF($3, '')::uuid)
		RETURNING id::text, type, status, payload, COALESCE(created_by::text, ''), created_at, updated_at, finished_at
	`, taskType, bytes, createdBy).Scan(&task.ID, &task.Type, &task.Status, &task.Payload, &task.CreatedBy, &task.CreatedAt, &task.UpdatedAt, &task.FinishedAt)
	return task, err
}

func (s *Store) ListRefreshTasks(ctx context.Context, limit int) ([]domain.RefreshTask, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, type, status, payload, COALESCE(created_by::text, ''), created_at, updated_at, finished_at
		FROM refresh_tasks ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.RefreshTask{}
	for rows.Next() {
		var item domain.RefreshTask
		if err := rows.Scan(&item.ID, &item.Type, &item.Status, &item.Payload, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListAllRefreshTasks(ctx context.Context) ([]domain.RefreshTask, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, type, status, payload, COALESCE(created_by::text, ''), created_at, updated_at, finished_at
		FROM refresh_tasks ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.RefreshTask{}
	for rows.Next() {
		var item domain.RefreshTask
		if err := rows.Scan(&item.ID, &item.Type, &item.Status, &item.Payload, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpdateRefreshTask(ctx context.Context, id, status string, payload any) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd, err := s.pool.Exec(ctx, `
		UPDATE refresh_tasks
		SET status=$2, payload=$3, updated_at=now(), finished_at=CASE WHEN $2 IN ('completed', 'failed') THEN now() ELSE finished_at END
		WHERE id=$1
	`, id, status, bytes)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
