package dev

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyDockerStartFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		output []string
		want   failureCategory
	}{
		"port in use": {
			output: []string{"Error response from daemon: driver failed programming external connectivity on endpoint shop-web-1: Bind for 0.0.0.0:8000 failed: port is already allocated"},
			want:   dockerStartFailurePortInUse,
		},
		"disk full": {
			output: []string{"failed to register layer: write /usr/lib/x86_64-linux-gnu/libLLVM.so.15: no space left on device"},
			want:   dockerStartFailureDiskFull,
		},
		"image pull": {
			output: []string{"Error response from daemon: toomanyrequests: You have reached your pull rate limit."},
			want:   dockerStartFailureImagePull,
		},
		"mount wins over permission": {
			output: []string{"Error response from daemon: error while creating mount source path '/home/me/shop': mkdir /home/me/shop: permission denied"},
			want:   dockerStartFailureMount,
		},
		"permission": {
			output: []string{"open /home/me/shop/compose.yaml: permission denied"},
			want:   dockerStartFailurePermission,
		},
		"last matching line wins": {
			output: []string{"\x1b[33mWARN\x1b[0m permission denied while reading cache", "Bind for 0.0.0.0:3306 failed: port is already allocated"},
			want:   dockerStartFailurePortInUse,
		},
		"unknown": {
			output: []string{"service \"web\" depends on undefined service \"db\""},
			want:   dockerStartFailureUnknown,
		},
		"no output": {
			want: dockerStartFailureUnknown,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifyDockerStartFailure(tt.output))
		})
	}
}
