package provider

import (
	"testing"
)

func TestBuildFQDN(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		zone  string
		want  string
	}{
		{
			name:  "without root node",
			owner: "test",
			zone:  "example.com",
			want:  "test.example.com.",
		},
		{
			name:  "with root node",
			owner: "test",
			zone:  "example.com.",
			want:  "test.example.com.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFQDN(tt.owner, tt.zone); got != tt.want {
				t.Errorf("buildFQDN() = %v, want %v", got, tt.want)
			}
		})
	}
}

