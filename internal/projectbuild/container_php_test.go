package projectbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestContainerPHPVersionDetection(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	caller := t.TempDir()
	t.Chdir(caller)
	testhelper.WriteFile(t, filepath.Join(caller, "composer.lock"), "must not read the caller's lockfile")
	for _, tc := range []struct {
		name, flag, pin, docker, lock, want, wantErr string
	}{
		{name: "default", want: "8.3"},
		{name: "flag wins", flag: "8.5", pin: "8.4", docker: "8.2", lock: "invalid", want: "8.5"},
		{name: "project pin wins", pin: "8.4", docker: "8.2", lock: "invalid", want: "8.4"},
		{name: "Docker pin wins", docker: "8.2", lock: "invalid", want: "8.2"},
		{name: "locked core", lock: `{"packages":[{"name":"shopware/core","require":{"php":"^8.4"}}]}`, want: "8.5"},
		{name: "locked platform", lock: `{"packages":[{"name":"shopware/platform","require":{"php":"~8.3.0"}}]}`, want: "8.3"},
		{name: "core takes precedence", lock: `{"packages":[{"name":"shopware/platform","require":{"php":"~8.2.0"}},{"name":"shopware/core","require":{"php":"~8.4.0"}}]}`, want: "8.4"},
		{name: "no locked Shopware", lock: `{"packages":[]}`, want: "8.3"},
		{name: "no locked PHP requirement", lock: `{"packages":[{"name":"shopware/core"}]}`, want: "8.3"},
		{name: "unsupported flag", flag: "7.4", pin: "8.4", wantErr: "unsupported PHP version"},
		{name: "unsupported project pin", pin: "7.4", docker: "8.4", wantErr: "unsupported PHP version"},
		{name: "unsupported Docker pin", docker: "7.4", wantErr: "unsupported PHP version"},
		{name: "malformed lock", lock: "invalid", wantErr: "detect container PHP version from composer.lock"},
		{name: "incompatible locked PHP", lock: `{"packages":[{"name":"shopware/core","require":{"php":"<8.2"}}]}`, wantErr: "no supported PHP image version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.lock != "" {
				testhelper.WriteFile(t, filepath.Join(root, "composer.lock"), tc.lock)
			}
			cfg := &shop.Config{PHPVersion: tc.pin}
			if tc.docker != "" {
				cfg.Docker = &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: tc.docker}}
			}
			paths, err := GenerateContainerFiles(t.Context(), root, cfg, ContainerOptions{PHPVersion: tc.flag})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, paths)
				assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
				assert.NoFileExists(t, filepath.Join(root, ".dockerignore"))
				return
			}
			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "ARG PHP_VERSION="+tc.want+"\n")
		})
	}
}

func TestContainerPHPVersionUnreadableLock(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "composer.lock"), 0o755))
	_, err := GenerateContainerFiles(t.Context(), root, &shop.Config{}, ContainerOptions{})
	require.ErrorContains(t, err, "detect container PHP version from composer.lock")
	assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
}
