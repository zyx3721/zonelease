package repository

import "testing"

func TestServerHealthUpdateBecameOffline(t *testing.T) {
	cases := []struct {
		name string
		item ServerHealthUpdate
		want bool
	}{
		{
			name: "online to offline",
			item: ServerHealthUpdate{PreviousStatus: "Online", Status: "Offline"},
			want: true,
		},
		{
			name: "offline stays offline",
			item: ServerHealthUpdate{PreviousStatus: "Offline", Status: "Offline"},
			want: false,
		},
		{
			name: "failed health before threshold keeps online",
			item: ServerHealthUpdate{PreviousStatus: "Online", Status: "Online"},
			want: false,
		},
		{
			name: "offline restored",
			item: ServerHealthUpdate{PreviousStatus: "Offline", Status: "Online"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.BecameOffline(); got != tc.want {
				t.Fatalf("BecameOffline()=%v, want %v", got, tc.want)
			}
		})
	}
}
