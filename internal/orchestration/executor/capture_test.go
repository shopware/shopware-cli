package executor_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func TestCaptureAndStreamingPreserveFailure(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		mode                     runtime.CaptureMode
		stdout, stderr, combined string
	}{
		{name: "disabled", mode: runtime.CaptureNone},
		{name: "separate", mode: runtime.CaptureSeparate, stdout: "outtail", stderr: "err"},
		{name: "combined", mode: runtime.CaptureCombined, combined: "outerrtail"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var streamed bytes.Buffer

			target := runtimeFunc(func(
				_ context.Context,
				_ runtime.Command,
				opts runtime.Options,
			) (runtime.Process, error) {
				assert.Equal(t, runtime.CaptureNone, opts.Capture, "capture belongs to the executor")

				if opts.Stdout != nil {
					_, _ = io.WriteString(opts.Stdout, "out")
				}
				if opts.Stderr != nil {
					_, _ = io.WriteString(opts.Stderr, "err")
				}
				if opts.Stdout != nil {
					_, _ = io.WriteString(opts.Stdout, "tail")
				}

				return completedProcess{result: runtime.Result{ExitCode: 2}}, nil
			})

			options := runtime.Options{Stdout: &streamed, Stderr: &streamed, Capture: tt.mode}
			result, err := executor.New(target).Command("tool").Run(t.Context(), options)

			var exit *runtime.ExitError
			require.ErrorAs(t, err, &exit)
			assert.Equal(t, 2, exit.Code)
			assert.Equal(t, 2, result.ExitCode)
			assert.Equal(t, "outerrtail", streamed.String())

			assert.Equal(t, tt.mode, result.Output.Mode)
			assert.Equal(t, tt.stdout, string(result.Output.Stdout))
			assert.Equal(t, tt.stderr, string(result.Output.Stderr))
			assert.Equal(t, tt.combined, string(result.Output.Combined))
		})
	}
}

func TestCaptureStopsAtLimitAndKeepsForwarding(t *testing.T) {
	executor.SetCaptureLimit(t, 5)

	var streamed bytes.Buffer
	target := runtimeFunc(func(_ context.Context, _ runtime.Command, opts runtime.Options) (runtime.Process, error) {
		_, _ = io.WriteString(opts.Stdout, "out")
		_, _ = io.WriteString(opts.Stderr, "err")
		_, _ = io.WriteString(opts.Stdout, "tail")

		return completedProcess{result: runtime.Result{ExitCode: 2}}, nil
	})

	options := runtime.Options{Stdout: &streamed, Stderr: &streamed, Capture: runtime.CaptureSeparate}
	result, err := executor.New(target).Command("tool").Run(t.Context(), options)

	require.ErrorIs(t, err, executor.ErrCaptureLimit)
	var exit *runtime.ExitError
	assert.NotErrorAs(t, err, &exit)
	assert.Equal(t, 2, result.ExitCode)
	assert.Equal(t, "out", string(result.Output.Stdout))
	assert.Equal(t, "er", string(result.Output.Stderr))
	assert.Equal(t, "outerrtail", streamed.String())
}

func TestInvalidCaptureDoesNotStart(t *testing.T) {
	target := runtimeFunc(func(context.Context, runtime.Command, runtime.Options) (runtime.Process, error) {
		t.Fatal("invalid settings must not start a command")
		return completedProcess{}, nil
	})

	_, err := executor.New(target).Command("tool").Start(t.Context(), runtime.Options{Capture: 255})
	require.Error(t, err)
}

func TestCaptureWithoutStreaming(t *testing.T) {
	target := runtimeFunc(func(
		_ context.Context,
		_ runtime.Command,
		opts runtime.Options,
	) (runtime.Process, error) {
		_, err := io.WriteString(opts.Stdout, "out")
		require.NoError(t, err)
		_, err = io.WriteString(opts.Stderr, "err")
		require.NoError(t, err)

		return completedProcess{}, nil
	})

	result, err := executor.New(target).Command("tool").Run(
		t.Context(),
		runtime.Options{Capture: runtime.CaptureSeparate},
	)

	require.NoError(t, err)
	assert.Equal(t, "out", string(result.Output.Stdout))
	assert.Equal(t, "err", string(result.Output.Stderr))
}

