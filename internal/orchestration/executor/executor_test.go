package executor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

type runtimeFunc func(context.Context, runtime.Command, runtime.Options) (runtime.Process, error)

func (f runtimeFunc) Start(
	ctx context.Context,
	cmd runtime.Command,
	opts runtime.Options,
) (runtime.Process, error) {
	return f(ctx, cmd, opts)
}

type completedProcess struct {
	result runtime.Result
	err    error
}

func (p completedProcess) Wait() (runtime.Result, error) { return p.result, p.err }
func (completedProcess) Stop(ctx context.Context) error  { return ctx.Err() }
func (completedProcess) Kill(ctx context.Context) error  { return ctx.Err() }

func TestCommandCopiesDefaultsAndInputs(t *testing.T) {
	var commands []runtime.Command
	target := runtimeFunc(func(
		_ context.Context,
		cmd runtime.Command,
		_ runtime.Options,
	) (runtime.Process, error) {
		commands = append(commands, cmd)
		return completedProcess{}, nil
	})

	env := map[string]string{"BASE": "base"}
	runner := executor.New(target).WithDir("/root").WithEnv(env)
	env["BASE"] = "changed"
	args := []string{"argument with spaces", "$(literal)"}
	base := runner.Command("tool", args...)
	args[0] = "changed"
	overrides := map[string]string{"EXTRA": "extra"}
	derived := base.WithDir("/other").WithEnv(overrides)
	overrides["EXTRA"] = "changed"
	otherRunner := runner.WithDir("/third").WithEnv(map[string]string{"BASE": "override"})

	for _, cmd := range []*executor.Command{base, derived, otherRunner.Command("other"), base} {
		_, err := cmd.Run(t.Context(), runtime.Options{})
		require.NoError(t, err)
	}

	require.Len(t, commands, 4)
	wantFirst := runtime.Command{
		Name: "tool",
		Args: []string{"argument with spaces", "$(literal)"},
		Dir:  "/root",
		Env:  map[string]string{"BASE": "base"},
	}
	assert.Equal(t, wantFirst, commands[0])
	assert.Equal(t, "/other", commands[1].Dir)
	assert.Equal(t, map[string]string{"BASE": "base", "EXTRA": "extra"}, commands[1].Env)
	assert.Equal(t, map[string]string{"BASE": "override"}, commands[2].Env)
	assert.Equal(t, "/third", commands[2].Dir)
	assert.Equal(t, commands[0], commands[3])
}

type contextKey struct{}

// recordingProcess makes swapped, skipped or repeated termination requests observable.
type recordingProcess struct {
	completedProcess
	onStop func(context.Context) error
	onKill func(context.Context) error
}

func (p *recordingProcess) Stop(ctx context.Context) error {
	return p.onStop(ctx)
}

func (p *recordingProcess) Kill(ctx context.Context) error {
	return p.onKill(ctx)
}
