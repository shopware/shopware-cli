package securityplugin

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func pluginList(output string) func(context.Context) ([]byte, error) {
	return func(context.Context) ([]byte, error) { return []byte(output), nil }
}

func failingPluginList(context.Context) ([]byte, error) {
	return nil, errors.New("environment is down")
}

func storeVersion(latest string) func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
	return func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
		return []account_api.StorePlugin{{Name: TechnicalName, Version: latest}}, nil
	}
}

func projectWithPlugin(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testhelper.ComposerLock(
		testhelper.LockPackage{Name: "shopware/core", Version: "v6.7.0.0"},
		testhelper.LockPackage{Name: lockedPackage, Version: version},
	))
	return dir
}

const lockedPackage = "store.shopware.com/swagplatformsecurity"

const (
	activeList      = `[{"name":"SwagPlatformSecurity","version":"2.0.20","active":true,"installedAt":"2026-01-01T00:00:00.000+00:00"}]`
	deactivatedList = `[{"name":"SwagPlatformSecurity","version":"2.0.27","active":false,"installedAt":"2026-01-01T00:00:00.000+00:00"}]`
	uninstalledList = `[{"name":"SwagPlatformSecurity","version":"2.0.27","active":false,"installedAt":null}]`
	otherList       = `[{"name":"SwagOther","version":"1.0.0","active":true,"installedAt":"2026-01-01T00:00:00.000+00:00"}]`
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name       string
		lockedAt   string // empty: plugin not in the project files
		pluginList func(context.Context) ([]byte, error)
		latest     string
		want       Status
	}{
		{"missing everywhere", "", pluginList(otherList), "2.0.27", Status{State: StateMissing}},
		{"absent from the files while the shop cannot be asked", "", failingPluginList, "2.0.27", Status{State: StateUnknown}},
		{"required but unknown to the shop", "2.0.27", pluginList(otherList), "2.0.27", Status{State: StateInactive, Version: "2.0.27", Latest: "2.0.27", Unregistered: true}},
		{"not installed in the shop", "2.0.27", pluginList(uninstalledList), "2.0.27", Status{State: StateInactive, Version: "2.0.27", Latest: "2.0.27"}},
		{"deactivated", "2.0.27", pluginList(deactivatedList), "2.0.27", Status{State: StateInactive, Version: "2.0.27", Latest: "2.0.27"}},
		{"outdated", "v2.0.20", pluginList(activeList), "2.0.27", Status{State: StateOutdated, Version: "2.0.20", Latest: "2.0.27"}},
		{"downloaded but not applied by the shop", "2.0.27", pluginList(activeList), "2.0.27", Status{State: StateOutdated, Version: "2.0.20", Latest: "2.0.27", Downloaded: true}},
		{"up to date", "2.0.20", pluginList(activeList), "2.0.20", Status{State: StateUpToDate, Version: "2.0.20", Latest: "2.0.20"}},
		{"listed in the shop only", "", pluginList(activeList), "2.0.20", Status{State: StateUpToDate, Version: "2.0.20", Latest: "2.0.20"}},
		{"notices before the JSON", "2.0.20", pluginList("Deprecated: something\n" + activeList), "2.0.20", Status{State: StateUpToDate, Version: "2.0.20", Latest: "2.0.20"}},
		{"outdated is reported without the shop", "2.0.20", failingPluginList, "2.0.27", Status{State: StateOutdated, Version: "2.0.20", Latest: "2.0.27"}},
		{"activation unknown is not positive", "2.0.27", failingPluginList, "2.0.27", Status{State: StateUnknown, Version: "2.0.27", Latest: "2.0.27"}},
		{"unparsable plugin list is not positive", "2.0.27", pluginList("boom"), "2.0.27", Status{State: StateUnknown, Version: "2.0.27", Latest: "2.0.27"}},
		{"latest unknown is not positive", "2.0.20", pluginList(activeList), "", Status{State: StateUnknown, Version: "2.0.20"}},
		{"uncomparable version is not positive", "dev-trunk", failingPluginList, "2.0.27", Status{State: StateUnknown, Version: "dev-trunk", Latest: "2.0.27"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.lockedAt != "" {
				dir = projectWithPlugin(t, tc.lockedAt)
			}
			c := &Checker{ProjectRoot: dir, ShopwareVersion: "6.7.0.0", PluginList: tc.pluginList, StorePlugins: storeVersion(tc.latest)}

			if tc.lockedAt != "" && tc.want.State != StateMissing {
				tc.want.Package = lockedPackage
			}

			assert.Equal(t, tc.want, c.Check(t.Context()))
		})
	}
}

