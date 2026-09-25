package verifier

import (
	"context"
	"errors"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func stubShopwareVersionList(t *testing.T, versions []string, err error) {
	t.Helper()
	original := getShopwareVersions
	t.Cleanup(func() { getShopwareVersions = original })
	getShopwareVersions = func(context.Context) ([]string, error) {
		return versions, err
	}
}

func mustConstraint(t *testing.T, raw string) *version.Constraints {
	t.Helper()
	cs, err := version.NewConstraint(raw)
	require.NoError(t, err)
	return &cs
}

func TestDetermineBaselinePicksLowestStableMatchingRelease(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.7.14.2", "6.6.10.21", "6.6.0.0", "6.6.0.0-RC1", "6.6.0.0-dev", "6.5.8.0"}, nil)
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, "~6.6.0 || ~6.7.0"), ""))

	assert.Equal(t, "6.6.0.0", cfg.MinShopwareVersion)
	assert.Equal(t, validation.Target{
		Version:          "6.6.0.0",
		Source:           validation.TargetSourceConstraint,
		Constraint:       "~6.6.0 || ~6.7.0",
		WithinConstraint: true,
	}, cfg.Target)
}

func TestDetermineBaselineUsesPrereleaseOnlyWhenNothingStableMatches(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.6.0.0", "6.8.0.0-RC2", "6.8.0.0-RC1"}, nil)
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, "~6.8.0"), ""))

	assert.Equal(t, "6.8.0.0-RC1", cfg.MinShopwareVersion)
	assert.Equal(t, validation.TargetSourceConstraint, cfg.Target.Source)
}

func TestDetermineBaselineFallsBackWhenNothingMatches(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.6.0.0"}, nil)
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, ">=7.0"), ""))

	assert.Equal(t, "6.7.0.0", cfg.MinShopwareVersion)
	assert.Equal(t, validation.Target{
		Version:    "6.7.0.0",
		Source:     validation.TargetSourceFallback,
		Constraint: ">=7.0",
	}, cfg.Target)
}

func TestDetermineBaselineReturnsLookupError(t *testing.T) {
	stubShopwareVersionList(t, nil, errors.New("offline"))

	err := determineBaseline(&ToolConfig{}, mustConstraint(t, "~6.6.0"), "")

	assert.EqualError(t, err, "offline")
}

func TestConstraintString(t *testing.T) {
	cases := map[string]string{
		"~6.6.0 || ~6.7.0": "~6.6.0 || ~6.7.0",
		">=6.6 <6.8":       ">=6.6 <6.8",
		"^6.6":             "^6.6",
	}

	for raw, want := range cases {
		assert.Equal(t, want, constraintString(mustConstraint(t, raw)), raw)
	}
}

func TestDetermineBaselineResolvesExplicitTarget(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.7.14.2", "6.7.14.1", "6.6.10.21", "6.6.0.0"}, nil)
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, "~6.6.0"), "v6.7"))

	assert.Equal(t, "6.7.14.2", cfg.MinShopwareVersion)
	assert.Equal(t, validation.Target{
		Version:    "6.7.14.2",
		Requested:  "6.7",
		Source:     validation.TargetSourceFlag,
		Constraint: "~6.6.0",
	}, cfg.Target)
}

func TestDetermineBaselineMarksTargetInsideConstraint(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.7.14.2", "6.6.10.21"}, nil)
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, "~6.6.0 || ~6.7.0"), "6.6.10.21"))

	assert.True(t, cfg.Target.WithinConstraint)
	assert.Equal(t, "6.6.10.21", cfg.Target.Requested)
}

func TestDetermineBaselineRejectsUnknownTarget(t *testing.T) {
	stubShopwareVersionList(t, []string{"6.7.14.2"}, nil)

	err := determineBaseline(&ToolConfig{}, mustConstraint(t, "~6.7.0"), "6.8")

	assert.EqualError(t, err, `--target-version "6.8" has no Shopware release yet. Newest release: 6.7.14.2`)
}

func TestDetermineBaselineKeepsExactTargetWhenReleaseListIsUnavailable(t *testing.T) {
	stubShopwareVersionList(t, nil, errors.New("offline"))
	cfg := &ToolConfig{}

	require.NoError(t, determineBaseline(cfg, mustConstraint(t, "~6.7.0"), "6.7.14.2"))

	assert.Equal(t, "6.7.14.2", cfg.MinShopwareVersion)
	assert.Equal(t, validation.Target{
		Version:          "6.7.14.2",
		Requested:        "6.7.14.2",
		Source:           validation.TargetSourceFlag,
		Constraint:       "~6.7.0",
		WithinConstraint: true,
		Unverified:       true,
	}, cfg.Target)
}

func TestDetermineBaselineCannotResolveShorthandWhenReleaseListIsUnavailable(t *testing.T) {
	stubShopwareVersionList(t, nil, errors.New("offline"))

	err := determineBaseline(&ToolConfig{}, mustConstraint(t, "~6.7.0"), "6.7")

	assert.EqualError(t, err, `cannot resolve --target-version "6.7" without the Shopware release list: offline. Pass a full release such as 6.7.14.2 to skip the lookup`)
}
