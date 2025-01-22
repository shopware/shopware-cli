package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/version"
)

func TestDetermineMinVersion(t *testing.T) {
	constraint, _ := version.NewConstraint("~6.5.0")

	matchingVersion := getMinMatchingVersion(&constraint, []string{"6.4.0.0", "6.5.0.0-rc1", "6.5.0.0"})
	assert.Equal(t, "6.5.0.0", matchingVersion)
	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.4.0.0", "6.5.0.0-rc1"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)
	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.5.0.0-rc1", "6.4.0.0"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)

	matchingVersion = getMinMatchingVersion(&constraint, []string{"1.0.0", "2.0.0"})
	assert.Equal(t, DevVersionNumber, matchingVersion)

	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.5.0.0-rc1", "abc", "6.4.0.0"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)
}
