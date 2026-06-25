package repository

import (
	"context"
	"time"
)

const (
	deleteRefreshTasksBeforeSQL  = `DELETE FROM refresh_tasks WHERE created_at < $1`
	deleteAuditEntriesBeforeSQL  = `DELETE FROM audit_entries WHERE ts < $1`
	deleteNotificationsBeforeSQL = `DELETE FROM notifications WHERE created_at < $1`
)

func (s *Store) DeleteLogRecordsBefore(ctx context.Context, before time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, deleteRefreshTasksBeforeSQL, before); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, deleteAuditEntriesBeforeSQL, before); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, deleteNotificationsBeforeSQL, before); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
