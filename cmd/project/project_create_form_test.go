package project

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
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

func TestDockerUnavailableReason(t *testing.T) {
	t.Parallel()

	t.Run("not running", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Docker is not running", dockerUnavailableReason(&system.MissingDependency{Name: "Docker", Reason: "not running"}))
	})

	t.Run("not installed", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Docker is not installed", dockerUnavailableReason(&system.MissingDependency{Name: "Docker", Reason: "not installed"}))
	})
}

func TestValidateDockerChoice(t *testing.T) {
	t.Parallel()

	t.Run("docker allowed when available", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateDockerChoice(tui.Yes, nil))
		assert.NoError(t, validateDockerChoice(tui.No, nil))
	})

	t.Run("local php allowed when docker unavailable", func(t *testing.T) {
		t.Parallel()
		missing := &system.MissingDependency{Name: "Docker", Reason: "not running"}
		assert.NoError(t, validateDockerChoice(tui.No, missing))
	})

	t.Run("docker blocked when daemon not running", func(t *testing.T) {
		t.Parallel()
		missing := &system.MissingDependency{Name: "Docker", Reason: "not running"}
		err := validateDockerChoice(tui.Yes, missing)
		assert.ErrorContains(t, err, "Docker is not running")
		assert.ErrorContains(t, err, "start Docker")
	})

	t.Run("docker blocked when not installed", func(t *testing.T) {
		t.Parallel()
		missing := &system.MissingDependency{Name: "Docker", Reason: "not installed"}
		err := validateDockerChoice(tui.Yes, missing)
		assert.ErrorContains(t, err, "Docker is not installed")
		assert.ErrorContains(t, err, "install Docker")
	})
}

func TestDockerUnavailableError(t *testing.T) {
	t.Parallel()

	t.Run("returns missing dependencies error", func(t *testing.T) {
		t.Parallel()
		err := dockerUnavailableError(&system.MissingDependency{Name: "Docker", Reason: "not running"})
		assert.ErrorContains(t, err, "missing required dependencies")
	})
}
