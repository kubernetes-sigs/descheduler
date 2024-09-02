package version

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    Info
	}{
		{
			name:    "parses automated container release tag",
			version: "v20240519-v0.30.0",
			want: Info{
				Major:      "0",
				Minor:      "30.0",
				GitVersion: "v20240519-v0.30.0",
			},
		},
		{
			name:    "parses automated container build",
			version: "v20240520-v0.30.0-5-g79990946",
			want: Info{
				Major:      "0",
				Minor:      "30.0",
				GitVersion: "v20240520-v0.30.0-5-g79990946",
			},
		},
		{
			name:    "parses helm release tag",
			version: "v20240606-descheduler-helm-chart-0.30.0-18-g8714397b",
			want: Info{
				Major:      "0",
				Minor:      "30.0",
				GitVersion: "v20240606-descheduler-helm-chart-0.30.0-18-g8714397b",
			},
		},
	}
	ignoreRuntimeFields := cmpopts.IgnoreFields(Info{}, "GoVersion", "Compiler", "Platform")
	for _, tt := range tests {
		version = tt.version
		t.Run(tt.name, func(t *testing.T) {
			got := Get()
			if diff := cmp.Diff(got, tt.want, ignoreRuntimeFields); diff != "" {
				t.Errorf("Get (-want, +got):\n%s", diff)
			}
		})
	}
}
