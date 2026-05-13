package packagist

import (
	"testing"

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
