package projectupgrade

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestCombinedRegistryPrefersStoreForOwnedPackages(t *testing.T) {
	t.Parallel()

	// swag/paypal is a commercial store plugin: vendor-named, only on the
	// store. The store must answer even though the name has no
	// store.shopware.com/ prefix.
	store := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{
		"swag/paypal": {{Version: "9.0.0"}},
	}}
	packagistReg := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{}}

	combined := &CombinedRegistry{Store: store, Packagist: packagistReg}

	versions, err := combined.GetPackageVersions(t.Context(), "swag/paypal")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "9.0.0", versions[0].Version)
}

func TestCombinedRegistryFallsBackToPackagistForPublicPackages(t *testing.T) {
	t.Parallel()

	// Store knows nothing about a public package; Packagist resolves it.
	store := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{}}
	packagistReg := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{
		"frosh/tools": {{Version: "2.0.0"}},
	}}

	combined := &CombinedRegistry{Store: store, Packagist: packagistReg}

	versions, err := combined.GetPackageVersions(t.Context(), "frosh/tools")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "2.0.0", versions[0].Version)
}

func TestCombinedRegistryFallsBackWhenStoreUnavailable(t *testing.T) {
	t.Parallel()

	// No token -> store load fails with ErrRegistryUnavailable; public
	// packages must still resolve via Packagist.
	store := &fakeRegistry{err: ErrRegistryUnavailable}
	packagistReg := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{
		"frosh/tools": {{Version: "2.0.0"}},
	}}

	combined := &CombinedRegistry{Store: store, Packagist: packagistReg}

	versions, err := combined.GetPackageVersions(t.Context(), "frosh/tools")
	require.NoError(t, err)
	require.Len(t, versions, 1)
}

func TestCombinedRegistryNilStoreUsesPackagist(t *testing.T) {
	t.Parallel()

	packagistReg := &fakeRegistry{versions: map[string][]packagist.ComposerPackageVersion{
		"frosh/tools": {{Version: "2.0.0"}},
	}}
	combined := &CombinedRegistry{Packagist: packagistReg}

	versions, err := combined.GetPackageVersions(t.Context(), "frosh/tools")
	require.NoError(t, err)
	require.Len(t, versions, 1)
}

func TestCombinedRegistryNilPackagistReturnsUnavailable(t *testing.T) {
	t.Parallel()

	combined := &CombinedRegistry{}
	_, err := combined.GetPackageVersions(context.Background(), "frosh/tools")
	assert.ErrorIs(t, err, ErrRegistryUnavailable)
}
