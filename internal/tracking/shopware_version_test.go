package tracking

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShopwareVersionLookupDoesNotWait(t *testing.T) {
	enableTracking(t)
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		lookup := startShopwareVersion("project.console", func() string {
			<-release
			return "6.7.11.0"
		})
		synctest.Wait()
		assert.Empty(t, lookup.Ready())
		close(release)
		// The worker must finish even though its result has not been consumed.
		synctest.Wait()
		assert.Equal(t, "6.7.11.0", lookup.Ready())
	})
}

func TestShopwareVersionLookupSkipsIneligibleCommands(t *testing.T) {
	enableTracking(t)
	for _, name := range []string{"", "project", "project.create", "extension.build"} {
		lookup := startShopwareVersion(name, func() string {
			t.Error("unexpected lookup")
			return ""
		})
		assert.Nil(t, lookup)
		assert.Empty(t, lookup.Ready())
	}
}

func TestShopwareVersionLookupRespectsDoNotTrack(t *testing.T) {
	for _, value := range []string{"", "1", "0"} {
		t.Setenv("DO_NOT_TRACK", value)
		lookup := startShopwareVersion("project.console", func() string {
			t.Error("unexpected lookup")
			return ""
		})
		assert.Nil(t, lookup)
		assert.Empty(t, lookup.Ready())
	}
}

func TestShopwareVersionLookupRespectsUserConfig(t *testing.T) {
	enableTracking(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("telemetry: false\n"), 0o600))
	t.Setenv("SHOPWARE_CLI_CONFIG", configPath)

	lookup := startShopwareVersion("project.console", func() string {
		t.Error("unexpected lookup")
		return ""
	})
	assert.Nil(t, lookup)
}

func TestProjectShopwareVersion(t *testing.T) {
	enableTracking(t)
	root := t.TempDir()
	t.Setenv("PROJECT_ROOT", root)
	for version, want := range map[string]string{
		"v6.7.11.0":       "6.7.11.0",
		"v6.7.0.0-rc1":    "6.7.0.0-rc1",
		"dev-trunk":       "dev-trunk",
		"v6.7.0.0-RC1":    "",
		"6.6.x-dev":       "",
		"6.7.12.x-dev":    "",
		"dev-tmp-0c8ec10": "",
		"dev-feature-xyz": "",
		"6.7.11.0-dev":    "",
	} {
		t.Run(version, func(t *testing.T) {
			lock := fmt.Sprintf(`{"packages":[{"name":"shopware/core","version":%q}],"packages-dev":[]}`, version)
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(lock), 0o644))
			assert.Equal(t, want, projectShopwareVersion())
		})
	}
	t.Setenv("PROJECT_ROOT", t.TempDir())
	assert.Empty(t, projectShopwareVersion())
	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(t.TempDir())
	assert.Empty(t, projectShopwareVersion())
}

func TestReportableShopwareVersion(t *testing.T) {
	assert.Equal(t, "6.6.0.0", ReportableShopwareVersion("6.6.0.0"))
	assert.Equal(t, "dev-trunk", ReportableShopwareVersion("dev-trunk"))
	assert.Empty(t, ReportableShopwareVersion("dev-feature-xyz"))
}

func TestShopwareVersionLookupUnavailable(t *testing.T) {
	enableTracking(t)
	synctest.Test(t, func(t *testing.T) {
		lookup := startShopwareVersion("project.console", func() string { return "" })
		synctest.Wait()
		assert.Empty(t, lookup.Ready())
	})
}
