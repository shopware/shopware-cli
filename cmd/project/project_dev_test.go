package project

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestProjectDevCmdMetadata(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "dev", projectDevCmd.Use)
	assert.NotEmpty(t, projectDevCmd.Short)
	assert.NotEmpty(t, projectDevCmd.Long)
	assert.NotNil(t, projectDevCmd.RunE)
}

func TestProjectDevStartCmdMetadata(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "start", projectDevStartCmd.Use)
	assert.NotEmpty(t, projectDevStartCmd.Short)
	assert.NotNil(t, projectDevStartCmd.RunE)
}

func TestProjectDevStopCmdMetadata(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "stop", projectDevStopCmd.Use)
	assert.NotEmpty(t, projectDevStopCmd.Short)
	assert.NotNil(t, projectDevStopCmd.RunE)
}

func TestProjectDevCmdSubcommandsRegistered(t *testing.T) {
	t.Parallel()

	subUses := make(map[string]*cobra.Command)
	for _, c := range projectDevCmd.Commands() {
		subUses[c.Use] = c
	}

	assert.Contains(t, subUses, "start")
	assert.Contains(t, subUses, "stop")
}

func TestProjectDevCmdRegisteredOnRoot(t *testing.T) {
	t.Parallel()

	var found bool
	for _, c := range projectRootCmd.Commands() {
		if c == projectDevCmd {
			found = true
			break
		}
	}
	assert.True(t, found, "projectDevCmd should be registered on projectRootCmd")
}

func TestProjectDevCmdHasNoLocalFlags(t *testing.T) {
	t.Parallel()

	var flagNames []string
	projectDevCmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		flagNames = append(flagNames, f.Name)
	})

	assert.Empty(t, flagNames, "projectDevCmd is not expected to declare local flags")
}

func TestProjectDevStartCmdHasNoLocalFlags(t *testing.T) {
	t.Parallel()

	var flagNames []string
	projectDevStartCmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		flagNames = append(flagNames, f.Name)
	})

	assert.Empty(t, flagNames)
}

func TestProjectDevStopCmdHasNoLocalFlags(t *testing.T) {
	t.Parallel()

	var flagNames []string
	projectDevStopCmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		flagNames = append(flagNames, f.Name)
	})

	assert.Empty(t, flagNames)
}

func TestProjectDevCmdArgsAcceptsNone(t *testing.T) {
	t.Parallel()

	if projectDevCmd.Args != nil {
		assert.NoError(t, projectDevCmd.Args(projectDevCmd, []string{}))
	}
}
