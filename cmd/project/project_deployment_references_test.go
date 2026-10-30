//go:build deployment

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestDeploymentReferenceCompletions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROJECT_ROOT", "")
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"*"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	t.Chdir(filepath.Join(root, "bin"))
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/quirky-hopper.tar.gz"), "archive")
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/focused-turing.tar.gz"), "archive")
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/ignored.txt"), "not an archive")
	require.NoError(t, os.Mkdir(filepath.Join(root, ".shopware-cli/deployments/directory.tar.gz"), 0o755))

	// Completion resolves the selected backend and honors project config flags.
	config := filepath.Join(t.TempDir(), "custom.yml")
	testhelper.WriteFile(t, config, `
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shop/current
`)
	cmd, _ := newLifecycleCommand(t, nil)
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--project-config", config, "-e", "production"}))
	references, directive := deploymentReferenceCompletions(cmd, nil, "")
	assert.Equal(t, []string{"focused-turing", "quirky-hopper"}, references)
	assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)

	references, directive = deploymentReferenceCompletions(cmd, nil, "fo")
	assert.Equal(t, []string{"focused-turing"}, references)
	assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)

	references, directive = deploymentReferenceCompletions(cmd, []string{"focused-turing"}, "")
	assert.Empty(t, references)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
