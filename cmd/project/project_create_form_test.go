package project

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestResolveFormVersion(t *testing.T) {
	t.Parallel()

	t.Run("trunk selection installs the dev branch", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, shop.VersionTrunk, resolveFormVersion(shop.VersionTrunk, ""))
	})

	t.Run("trunk selection discards a stale patch version from a previous pass", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, shop.VersionTrunk, resolveFormVersion(shop.VersionTrunk, "6.6.10.0"))
	})

	t.Run("latest selection discards a stale patch version from a previous pass", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, shop.VersionLatest, resolveFormVersion(shop.VersionLatest, "6.6.10.0"))
	})

	t.Run("patch version of the selected minor group is kept", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "6.6.10.0", resolveFormVersion("6.6", "6.6.10.0"))
	})

	t.Run("empty patch selection falls back to latest", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, shop.VersionLatest, resolveFormVersion("6.6", ""))
	})
}
