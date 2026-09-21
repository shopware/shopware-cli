package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
	"github.com/shopware/shopware-cli/internal/oci/ocitest"
)

func TestDockerExecutorOmitsTWhenTTYRequested(t *testing.T) {
	ctx, cancel := context.WithCancel(WithTTY(t.Context()))
	t.Cleanup(cancel)
	exec := &DockerExecutor{projectRoot: "/project"}

	for _, p := range []*Process{
		exec.ConsoleCommand(ctx, "cache:clear"),
		exec.ComposerCommand(ctx, "install"),
		exec.PHPCommand(ctx, "-v"),
		exec.NPMCommand(ctx, "run", "dev"),
	} {
		assert.NotContains(t, p.Cmd.Args(), "-T", "WithTTY compose exec must allocate a TTY: %v", p.Cmd.Args())
	}
}

func TestDockerExecutorPassesTByDefault(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	for _, p := range []*Process{
		exec.ConsoleCommand(t.Context(), "cache:clear"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.PHPCommand(t.Context(), "-v"),
		exec.NPMCommand(t.Context(), "run", "dev"),
	} {
		assert.Contains(t, p.Cmd.Args(), "-T", "compose exec must disable TTY unless WithTTY: %v", p.Cmd.Args())
	}
}

func TestDockerStopEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		opts        StopOptions
		projectName string
		want        []string
	}{
		{
			name: "plain down",
			opts: StopOptions{},
			want: []string{"compose", "down"},
		},
		{
			name: "down removes volumes",
			opts: StopOptions{RemoveVolumes: true},
			want: []string{"compose", "down", "--volumes"},
		},
		{
			name:        "pinned project keeps -p before down",
			projectName: "sw-shop-abc123",
			opts:        StopOptions{},
			want:        []string{"compose", "-p", "sw-shop-abc123", "down"},
		},
		{
			name:        "pinned project with volumes",
			projectName: "sw-shop-abc123",
			opts:        StopOptions{RemoveVolumes: true},
			want:        []string{"compose", "-p", "sw-shop-abc123", "down", "--volumes"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &ocitest.Runtime{}
			ctx := oci.WithRuntime(t.Context(), rt)

			dockerExec := &DockerExecutor{projectRoot: t.TempDir(), composeProjectName: tc.projectName}

			require.NoError(t, dockerExec.StopEnvironment(ctx, tc.opts))

			require.Len(t, rt.Cmds, 1)
			assert.Equal(t, append([]string{"docker"}, tc.want...), rt.Cmds[0].ArgsV)
		})
	}
}
