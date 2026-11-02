package service

import "testing"

func TestPickColorForUser(t *testing.T) {
	tests := []struct {
		userID string
		want   string
	}{
		{"", "#e6194b"},
		{"00000000-0000-4000-8000-000000000001", "#fabebe"},
		{"550e8400-e29b-41d4-a716-446655440000", "#f032e6"},
		{"6ba7b810-9dad-11d1-80b4-00c04fd430c8", "#fffac8"},
		{"ffffffff-ffff-ffff-ffff-ffffffffffff", "#f58231"},
	}
	for _, tc := range tests {
		if got := PickColorForUser(tc.userID); got != tc.want {
			t.Fatalf("PickColorForUser(%q) = %q, want %q", tc.userID, got, tc.want)
		}
	}
}
