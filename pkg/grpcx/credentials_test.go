package grpcx

import "testing"

func TestTransportCredentials(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{target: "localhost:7001", want: "insecure"},
		{target: "room:7001", want: "insecure"},
		{target: "room-123.development.catalystappsail.in:443", want: "tls"},
		{target: "dns:///room.example.com:443", want: "tls"},
		{target: "room.example.com:4430", want: "insecure"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := transportCredentials(tt.target).Info().SecurityProtocol
			if got != tt.want {
				t.Fatalf("security protocol %q, want %q", got, tt.want)
			}
		})
	}
}
