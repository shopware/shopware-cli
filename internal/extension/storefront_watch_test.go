package extension

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/system"
)

type storefrontWatchExecutor struct {
	executor.Executor
	process *executor.Process
	args    []string
}

func (e *storefrontWatchExecutor) ConsoleCommand(_ context.Context, args ...string) *executor.Process {
	e.args = args
	return e.process
}

func TestStorefrontThemeDumpArgs(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		opts        StorefrontWatcherOptions
		want        []string
		wantErr     string
	}{
		{
			name:        "interactive without a selected theme",
			interactive: true,
			want:        []string{"theme:dump"},
		},
		{
			name:    "non-interactive without a selected theme",
			wantErr: "theme selection requires interaction; pass --sales-channel <id> when using --no-interaction",
		},
		{
			name: "non-interactive with a selected theme",
			opts: StorefrontWatcherOptions{ThemeID: "theme-id"},
			want: []string{"theme:dump", "theme-id"},
		},
		{
			name: "non-interactive with a selected theme and domain",
			opts: StorefrontWatcherOptions{ThemeID: "theme-id", DomainURL: "https://example.com"},
			want: []string{"theme:dump", "theme-id", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := system.WithInteraction(context.Background(), tt.interactive)
			args, err := storefrontThemeDumpArgs(ctx, tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, args)
		})
	}
}

func TestRunStorefrontThemeDumpAttachesInput(t *testing.T) {
	process := &executor.Process{Cmd: exec.Command(os.Args[0], "-test.run=^$")}
	exec := &storefrontWatchExecutor{process: process}
	in := strings.NewReader("selected theme\n")
	var out bytes.Buffer

	require.NoError(t, runStorefrontThemeDump(t.Context(), exec, in, &out, "theme:dump"))
	assert.Equal(t, []string{"theme:dump"}, exec.args)
	assert.Same(t, in, process.Cmd.Stdin)
	assert.Same(t, &out, process.Cmd.Stdout)
	assert.Same(t, &out, process.Cmd.Stderr)
}
