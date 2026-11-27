package executor_test

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// versionOutput answers every command with stdout and a successful exit.
func versionOutput(commands *[]runtime.Command, stdout string) runtimeFunc {
	return func(_ context.Context, cmd runtime.Command, options runtime.Options) (runtime.Process, error) {
		*commands = append(*commands, cmd)

		if _, err := io.WriteString(options.Stdout, stdout); err != nil {
			return nil, err
		}

		return completedProcess{}, nil
	}
}

func TestVersionReadsToolOutput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version func(context.Context, *executor.Executor) (*version.Version, error)
		stdout  string
		command runtime.Command
		want    string
	}{
		{
			name:    "PHP",
			version: phpVersion,
			stdout:  "PHP 8.3.6 (cli) (built: Apr 11 2024 20:23:38) (NTS)\nCopyright (c) The PHP Group\n",
			command: runtime.Command{Name: "php", Args: []string{"-v"}},
			want:    "8.3.6",
		},
		{
			name:    "PHP with distribution suffix",
			version: phpVersion,
			stdout:  "PHP 8.3.6-1ubuntu1 (cli)\n",
			command: runtime.Command{Name: "php", Args: []string{"-v"}},
			want:    "8.3.6",
		},
		{
			name:    "PHP after startup warnings",
			version: phpVersion,
			stdout:  "PHP Warning:  Module \"redis\" is already loaded in Unknown on line 0\nPHP 8.2.0 (cli)\n",
			command: runtime.Command{Name: "php", Args: []string{"-v"}},
			want:    "8.2.0",
		},
		{
			name:    "Node.js",
			version: nodeVersion,
			stdout:  "v20.11.1\n",
			command: runtime.Command{Name: "node", Args: []string{"-v"}},
			want:    "20.11.1",
		},
		{
			name:    "Node.js nightly build",
			version: nodeVersion,
			stdout:  "v23.0.0-nightly20240612a1b2c3d4e5\n",
			command: runtime.Command{Name: "node", Args: []string{"-v"}},
			want:    "23.0.0",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var commands []runtime.Command
			runner := executor.New(versionOutput(&commands, tt.stdout))

			installed, err := tt.version(t.Context(), runner)

			require.NoError(t, err)
			assert.Equal(t, tt.want, installed.String())
			assert.Equal(t, []runtime.Command{tt.command}, commands)
		})
	}
}

func TestVersionRejectsUnexpectedOutput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version func(context.Context, *executor.Executor) (*version.Version, error)
		stdout  string
	}{
		{name: "PHP", version: phpVersion, stdout: "PHP x.y (cli)"},
		{name: "Node.js", version: nodeVersion, stdout: "unknown"},
		{name: "Node.js without output", version: nodeVersion},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var commands []runtime.Command
			runner := executor.New(versionOutput(&commands, tt.stdout))

			_, err := tt.version(t.Context(), runner)

			require.Error(t, err)
			if tt.stdout != "" {
				assert.ErrorContains(t, err, tt.stdout)
			}
		})
	}
}

func TestVersionReportsProbeFailures(t *testing.T) {
	for _, tt := range []struct {
		name       string
		start      func(context.Context, runtime.Options) (runtime.Process, error)
		wantErr    error
		wantText   string
		unwantText string
	}{
		{
			name: "missing executable",
			start: func(context.Context, runtime.Options) (runtime.Process, error) {
				return nil, fmt.Errorf("cannot start %q: %w", "php", exec.ErrNotFound)
			},
			wantErr:    exec.ErrNotFound,
			unwantText: "output:",
		},
		{
			name: "unsuccessful exit",
			start: func(_ context.Context, options runtime.Options) (runtime.Process, error) {
				if _, err := io.WriteString(options.Stderr, "broken installation"); err != nil {
					return nil, err
				}

				return completedProcess{result: runtime.Result{ExitCode: 3}}, nil
			},
			wantText: "broken installation",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := runtimeFunc(func(
				ctx context.Context,
				_ runtime.Command,
				options runtime.Options,
			) (runtime.Process, error) {
				return tt.start(ctx, options)
			})

			_, err := executor.New(target).PHP().Version(t.Context())

			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantText != "" {
				assert.ErrorContains(t, err, tt.wantText)
			}
			if tt.unwantText != "" {
				assert.NotContains(t, err.Error(), tt.unwantText)
			}
		})
	}
}

// blockingProcess runs until its context ends, like a hanging version command.
type blockingProcess struct {
	completedProcess
	done <-chan struct{}
	err  func() error
}

func (p blockingProcess) Wait() (runtime.Result, error) {
	<-p.done
	return runtime.Result{ExitCode: -1}, p.err()
}

func TestVersionGivesUpOnHangingCommand(t *testing.T) {
	executor.SetVersionProbeTimeout(t, 10*time.Millisecond)

	target := runtimeFunc(func(ctx context.Context, _ runtime.Command, _ runtime.Options) (runtime.Process, error) {
		return blockingProcess{done: ctx.Done(), err: ctx.Err}, nil
	})

	_, err := executor.New(target).Node().Version(t.Context())

	assert.ErrorIs(t, err, executor.ErrVersionProbeTimeout)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
}

func phpVersion(ctx context.Context, runner *executor.Executor) (*version.Version, error) {
	return runner.PHP().Version(ctx)
}

func nodeVersion(ctx context.Context, runner *executor.Executor) (*version.Version, error) {
	return runner.Node().Version(ctx)
}
