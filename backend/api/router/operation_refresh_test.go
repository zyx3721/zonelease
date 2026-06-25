package router

import (
	"testing"
	"time"
)

func TestOperationRefreshTargetKey(t *testing.T) {
	tests := []struct {
		name   string
		target operationRefreshTarget
		want   string
	}{
		{
			name: "dns zone",
			target: operationRefreshTarget{
				Kind:     operationRefreshDNSZone,
				ServerID: "server-1",
				ZoneName: "example.com",
			},
			want: "dns.zone:server-1:example.com",
		},
		{
			name: "dhcp scope",
			target: operationRefreshTarget{
				Kind:            operationRefreshDHCPScope,
				ServerID:        "server-1",
				ScopeExternalID: "10.0.0.0",
			},
			want: "dhcp.scope:server-1:10.0.0.0",
		},
		{
			name:   "missing target",
			target: operationRefreshTarget{Kind: operationRefreshDNSZone, ServerID: "server-1"},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.key(); got != tt.want {
				t.Fatalf("expected key %q, got %q", tt.want, got)
			}
		})
	}
}

func TestOperationRefreshStateWaitsForActiveOperations(t *testing.T) {
	state := operationRefreshState{active: 1}
	if state.active == 0 {
		t.Fatal("expected operation to be active")
	}
	state.dirty = true
	if !state.dirty {
		t.Fatal("expected dirty target")
	}
	state.active--
	if state.active != 0 {
		t.Fatalf("expected operation to finish, got active=%d", state.active)
	}
}

func TestOperationRefreshTimerCanBeReset(t *testing.T) {
	timer := time.AfterFunc(time.Hour, func() {})
	if !timer.Stop() {
		t.Fatal("expected newly created timer to stop before firing")
	}
	timer.Reset(time.Hour)
	if !timer.Stop() {
		t.Fatal("expected reset timer to stop before firing")
	}
}
