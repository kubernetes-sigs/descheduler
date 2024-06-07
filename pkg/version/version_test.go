package version

import (
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantMajor string
		wantMinor string
	}{
		{
			name:      "parses automated container release tag",
			version:   "v20240519-v0.30.0",
			wantMajor: "0",
			wantMinor: "30.0",
		},
		{
			name:      "parses automated container build",
			version:   "v20240520-v0.30.0-5-g79990946",
			wantMajor: "0",
			wantMinor: "30.0",
		},
		{
			name:      "parses helm release tag",
			version:   "v20240606-descheduler-helm-chart-0.30.0-18-g8714397b",
			wantMajor: "0",
			wantMinor: "30.0",
		},
	}
	for _, tt := range tests {
		version = tt.version
		t.Run(tt.name, func(t *testing.T) {
			got := Get()
			if got.Major != tt.wantMajor {
				t.Errorf("Get() major = got %v, want %v", got.Major, tt.wantMajor)
			}
			if got.Minor != tt.wantMinor {
				t.Errorf("Get() minor = got %v, want %v", got.Minor, tt.wantMinor)
			}
		})
	}
}
