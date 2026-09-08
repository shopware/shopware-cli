package project

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/asset"
)

func TestSelectExtensionsInteractivelyRejectsEmptyCandidates(t *testing.T) {
	cmd := &cobra.Command{}

	_, err := selectExtensionsInteractively(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no extensions available to select")

	// The storefront bundle is always excluded from the picker, so a list
	// containing only it must fail the same way before any TUI opens.
	_, err = selectExtensionsInteractively(cmd, []asset.Source{{Name: storefrontBundleName}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no extensions available to select")
}
