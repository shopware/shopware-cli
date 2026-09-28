package cliversion

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgent(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	Version = "1.2.3"

	assert.Equal(t, "shopware-cli/1.2.3", UserAgent())
}

func TestResolve(t *testing.T) {
	withModuleVersion := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}

	tests := []struct {
		name    string
		current string
		info    *debug.BuildInfo
		want    string
	}{
		{name: "ldflags win over module version", current: "1.2.3", info: withModuleVersion("v4.5.6"), want: "1.2.3"},
		{name: "tagged module version", current: devVersion, info: withModuleVersion("v4.5.6"), want: "4.5.6"},
		{name: "dirty tagged module version", current: devVersion, info: withModuleVersion("v4.5.6+dirty"), want: "4.5.6"},
		{name: "pseudo-version", current: devVersion, info: withModuleVersion("v0.18.6-0.20260928094429-830aa6f43b2e"), want: devVersion},
		{name: "dirty pseudo-version", current: devVersion, info: withModuleVersion("v0.18.6-0.20260928094429-830aa6f43b2e+dirty"), want: devVersion},
		{name: "devel build", current: devVersion, info: withModuleVersion("(devel)"), want: devVersion},
		{name: "empty module version", current: devVersion, info: withModuleVersion(""), want: devVersion},
		{name: "no build info", current: devVersion, info: nil, want: devVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolve(tt.current, tt.info))
		})
	}
}
