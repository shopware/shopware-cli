package verifier

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var knownReleases = []string{
	"6.7.14.2", "6.7.14.1", "6.7.14.0", "6.7.13.1", "6.7.0.0", "6.7.0.0-RC1", "6.7.0.0-dev",
	"6.6.10.21", "6.6.0.0", "6.6.0.0-RC1", "6.5.8.18", "6.8.0.0-RC1", "dev-trunk",
}

func TestResolveTargetVersion(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		requested string
		want      string
		wantErr   string
	}{
		"exact release":            {requested: "6.7.14.2", want: "6.7.14.2"},
		"v prefix":                 {requested: "v6.7.13.1", want: "6.7.13.1"},
		"minor picks newest":       {requested: "6.7", want: "6.7.14.2"},
		"patch picks newest":       {requested: "6.7.14", want: "6.7.14.2"},
		"older minor":              {requested: "6.5", want: "6.5.8.18"},
		"exact prerelease allowed": {requested: "6.7.0.0-rc1", want: "6.7.0.0-RC1"},
		"minor ignores prerelease": {requested: "6.8", wantErr: `--target-version "6.8" has no Shopware release yet. Newest release: 6.7.14.2`},
		"unknown patch":            {requested: "6.7.99.0", wantErr: `--target-version "6.7.99.0" is not a Shopware release. Closest 6.7 releases: 6.7.14.2, 6.7.14.1, 6.7.14.0`},
		"unknown old minor":        {requested: "6.2.0.0", wantErr: `--target-version "6.2.0.0" is not a Shopware release. Newest release: 6.7.14.2`},
		"keyword":                  {requested: "latest", wantErr: `--target-version "latest" is not a version. Pass a release such as 6.7.14.2, or a minor such as 6.7 to use its newest release`},
		"constraint":               {requested: "~6.7", wantErr: `--target-version "~6.7" is not a version. Pass a release such as 6.7.14.2, or a minor such as 6.7 to use its newest release`},
		"wildcard":                 {requested: "6.7.x", wantErr: `--target-version "6.7.x" is not a version. Pass a release such as 6.7.14.2, or a minor such as 6.7 to use its newest release`},
		"major only":               {requested: "6", wantErr: `--target-version "6" is not a version. Pass a release such as 6.7.14.2, or a minor such as 6.7 to use its newest release`},
		"empty":                    {requested: " ", wantErr: "--target-version must not be empty"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTargetVersion(tc.requested, knownReleases)

			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveTargetVersionWithoutKnownReleases(t *testing.T) {
	t.Parallel()

	_, err := ResolveTargetVersion("6.7.14.2", nil)

	assert.EqualError(t, err, `--target-version "6.7.14.2" is not a Shopware release`)
}

func TestTargetVersionCompletionsListMinorsAndStableReleasesNewestFirst(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		"6.7", "6.7.14.2", "6.7.14.1", "6.7.14.0", "6.7.13.1", "6.7.0.0",
		"6.6", "6.6.10.21", "6.6.0.0",
		"6.5", "6.5.8.18",
	}, targetVersionCompletions(knownReleases))
}

func TestIsExactRelease(t *testing.T) {
	t.Parallel()

	assert.True(t, isExactRelease("6.7.14.2"))
	assert.False(t, isExactRelease("6.7.14"))
	assert.False(t, isExactRelease("6.7.14.2-RC1"))
	assert.False(t, isExactRelease(""))
}

func TestTargetVersionCompletionsUsesTheReleaseList(t *testing.T) {
	stubShopwareVersionList(t, knownReleases, nil)

	assert.Equal(t, targetVersionCompletions(knownReleases), TargetVersionCompletions(t.Context()))
}

func TestTargetVersionCompletionsAreEmptyWhenTheLookupFails(t *testing.T) {
	stubShopwareVersionList(t, nil, errors.New("offline"))

	assert.Nil(t, TargetVersionCompletions(t.Context()))
}
