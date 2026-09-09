package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestMapAliasArgs_NoArgs(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{}, mapAliasArgs([]string{"shopware-cli"}))
}

func TestMapAliasArgs_RegularBinary(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"shopware-cli", "project", "console", "debug:router"})

	assert.Equal(t, []string{"project", "console", "debug:router"}, args)
}

func TestMapAliasArgs_SwxAlias(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "debug:router", "--env=prod"})

	assert.Equal(t, []string{"project", "console", "debug:router", "--env=prod"}, args)
}

func TestMapAliasArgs_SwxAliasWithoutArgs(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/swx"})

	assert.Equal(t, []string{"project", "console", "list"}, args)
}

func TestMapAliasArgs_SwxExeAlias(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"C:\\tools\\swx.exe", "cache:clear"})

	assert.Equal(t, []string{"project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxCaseInsensitive(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/SWX", "cache:clear"})

	assert.Equal(t, []string{"project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxCompletion(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "completion", "bash"})

	assert.Equal(t, []string{"completion", "bash"}, args)
}

func TestMapAliasArgs_SwxInternalCompletion(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "__complete", "cache:clear"})

	assert.Equal(t, []string{"__complete", "project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxInternalCompletionNoDesc(t *testing.T) {
	t.Parallel()
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "__completeNoDesc", "cache:clear"})

	assert.Equal(t, []string{"__completeNoDesc", "project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxHelp(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"project", "console", "--help"}, mapAliasArgs([]string{"/usr/local/bin/swx", "--help"}))
	assert.Equal(t, []string{"project", "console", "-h"}, mapAliasArgs([]string{"/usr/local/bin/swx", "-h"}))
}

func TestMapAliasArgs_SwxVersion(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"project", "console", "--version"}, mapAliasArgs([]string{"/usr/local/bin/swx", "--version"}))
	assert.Equal(t, []string{"project", "console", "-v"}, mapAliasArgs([]string{"/usr/local/bin/swx", "-v"}))
}

func TestCommandNameFromArgs(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "shopware-cli", commandNameFromArgs([]string{"/usr/local/bin/shopware-cli"}))
	assert.Equal(t, "swx", commandNameFromArgs([]string{"C:\\tools\\swx.exe"}))
	assert.Equal(t, "shopware-cli", commandNameFromArgs(nil))
}

func executeRootWithCommand(t *testing.T, runErr error, args ...string) (string, error) {
	t.Helper()

	probe := &cobra.Command{
		Use: "usage-probe",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runErr
		},
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	rootCmd.SetArgs(append([]string{"usage-probe"}, args...))
	err := rootCmd.ExecuteContext(context.Background())

	return out.String(), err
}

func TestRuntimeErrorDoesNotPrintUsage(t *testing.T) {
	runErr := errors.New("validation found problems")

	out, err := executeRootWithCommand(t, runErr)

	assert.ErrorIs(t, err, runErr)
	assert.NotContains(t, out, "Usage:")
}

func TestInvocationErrorPrintsUsage(t *testing.T) {
	out, err := executeRootWithCommand(t, nil, "--unknown-flag")

	assert.ErrorContains(t, err, "unknown flag")
	assert.Contains(t, out, "Usage:")
}
