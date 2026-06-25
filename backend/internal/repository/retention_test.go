package repository

import (
	"strings"
	"testing"
)

func TestLogRetentionUsesStableTimeColumns(t *testing.T) {
	queries := map[string]string{
		"refresh_tasks": deleteRefreshTasksBeforeSQL,
		"audit_entries": deleteAuditEntriesBeforeSQL,
		"notifications": deleteNotificationsBeforeSQL,
	}
	for table, query := range queries {
		if strings.Contains(query, "status") || strings.Contains(query, "finished_at") || strings.Contains(query, "read_at") || strings.Contains(query, "dismissed_at") {
			t.Fatalf("%s retention SQL must not depend on status or lifecycle timestamps: %s", table, query)
		}
	}
	if !strings.Contains(deleteRefreshTasksBeforeSQL, "created_at < $1") {
		t.Fatalf("refresh_tasks retention SQL must use created_at: %s", deleteRefreshTasksBeforeSQL)
	}
	if !strings.Contains(deleteAuditEntriesBeforeSQL, "ts < $1") {
		t.Fatalf("audit_entries retention SQL must use ts: %s", deleteAuditEntriesBeforeSQL)
	}
	if !strings.Contains(deleteNotificationsBeforeSQL, "created_at < $1") {
		t.Fatalf("notifications retention SQL must use created_at: %s", deleteNotificationsBeforeSQL)
	}
}
