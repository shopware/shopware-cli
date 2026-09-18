package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerRepo(t *testing.T) {
	assert.Equal(t, "shopware/deployment-helper", ownerRepo("https://github.com/shopware/deployment-helper"))
	assert.Equal(t, "shopware/deployment-helper", ownerRepo("https://github.com/shopware/deployment-helper.git"))
}

func TestLatestStableTag(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(`
deadbeef	refs/tags/0.1.5
deadbeef	refs/tags/0.1.7
deadbeef	refs/tags/0.1.7^{}
deadbeef	refs/tags/0.1.6
deadbeef	refs/tags/0.2.0-RC1
`), "\n")

	got, err := latestStableTag(lines)
	require.NoError(t, err)
	assert.Equal(t, "0.1.7", got) // highest stable; the peeled dup and 0.2.0-RC1 are ignored

	_, err = latestStableTag([]string{"deadbeef\trefs/tags/1.0.0-beta"})
	assert.Error(t, err) // only pre-releases → no stable tag
}
