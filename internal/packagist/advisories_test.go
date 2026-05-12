package packagist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityAdvisoryAffects(t *testing.T) {
	a := SecurityAdvisory{
		AffectedVersions: "<6.6.10.15|>=6.7.0.0,<6.7.8.1",
	}

	t.Run("first branch matches", func(t *testing.T) {
		assert.True(t, a.Affects("6.6.10.14"))
	})

	t.Run("second branch matches", func(t *testing.T) {
		assert.True(t, a.Affects("6.7.5.0"))
	})

	t.Run("between branches does not match", func(t *testing.T) {
		assert.False(t, a.Affects("6.6.10.15"))
	})

	t.Run("above all branches does not match", func(t *testing.T) {
		assert.False(t, a.Affects("6.7.8.1"))
	})

	t.Run("version with v prefix", func(t *testing.T) {
		assert.True(t, a.Affects("v6.6.10.14"))
	})

	t.Run("invalid version is not affected", func(t *testing.T) {
		assert.False(t, a.Affects("not-a-version"))
	})
}

func TestFilterAdvisoriesForVersion(t *testing.T) {
	advisories := []SecurityAdvisory{
		{Title: "A", AffectedVersions: "<6.6.10.15"},
		{Title: "B", AffectedVersions: ">=6.7.0.0,<6.7.8.1"},
		{Title: "C", AffectedVersions: ">=7.0.0.0"},
	}

	matching := FilterAdvisoriesForVersion(advisories, "6.7.5.0")
	assert.Len(t, matching, 1)
	assert.Equal(t, "B", matching[0].Title)
}
