package executor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
	"github.com/shopware/shopware-cli/logging"
)

func TestDescriptionDoesNotExposeCommandData(t *testing.T) {
	cmd := executor.New(runtimeFunc(nil)).
		WithEnv(map[string]string{"KEY": "value"}).
		Command("tool", "argument")

	description := cmd.Description()
	description.Args[0] = "changed"
	description.Env["KEY"] = "changed"

	assert.Equal(t, []string{"argument"}, cmd.Description().Args)
	assert.Equal(t, map[string]string{"KEY": "value"}, cmd.Description().Env)
}

func TestStartPreservesInputContextAndProcessControls(t *testing.T) {
	input := bytes.NewBufferString("input")
	ctx, cancelExecution := context.WithCancel(context.WithValue(t.Context(), contextKey{}, "execution"))
	defer cancelExecution()
	canceled, cancel := context.WithCancel(context.WithValue(ctx, contextKey{}, "stop"))
	cancel()
	killContext := context.WithValue(ctx, contextKey{}, "kill")
	stopErr, killErr := errors.New("stop request failed"), errors.New("kill request failed")

	var requests []string
	var executionContext context.Context

	underlying := &recordingProcess{
		onStop: func(actual context.Context) error {
			assert.Equal(t, "stop", actual.Value(contextKey{}))
			assert.ErrorIs(t, actual.Err(), context.Canceled)
			requests = append(requests, "stop")
			return stopErr
		},
		onKill: func(actual context.Context) error {
			assert.Equal(t, "kill", actual.Value(contextKey{}))
			assert.NoError(t, actual.Err())
			requests = append(requests, "kill")
			return killErr
		},
	}

	target := runtimeFunc(func(
		actual context.Context,
		_ runtime.Command,
		opts runtime.Options,
	) (runtime.Process, error) {
		executionContext = actual
		assert.Equal(t, "execution", actual.Value(contextKey{}))
		require.NotNil(t, opts.Stdin)
		data, err := io.ReadAll(opts.Stdin)
		require.NoError(t, err)
		assert.Equal(t, "input", string(data))
		assert.True(t, opts.TTY)

		return underlying, nil
	})

	process, err := executor.New(target).Command("tool").Start(ctx, runtime.Options{
		Stdin: input, TTY: true, Capture: runtime.CaptureCombined,
	})

	require.NoError(t, err)
	assert.ErrorIs(t, process.Stop(canceled), stopErr)
	assert.ErrorIs(t, process.Kill(killContext), killErr)
	assert.Equal(t, []string{"stop", "kill"}, requests)

	cancelExecution()
	select {
	case <-executionContext.Done():
		assert.ErrorIs(t, executionContext.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("execution context lost parent cancellation")
	}
}

func TestStartFailureHasUnknownExitCode(t *testing.T) {
	failure := errors.New("cannot start tool")
	target := runtimeFunc(func(context.Context, runtime.Command, runtime.Options) (runtime.Process, error) {
		return nil, failure
	})

	result, err := executor.New(target).Command("missing").Run(
		t.Context(),
		runtime.Options{Capture: runtime.CaptureSeparate},
	)

	require.ErrorIs(t, err, failure)
	assert.Equal(t, -1, result.ExitCode)
	assert.Equal(t, runtime.CaptureSeparate, result.Output.Mode)
}

func TestRunTurnsUnsuccessfulExitIntoExitError(t *testing.T) {
	failure := errors.New("output forwarding failed")

	for _, tt := range []struct {
		name       string
		result     runtime.Result
		runtimeErr error
		exitCode   int
		wantExit   bool
		wantErr    error
	}{
		{name: "success", result: runtime.Result{ExitCode: 0}},
		{name: "unsuccessful exit", result: runtime.Result{ExitCode: 7}, exitCode: 7, wantExit: true},
		{
			name:     "signal death",
			result:   runtime.Result{ExitCode: -1, Signal: "SIGKILL"},
			exitCode: -1,
			wantExit: true,
		},
		{
			name:       "runtime failure keeps the exit code in the result",
			result:     runtime.Result{ExitCode: 7},
			runtimeErr: failure,
			exitCode:   7,
			wantErr:    failure,
		},
		{
			name:       "cancellation",
			result:     runtime.Result{ExitCode: -1},
			runtimeErr: context.Canceled,
			exitCode:   -1,
			wantErr:    context.Canceled,
		},
	} {
		for _, capture := range []runtime.CaptureMode{runtime.CaptureNone, runtime.CaptureSeparate} {
			t.Run(fmt.Sprintf("%s/capture %d", tt.name, capture), func(t *testing.T) {
				target := runtimeFunc(func(context.Context, runtime.Command, runtime.Options) (runtime.Process, error) {
					return completedProcess{result: tt.result, err: tt.runtimeErr}, nil
				})

				result, err := executor.New(target).Command("tool").Run(t.Context(), runtime.Options{Capture: capture})

				assert.Equal(t, tt.exitCode, result.ExitCode)
				var exit *runtime.ExitError
				assert.Equal(t, tt.wantExit, errors.As(err, &exit))
				if tt.wantExit {
					assert.Equal(t, tt.exitCode, exit.Code)
					assert.Equal(t, tt.result.Signal, exit.Signal)
				}
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				}
				if !tt.wantExit && tt.wantErr == nil {
					assert.NoError(t, err)
				}
			})
		}
	}
}

func TestExecutorLogsStartFailureWithoutSensitiveCommandData(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())
	failure := errors.New("sensitive runtime diagnostic")
	target := runtimeFunc(func(context.Context, runtime.Command, runtime.Options) (runtime.Process, error) {
		return nil, failure
	})

	cmd := executor.New(target).
		WithDir("/workspace").
		WithEnv(map[string]string{"SECRET": "secret environment"}).
		Command("tool", "secret argument")

	_, err := cmd.Start(ctx, runtime.Options{})
	require.ErrorIs(t, err, failure)

	failures := observed.FilterField(zap.String("event", "executor_start_failed")).All()
	require.Len(t, failures, 1)
	assert.Equal(t, "tool", failures[0].ContextMap()["executable"])

	encoded, err := json.Marshal(observed.All())
	require.NoError(t, err)
	for _, secret := range []string{"secret environment", "secret argument", failure.Error()} {
		assert.NotContains(t, string(encoded), secret)
	}
}
