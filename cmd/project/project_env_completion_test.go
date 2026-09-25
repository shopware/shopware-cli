package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEnvCompletionConfig(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

// writeEnvCompletionProject markers make dir a Shopware project root for
// shop.FindClosestShopwareProject, so completion can walk up from subdirectories.
func writeEnvCompletionProject(t *testing.T, dir string) {
	t.Helper()

	writeEnvCompletionConfig(t, dir, "bin/console", "#!/bin/sh\n")
	writeEnvCompletionConfig(t, dir, "composer.json", `{"require": {"shopware/core": "*"}}`)
}

// withProjectConfigPath temporarily sets projectConfigPath for a test and
// restores it afterwards, keeping tests hermetic despite the package-global.
func withProjectConfigPath(t *testing.T, path string) {
	t.Helper()
	t.Setenv("PROJECT_ROOT", "")

	previous := projectConfigPath
	projectConfigPath = path
	t.Cleanup(func() { projectConfigPath = previous })
}

func TestCompleteEnvironmentNames(t *testing.T) {
	dir := t.TempDir()
	writeEnvCompletionConfig(t, dir, ".config/shopware-project.yml", `
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
    url: http://localhost
  staging:
    type: ssh
    url: https://staging.example.com
`)

	withProjectConfigPath(t, "")
	t.Chdir(dir)

	completions, directive := completeEnvironmentNames(projectExtensionListCmd, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.ElementsMatch(t, []string{
		"local\thttp://localhost",
		"staging\thttps://staging.example.com",
	}, completions)
}

func TestCompleteEnvironmentNamesMergesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	writeEnvCompletionConfig(t, dir, ".config/shopware-project.yml", `
compatibility_date: "2026-01-01"
environments:
  local:
    url: http://localhost
`)
	writeEnvCompletionConfig(t, dir, ".config/shopware-project.local.yml", `
environments:
  extra:
    url: http://extra
`)

	withProjectConfigPath(t, "")
	t.Chdir(dir)

	// ReadConfig merges the .local override, so both names must complete.
	completions, _ := completeEnvironmentNames(projectExtensionListCmd, nil, "")
	assert.ElementsMatch(t, []string{
		"local\thttp://localhost",
		"extra\thttp://extra",
	}, completions)
}

func TestCompleteEnvironmentNamesNoConfig(t *testing.T) {
	dir := t.TempDir()

	withProjectConfigPath(t, "")
	t.Chdir(dir)

	completions, directive := completeEnvironmentNames(projectExtensionListCmd, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Empty(t, completions)
}

func TestCompleteEnvironmentNamesExplicitProjectConfig(t *testing.T) {
	dir := t.TempDir()
	custom := writeEnvCompletionConfig(t, dir, "custom.yml", `
environments:
  custom-env:
    url: http://custom
`)

	withProjectConfigPath(t, custom)

	completions, _ := completeEnvironmentNames(projectExtensionListCmd, nil, "")
	assert.ElementsMatch(t, []string{"custom-env\thttp://custom"}, completions)
}

func TestCompleteEnvironmentNamesFindsParentConfig(t *testing.T) {
	dir := t.TempDir()
	writeEnvCompletionConfig(t, dir, ".config/shopware-project.yml", `
environments:
  parent-env:
    url: http://parent
`)
	writeEnvCompletionProject(t, dir)

	withProjectConfigPath(t, "")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755))
	t.Chdir(filepath.Join(dir, "sub", "deep"))

	completions, _ := completeEnvironmentNames(projectExtensionListCmd, nil, "")
	assert.ElementsMatch(t, []string{"parent-env\thttp://parent"}, completions)
}
