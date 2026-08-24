package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

// writeRecordingDocker installs a docker stub on PATH that records every
// argument it is invoked with, one per line, into argsFile.
func writeRecordingDocker(t *testing.T, argsFile string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake docker binary requires a POSIX shell")
	}

	shPath, err := exec.LookPath("sh")
	require.NoError(t, err)

	script := fmt.Sprintf("#!%s\nprintf '%%s\\n' \"$@\" > %q\n", shPath, argsFile)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir)
}

// recordedArgs reads the arguments captured by writeRecordingDocker.
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

func stubHostTerminals(t *testing.T, interactive bool) {
	t.Helper()

	orig := hostStdinStdoutAreTerminals
	hostStdinStdoutAreTerminals = func() bool { return interactive }
	t.Cleanup(func() { hostStdinStdoutAreTerminals = orig })
}

func TestDockerExecutorOmitsTWhenInteractive(t *testing.T) {
	stubHostTerminals(t, true)
	system.SetTUIActive(false)
	t.Cleanup(func() { system.SetTUIActive(false) })

	exec := &DockerExecutor{projectRoot: "/project"}

	for _, p := range []*Process{
		exec.ConsoleCommand(t.Context(), "cache:clear"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.PHPCommand(t.Context(), "-v"),
		exec.NPMCommand(t.Context(), "run", "dev"),
	} {
		assert.NotContains(t, p.Cmd.Args, "-T", "interactive compose exec must allocate a TTY: %v", p.Cmd.Args)
	}
}

func TestDockerExecutorPassesTWhenTUIActive(t *testing.T) {
	stubHostTerminals(t, true)
	system.SetTUIActive(true)
	t.Cleanup(func() { system.SetTUIActive(false) })

	exec := &DockerExecutor{projectRoot: "/project"}

	for _, p := range []*Process{
		exec.ConsoleCommand(t.Context(), "cache:clear"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.PHPCommand(t.Context(), "-v"),
		exec.NPMCommand(t.Context(), "run", "dev"),
	} {
		assert.Contains(t, p.Cmd.Args, "-T", "TUI-launched compose exec must keep -T: %v", p.Cmd.Args)
	}
}

func TestDockerExecutorPassesTWhenNonInteractive(t *testing.T) {
	stubHostTerminals(t, false)

	exec := &DockerExecutor{projectRoot: "/project"}

	for _, p := range []*Process{
		exec.ConsoleCommand(t.Context(), "cache:clear"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.PHPCommand(t.Context(), "-v"),
		exec.NPMCommand(t.Context(), "run", "dev"),
	} {
		assert.Contains(t, p.Cmd.Args, "-T", "non-interactive compose exec must disable TTY: %v", p.Cmd.Args)
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
			writeRecordingDocker(t, argsFile)

			dockerExec := &DockerExecutor{projectRoot: t.TempDir(), composeProjectName: tc.projectName}

			require.NoError(t, dockerExec.StopEnvironment(t.Context(), tc.opts))

			assert.Equal(t, tc.want, recordedArgs(t, argsFile))
		})
	}
}