func TestCheckPassesShopwareVersionToStore(t *testing.T) {
	var gotVersion string
	var gotNames []string
	c := &Checker{
		ProjectRoot:     projectWithPlugin(t, "2.0.20"),
		ShopwareVersion: "6.6.10.3",
		PluginList:      pluginList(activeList),
		StorePlugins: func(_ context.Context, _, shopwareVersion string, names []string) ([]account_api.StorePlugin, error) {
			gotVersion, gotNames = shopwareVersion, names
			return nil, nil
		},
	}

	c.Check(t.Context())

	assert.Equal(t, "6.6.10.3", gotVersion)
	assert.Equal(t, []string{TechnicalName}, gotNames)
}

func TestCheckStoreUnavailable(t *testing.T) {
	c := &Checker{
		ProjectRoot:     projectWithPlugin(t, "2.0.20"),
		ShopwareVersion: "6.7.0.0",
		PluginList:      pluginList(activeList),
		StorePlugins: func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
			return nil, errors.New("registry unavailable")
		},
	}

	assert.Equal(t, StateUnknown, c.Check(t.Context()).State)
}

func TestCheckWithoutShopwareVersionSkipsStore(t *testing.T) {
	called := false
	c := &Checker{
		ProjectRoot: projectWithPlugin(t, "2.0.20"),
		PluginList:  pluginList(activeList),
		StorePlugins: func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
			called = true
			return nil, nil
		},
	}

	assert.Equal(t, StateUnknown, c.Check(t.Context()).State)
	assert.False(t, called)
}

func TestCheckFindsPluginInCustomPlugins(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "custom", "plugins", TechnicalName, "composer.json"), `{"name":"swag/platform-security","version":"2.0.20"}`)
	c := &Checker{ProjectRoot: dir, ShopwareVersion: "6.7.0.0", StorePlugins: storeVersion("2.0.27")}

	assert.Equal(t, Status{State: StateOutdated, Version: "2.0.20", Latest: "2.0.27"}, c.Check(t.Context()))
}

func TestNextStepsOnlyWhenActionIsNeeded(t *testing.T) {
	for _, state := range []State{StateMissing, StateInactive, StateOutdated} {
		assert.NotEmpty(t, Status{State: state}.NextSteps())
	}
	for _, state := range []State{StateUnknown, StateUpToDate} {
		assert.Empty(t, Status{State: state}.NextSteps())
	}
}

func TestNextStepsCoverTheShopLifecycle(t *testing.T) {
	// An unregistered plugin needs the plugin list refreshed before installing.
	registered := Status{State: StateInactive}.NextSteps()
	unregistered := Status{State: StateInactive, Unregistered: true}.NextSteps()
	assert.Len(t, unregistered, len(registered)+1)
	assert.Contains(t, unregistered[0], "plugin:refresh")
	assert.Equal(t, registered[0], unregistered[1])

	// An outdated Composer package is downloaded first, then applied by the
	// shop; a version that is already downloaded is only applied.
	outdated := Status{State: StateOutdated, Package: lockedPackage}.NextSteps()
	downloaded := Status{State: StateOutdated, Package: lockedPackage, Downloaded: true}.NextSteps()
	assert.Len(t, outdated, 2)
	assert.Contains(t, outdated[0], lockedPackage)
	assert.Equal(t, []string{outdated[1]}, downloaded)
	assert.Contains(t, downloaded[0], "plugin:update")
}
