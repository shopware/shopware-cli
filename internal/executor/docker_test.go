package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
)

// scriptRuntime is an oci.Runtime that runs the stub shell script at path
// instead of a real container runtime binary.
type scriptRuntime struct{ path string }

func (s scriptRuntime) Binary() string { return s.path }

func (s scriptRuntime) Command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, s.path, args...)
}

func (s scriptRuntime) ComposeCommand(ctx context.Context, args ...string) *exec.Cmd {
	return s.Command(ctx, append([]string{"compose"}, args...)...)
}

// writeStubRuntime writes script to a stub binary and returns it as an
// oci.Runtime, ready to be injected with oci.WithRuntime.
func writeStubRuntime(t *testing.T, script string) oci.Runtime {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake runtime stub requires a POSIX shell")
	}

	shPath, err := exec.LookPath("sh")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "oci-stub")
	require.NoError(t, os.WriteFile(path, []byte("#!"+shPath+"\n"+script), 0o755))

	return scriptRuntime{path: path}
}

// writeRecordingRuntime returns an oci.Runtime stub that records every
// argument it is invoked with, one per line, into argsFile.
func writeRecordingRuntime(t *testing.T, argsFile string) oci.Runtime {
	t.Helper()
	return writeStubRuntime(t, fmt.Sprintf("printf '%%s\\n' \"$@\" > %q\n", argsFile))
}

// recordedArgs reads the arguments captured by writeRecordingRuntime.
func recordedArgs(t *testing.T, argsFile string) []string {
	t.Helper()

	data, err := os.ReadFile(argsFile)
	require.NoError(t, err)

	var args []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			args = append(args, line)
		}
	}

	return args
}

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
		assert.NotContains(t, p.Cmd.Args, "-T", "WithTTY compose exec must allocate a TTY: %v", p.Cmd.Args)
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
		assert.Contains(t, p.Cmd.Args, "-T", "compose exec must disable TTY unless WithTTY: %v", p.Cmd.Args)
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
			argsFile := filepath.Join(t.TempDir(), "args.txt")
			ctx := oci.WithRuntime(t.Context(), writeRecordingRuntime(t, argsFile))

			dockerExec := &DockerExecutor{projectRoot: t.TempDir(), composeProjectName: tc.projectName}

			require.NoError(t, dockerExec.StopEnvironment(ctx, tc.opts))

			assert.Equal(t, tc.want, recordedArgs(t, argsFile))
		})
	}
}
