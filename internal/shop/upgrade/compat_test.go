package upgrade

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

type fakeProvider map[string]*repository.Package

func (f fakeProvider) Package(_ context.Context, name string) (*repository.Package, error) {
	if p, ok := f[name]; ok {
		return p, nil
	}
	return nil, repository.ErrPackageNotFound
}

func release(name, ver, coreConstraint string) repository.Version {
	rel := repository.Version{Name: name, Version: ver}
	if coreConstraint != "" {
		rel.Require = map[string]string{"shopware/core": coreConstraint}
	}
	return rel
}

// compatUpgrader builds an upgrader whose package lookups hit an in-memory
// Composer repository and whose Store update check returns fixed results.
func compatUpgrader(t *testing.T, dir string, provider fakeProvider, store []account_api.UpdateCheckExtensionCompatibility, storeErr error) *ProjectUpgrader {
	t.Helper()
	server := httptest.NewServer(repository.NewHandler(provider))
	t.Cleanup(server.Close)

	u := NewProjectUpgrader(dir, nil)
	u.repositories = func(*composer.Json, *composer.Auth) *repository.Set {
		return repository.NewSet(repository.New(server.URL, nil))
	}
	u.extensionUpdates = func(context.Context, string, string, []account_api.UpdateCheckExtension) ([]account_api.UpdateCheckExtensionCompatibility, error) {
		return store, storeErr
	}
	return u
}

func storeStatus(name, statusType, label string) account_api.UpdateCheckExtensionCompatibility {
	return account_api.UpdateCheckExtensionCompatibility{
		Name:   name,
		Status: account_api.UpdateCheckExtensionCompatibilityStatus{Name: label, Label: label, Type: statusType},
	}
}

func compatVersions(t *testing.T) (*version.Version, *version.Version) {
	t.Helper()
	return version.Must(version.NewVersion("6.6.10.3")), version.Must(version.NewVersion("6.7.11.0"))
}

func TestCheckExtensionsClassification(t *testing.T) {
	dir := setupProject(t)
	current, target := compatVersions(t)

	u := compatUpgrader(t, dir, fakeProvider{
		"swag/ok": {Name: "swag/ok", Versions: []repository.Version{
			release("swag/ok", "2.0.0", "~6.6.0 || ~6.7.0"),
		}},
		"swag/needs-update": {Name: "swag/needs-update", Versions: []repository.Version{
			release("swag/needs-update", "9.1.0", "~6.7.0"),
			release("swag/needs-update", "9.0.0", "~6.7.0"),
			release("swag/needs-update", "8.3.1", "~6.6.0"),
		}},
		"swag/blocked": {Name: "swag/blocked", Versions: []repository.Version{
			release("swag/blocked", "3.2.0", "~6.6.0"),
		}},
	}, []account_api.UpdateCheckExtensionCompatibility{
		storeStatus("SwagOk", "success", "Compatible"),
		storeStatus("SwagNeedsUpdate", "warning", "Update required"),
		storeStatus("SwagBlocked", "error", "Not compatible"),
	}, nil)

	extensions := []InstalledExtension{
		{Name: "SwagOk", Package: "swag/ok", Version: "2.0.0", ComposerManaged: true},
		{Name: "SwagNeedsUpdate", Package: "swag/needs-update", Version: "8.3.1", ComposerManaged: true},
		{Name: "SwagBlocked", Package: "swag/blocked", Version: "3.2.0", ComposerManaged: true},
		{Name: "LocalPlugin", Package: "acme/local-plugin", Version: "1.0.0", ComposerManaged: false},
	}

	results := u.CheckExtensions(t.Context(), current, target, extensions)
	require.Len(t, results, 4)

	byName := make(map[string]ExtensionResult)
	order := make([]string, 0, len(results))
	for _, r := range results {
		byName[r.Extension.Name] = r
		order = append(order, r.Extension.Name)
	}

	assert.Equal(t, ExtOK, byName["SwagOk"].Status)
	assert.Equal(t, ExtNeedsUpdate, byName["SwagNeedsUpdate"].Status)
	assert.Equal(t, "9.0.0", byName["SwagNeedsUpdate"].Available, "lowest compatible release wins")
	assert.Equal(t, ExtBlocked, byName["SwagBlocked"].Status)
	assert.Equal(t, ExtReview, byName["LocalPlugin"].Status)

	assert.Equal(t, []string{"SwagBlocked", "SwagNeedsUpdate", "LocalPlugin", "SwagOk"}, order, "most severe first")
}