func TestConcurrentCaptureAndWaitSnapshots(t *testing.T) {
	var outputs runtime.Options
	target := runtimeFunc(func(
		_ context.Context,
		_ runtime.Command,
		opts runtime.Options,
	) (runtime.Process, error) {
		outputs = opts
		return completedProcess{}, nil
	})

	var streamed bytes.Buffer
	process, err := executor.New(target).Command("tool").Start(t.Context(), runtime.Options{
		Stdout: &streamed, Stderr: &streamed, Capture: runtime.CaptureCombined,
	})
	require.NoError(t, err)

	require.NotNil(t, outputs.Stdout)
	require.NotNil(t, outputs.Stderr)

	// Runtime writers remain live after Start, until the process completes.
	var writes sync.WaitGroup
	for _, output := range []struct {
		writer io.Writer
		data   string
	}{
		{writer: outputs.Stdout, data: "o"},
		{writer: outputs.Stderr, data: "e"},
	} {
		writes.Go(func() {
			for range 100 {
				_, writeErr := io.WriteString(output.writer, output.data)
				assert.NoError(t, writeErr)
			}
		})
	}
	writes.Wait()

	want := bytes.Clone(streamed.Bytes())
	require.Len(t, want, 200)
	assert.Equal(t, 100, bytes.Count(want, []byte("o")))
	assert.Equal(t, 100, bytes.Count(want, []byte("e")))

	const waiters = 10
	results := make([]runtime.Result, waiters)
	errs := make([]error, waiters)
	var group sync.WaitGroup
	for i := range waiters {
		group.Go(func() { results[i], errs[i] = process.Wait() })
	}
	group.Wait()

	for i := range waiters {
		require.NoError(t, errs[i])
		require.Equal(t, want, results[i].Output.Combined)
	}

	results[0].Output.Combined[0] = 'X'
	for _, result := range results[1:] {
		assert.Equal(t, want, result.Output.Combined, "Wait results must own their captured bytes")
	}

	later, err := process.Wait()
	require.NoError(t, err)
	assert.Equal(t, want, later.Output.Combined)
}

func TestCapturePreservesOutputWhenForwardingFails(t *testing.T) {
	failure := errors.New("output unavailable")
	target := runtimeFunc(func(
		_ context.Context,
		_ runtime.Command,
		opts runtime.Options,
	) (runtime.Process, error) {
		_, err := io.WriteString(opts.Stdout, "partial output")
		return completedProcess{result: runtime.Result{ExitCode: 0}, err: err}, nil
	})

	options := runtime.Options{Stdout: failingWriter{failure}, Capture: runtime.CaptureSeparate}
	result, err := executor.New(target).Command("tool").Run(t.Context(), options)

	require.ErrorIs(t, err, failure)
	assert.Equal(t, "partial output", string(result.Output.Stdout))
}

func TestCapturePreservesResultsAndIndependentFailures(t *testing.T) {
	forwarding := errors.New("cannot forward output")
	cleanup := errors.New("cannot clean up execution")

	for _, tt := range []struct {
		name   string
		code   int
		err    error
		causes []error
	}{
		{
			name:   "successful workload with forwarding failure",
			code:   0,
			err:    forwarding,
			causes: []error{forwarding},
		},
		{
			name:   "exit and forwarding failure",
			code:   1,
			err:    forwarding,
			causes: []error{forwarding},
		},
		{
			name:   "exit and cleanup failure",
			code:   1,
			err:    cleanup,
			causes: []error{cleanup},
		},
		{
			name:   "unknown status with cancellation",
			code:   -1,
			err:    context.Canceled,
			causes: []error{context.Canceled},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := runtimeFunc(func(
				_ context.Context,
				_ runtime.Command,
				opts runtime.Options,
			) (runtime.Process, error) {
				_, err := io.WriteString(opts.Stdout, "partial report")
				require.NoError(t, err)

				return completedProcess{result: runtime.Result{ExitCode: tt.code}, err: tt.err}, nil
			})

			result, err := executor.New(target).Command("tool").Run(
				t.Context(),
				runtime.Options{Capture: runtime.CaptureSeparate},
			)

			require.Error(t, err)
			for _, cause := range tt.causes {
				assert.ErrorIs(t, err, cause)
			}

			assert.Equal(t, tt.code, result.ExitCode)
			assert.Equal(t, "partial report", string(result.Output.Stdout))

			var exit *runtime.ExitError
			assert.False(t, errors.As(err, &exit), "a runtime failure is never reported as an exit")
		})
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
