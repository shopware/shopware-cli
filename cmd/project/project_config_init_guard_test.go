package project

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

// The guard keeps `config init` from hanging a headless CI job inside a form.
func TestConfigInitHeadlessGuardAndEmptyValidator(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := &cobra.Command{}
	cmd.SetContext(system.WithInteraction(t.Context(), false))

	err := projectConfigInitCmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires interaction")
	assert.NoFileExists(t, dir+"/.shopware-project.yml")

	assert.Error(t, emptyValidator(""))
	assert.NoError(t, emptyValidator("x"))
}
