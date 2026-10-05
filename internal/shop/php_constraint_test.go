package shop

import (
	"strings"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/stretchr/testify/assert"
)

func TestPHPConstraintHighestSupported(t *testing.T) {
	t.Run("nil receiver returns highest", func(t *testing.T) {
		var c *PHPConstraint
		assert.Equal(t, "8.5", c.HighestSupported())
	})

	t.Run("single constraint caps version", func(t *testing.T) {
		assert.Equal(t, "8.3", NewPHPConstraint("~8.2.0 || ~8.3.0").HighestSupported())
	})

	t.Run("multiple constraints take intersection", func(t *testing.T) {
		assert.Equal(t, "8.4", NewPHPConstraint("^8.2", "<8.5").HighestSupported())
	})

	t.Run("invalid constraint is ignored", func(t *testing.T) {
		assert.Equal(t, "8.5", NewPHPConstraint("not-a-constraint").HighestSupported())
	})
}

func TestPHPConstraintSupportedVersions(t *testing.T) {
	t.Run("nil receiver returns all", func(t *testing.T) {
		var c *PHPConstraint
		assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, c.SupportedVersions())
	})

	t.Run("filters by constraint", func(t *testing.T) {
		assert.Equal(t, []string{"8.2", "8.3"}, NewPHPConstraint("~8.2.0 || ~8.3.0").SupportedVersions())
	})

	t.Run("multiple constraints take intersection", func(t *testing.T) {
		assert.Equal(t, []string{"8.2", "8.3", "8.4"}, NewPHPConstraint("^8.2", "<8.5").SupportedVersions())
	})

	t.Run("no match falls back to full list", func(t *testing.T) {
		assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, NewPHPConstraint("^9.0").SupportedVersions())
	})

	t.Run("invalid constraint is ignored", func(t *testing.T) {
		assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, NewPHPConstraint("not-a-constraint").SupportedVersions())
	})
}

func TestPHPConstraintCheck(t *testing.T) {
	t.Run("nil receiver always matches", func(t *testing.T) {
		var c *PHPConstraint
		assert.True(t, c.Check("8.2.0"))
	})

	t.Run("version satisfies constraint", func(t *testing.T) {
		assert.True(t, NewPHPConstraint("^8.2").Check("8.3.7"))
	})

	t.Run("version below constraint fails", func(t *testing.T) {
		assert.False(t, NewPHPConstraint("^8.3").Check("8.2.10"))
	})

	t.Run("invalid php version returns false", func(t *testing.T) {
		assert.False(t, NewPHPConstraint("^8.2").Check("not-a-version"))
	})
}

func TestPHPConstraintForShopwareVersion(t *testing.T) {
	releases := []repository.Version{
		{Version: "v6.6.0.0", Require: map[string]string{"php": "~8.2.0 || ~8.3.0"}},
		{Version: "dev-trunk", Require: map[string]string{"php": "~8.3.0 || ~8.4.0"}},
		{Version: "6.5.0.0"},
	}

	t.Run("looks up a released version ignoring the v prefix", func(t *testing.T) {
		c := PHPConstraintForShopwareVersion(releases, "6.6.0.0")
		assert.Equal(t, "~8.2.0 || ~8.3.0", c.String())
	})

	t.Run("resolves the constraint of a dev branch", func(t *testing.T) {
		c := PHPConstraintForShopwareVersion(releases, VersionTrunk)
		assert.Equal(t, "~8.3.0 || ~8.4.0", c.String())
		assert.Equal(t, []string{"8.3", "8.4"}, c.SupportedVersions())
	})

	t.Run("returns nil for an unknown version", func(t *testing.T) {
		assert.Nil(t, PHPConstraintForShopwareVersion(releases, "6.4.0.0"))
	})

	t.Run("returns an unconstrained result when the release declares no php requirement", func(t *testing.T) {
		c := PHPConstraintForShopwareVersion(releases, "6.5.0.0")
		assert.Equal(t, "", c.String())
		assert.True(t, c.Check("8.2.0"))
	})
}

func TestValidatePHPVersion(t *testing.T) {
	for _, supported := range SupportedPHPVersions {
		assert.NoError(t, ValidatePHPVersion(supported))
	}

	t.Run("an unsupported series is rejected", func(t *testing.T) {
		err := ValidatePHPVersion("8.0")
		assert.ErrorContains(t, err, "8.0")
		assert.ErrorContains(t, err, strings.Join(SupportedPHPVersions, ", "))
	})

	t.Run("a patch level is rejected", func(t *testing.T) {
		// The value doubles as a Docker image tag and a config pin, so it must be
		// the major.minor series.
		assert.Error(t, ValidatePHPVersion("8.3.19"))
	})

	t.Run("an empty version is rejected", func(t *testing.T) {
		assert.Error(t, ValidatePHPVersion(""))
	})
}