func TestCheckExtensionsStoreMismatch(t *testing.T) {
	dir := setupProject(t)
	current, target := compatVersions(t)

	// The Store claims compatibility although no Composer release allows 6.7.
	u := compatUpgrader(t, dir, fakeProvider{
		"vendor/example": {Name: "vendor/example", Versions: []repository.Version{
			release("vendor/example", "1.0.0", "~6.6.0"),
		}},
	}, []account_api.UpdateCheckExtensionCompatibility{
		storeStatus("ExampleExtension", "success", "Compatible"),
	}, nil)

	results := u.CheckExtensions(t.Context(), current, target, []InstalledExtension{
		{Name: "ExampleExtension", Package: "vendor/example", Version: "1.0.0", ComposerManaged: true},
	})

	require.Len(t, results, 1)
	assert.Equal(t, ExtMismatch, results[0].Status)
	assert.True(t, results[0].Status.BlocksUpgrade())
	assert.Contains(t, results[0].Detail, "Composer")
}

func TestCheckExtensionsDeprecated(t *testing.T) {
	dir := setupProject(t)
	current, target := compatVersions(t)

	u := compatUpgrader(t, dir, fakeProvider{
		"vendor/old": {Name: "vendor/old", Versions: []repository.Version{
			release("vendor/old", "2.4.1", "~6.6.0"),
		}},
	}, []account_api.UpdateCheckExtensionCompatibility{
		storeStatus("OldSearchAdapter", "error", "Deprecated"),
	}, nil)

	results := u.CheckExtensions(t.Context(), current, target, []InstalledExtension{
		{Name: "OldSearchAdapter", Package: "vendor/old", Version: "2.4.1", ComposerManaged: true},
	})

	require.Len(t, results, 1)
	assert.Equal(t, ExtDeprecated, results[0].Status)
}

func TestCheckExtensionsPackageUnknownAndStoreDown(t *testing.T) {
	dir := setupProject(t)
	current, target := compatVersions(t)

	u := compatUpgrader(t, dir, fakeProvider{}, nil, assert.AnError)

	results := u.CheckExtensions(t.Context(), current, target, []InstalledExtension{
		{Name: "GhostExtension", Package: "vendor/ghost", Version: "1.0.0", ComposerManaged: true},
	})

	require.Len(t, results, 1)
	assert.Equal(t, ExtBlocked, results[0].Status)
	assert.Contains(t, results[0].Detail, "not found")
	assert.Equal(t, "Not available in Store", results[0].StoreLabel)
}

func TestCheckExtensionsPrereleaseOnlyDoesNotCount(t *testing.T) {
	dir := setupProject(t)
	current, target := compatVersions(t)

	u := compatUpgrader(t, dir, fakeProvider{
		"vendor/rc-only": {Name: "vendor/rc-only", Versions: []repository.Version{
			release("vendor/rc-only", "2.0.0-rc1", "~6.7.0"),
			release("vendor/rc-only", "1.0.0", "~6.6.0"),
		}},
	}, nil, nil)

	results := u.CheckExtensions(t.Context(), current, target, []InstalledExtension{
		{Name: "RcOnly", Package: "vendor/rc-only", Version: "1.0.0", ComposerManaged: true},
	})

	require.Len(t, results, 1)
	assert.Equal(t, ExtBlocked, results[0].Status, "prerelease compatibility does not unblock")
}

func TestTargetPHPRequirement(t *testing.T) {
	dir := setupProject(t)
	_, target := compatVersions(t)

	u := compatUpgrader(t, dir, fakeProvider{
		"shopware/core": {Name: "shopware/core", Versions: []repository.Version{
			{Name: "shopware/core", Version: "6.7.11.0", Require: map[string]string{"php": ">=8.2"}},
		}},
	}, nil, nil)

	assert.Equal(t, ">=8.2", u.TargetPHPRequirement(t.Context(), target))
	assert.Empty(t, u.TargetPHPRequirement(t.Context(), version.Must(version.NewVersion("6.9.0.0"))))
}
