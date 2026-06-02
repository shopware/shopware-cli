package projectupgrade

import (
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterUpdateVersionsReturnsCurrentAndNextMajor(t *testing.T) {
	t.Parallel()

	current, err := version.NewVersion("6.5.8.0")
	require.NoError(t, err)

	all := []string{
		"6.4.20.0", // older, dropped
		"6.5.8.0",  // equal, dropped
		"6.5.8.1",
		"6.5.9.0",
		"6.6.0.0",
		"6.6.1.0",
		"6.7.0.0", // two majors away, dropped
	}

	result := FilterUpdateVersions(current, all)

	// next major (6.6) versions come first, descending, then current major (6.5) versions descending.
	assert.Equal(t, []string{"6.6.1.0", "6.6.0.0", "6.5.9.0", "6.5.8.1"}, result)
}

func TestFilterUpdateVersionsDropsReleaseCandidates(t *testing.T) {
	t.Parallel()

	current, err := version.NewVersion("6.5.8.0")
	require.NoError(t, err)

	all := []string{
		"6.5.9.0",
		"6.6.0.0-rc1",
		"6.6.0.0-RC2",
		"6.6.0.0",
	}

	result := FilterUpdateVersions(current, all)
	assert.Equal(t, []string{"6.6.0.0", "6.5.9.0"}, result)
}

func TestFilterUpdateVersionsReturnsEmptyWhenLatest(t *testing.T) {
	t.Parallel()

	current, err := version.NewVersion("6.6.5.0")
	require.NoError(t, err)

	all := []string{"6.5.0.0", "6.5.8.0", "6.6.5.0"}
	result := FilterUpdateVersions(current, all)
	assert.Empty(t, result)
}
