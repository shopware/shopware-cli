package deploymenthelper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/require"
)

func TestCheckRequired(t *testing.T) {
	for _, tt := range []struct {
		name     string
		contents string
		wantErr  bool
	}{
		{name: "required", contents: `{"require":{"shopware/deployment-helper":"^0.5"}}`},
		{name: "missing dependency", contents: `{"require":{"shopware/core":"^6.7"}}`, wantErr: true},
		{name: "development only", contents: `{"require-dev":{"shopware/deployment-helper":"*"}}`, wantErr: true},
		{name: "invalid composer json", contents: `{`, wantErr: true},
		{name: "missing composer json", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.contents != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(tt.contents), 0o600))
			}
			err := CheckRequired(root)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEnsureRequired(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		production, development string
		want                    string
		changed                 bool
	}{
		{"missing", "", "", "*", true},
		{"required", "^1.0", "", "^1.0", false},
		{"development only", "", "^0.5", "^0.5", true},
		{"both", "^1.0", "^0.5", "^1.0", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &composer.Json{}
			if tt.production != "" {
				c.AddPackage(Package, tt.production)
			}
			if tt.development != "" {
				c.AddPackageDev(Package, tt.development)
			}
			require.Equal(t, tt.changed, EnsureRequired(c))
			require.Equal(t, tt.want, c.Require[Package])
			require.NotContains(t, c.RequireDev, Package)
			require.False(t, EnsureRequired(c))
		})
	}
}
