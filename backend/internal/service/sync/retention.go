package sync

import (
	"context"
	"time"
)

func (s *Service) StartLogRetention(ctx context.Context) {
	retentionDays := s.cfg.LogRetentionDays
	if retentionDays <= 0 {
		s.logger.Info("Log retention cleanup disabled")
		return
	}
	go func() {
		s.cleanupOldLogs(ctx, retentionDays)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanupOldLogs(ctx, retentionDays)
			}
		}
	}()
}

func (s *Service) cleanupOldLogs(ctx context.Context, retentionDays int) {
	before := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	if err := s.store.DeleteLogRecordsBefore(ctx, before); err != nil {
		s.logger.Warn("Delete old log records failed", "error", err)
	}
}
