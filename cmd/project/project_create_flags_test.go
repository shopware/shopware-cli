package project

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

func TestParseCreateFlagsMapsFlagsAndArgs(t *testing.T) {
	flags := projectCreateCmd.PersistentFlags()
	// Flag values and Changed state persist on the package-level command.
	t.Cleanup(func() {
		flags.VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})

	// parseCreateFlags reads interactivity from the command context.
	oldCtx := projectCreateCmd.Context()
	t.Cleanup(func() { projectCreateCmd.SetContext(oldCtx) })
	projectCreateCmd.SetContext(system.WithInteraction(context.Background(), false))

	require.NoError(t, flags.Set("version", "6.6.10.0"))
	require.NoError(t, flags.Set("php-version", "8.3"))
	require.NoError(t, flags.Set("without-elasticsearch", "true"))

	opts := parseCreateFlags(projectCreateCmd, []string{"my-shop", "6.5.0.0"})

	assert.Equal(t, "my-shop", opts.projectFolder)
	// The version flag wins over the second positional argument.
	assert.Equal(t, "6.6.10.0", opts.selectedVersion)
	// The deprecated flag inverts the elasticsearch default and marks it explicit.
	assert.False(t, opts.withElasticsearch)
	assert.True(t, opts.elasticsearchExplicit)
	assert.True(t, opts.phpVersionExplicit)
	assert.Equal(t, "8.3", opts.phpVersion)
	assert.False(t, opts.interactive)

	// Without the flag the second positional argument selects the version.
	require.NoError(t, flags.Set("version", ""))
	opts = parseCreateFlags(projectCreateCmd, []string{"my-shop", "6.5.0.0"})
	assert.Equal(t, "6.5.0.0", opts.selectedVersion)
}
