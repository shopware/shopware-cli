package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/cmd/project"
	"github.com/shopware/shopware-cli/logging"
)

func executeRootWithCommand(t *testing.T, runErr error, args ...string) (string, error) {
	t.Helper()

	probe := &cobra.Command{
		Use: "usage-probe",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runErr
		},
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	rootCmd.SetArgs(append([]string{"usage-probe"}, args...))
	err := rootCmd.ExecuteContext(context.Background())

	return out.String(), err
}

func TestRuntimeErrorDoesNotPrintUsage(t *testing.T) {
	runErr := errors.New("validation found problems")

	out, err := executeRootWithCommand(t, runErr)

	assert.ErrorIs(t, err, runErr)
	assert.NotContains(t, out, "Usage:")
}

func TestInvocationErrorPrintsUsage(t *testing.T) {
	out, err := executeRootWithCommand(t, nil, "--unknown-flag")

	assert.ErrorContains(t, err, "unknown flag")
	assert.Contains(t, out, "Usage:")
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantLog  []string
	}{
		{
			name:     "success",
			err:      nil,
			wantCode: 0,
		},
		{
			name:     "runtime error is logged",
			err:      errors.New("something broke"),
			wantCode: 1,
			wantLog:  []string{"something broke"},
		},
		{
			name:     "wrapped runtime error is logged",
			err:      fmt.Errorf("running command: %w", errors.New("something broke")),
			wantCode: 1,
			wantLog:  []string{"running command: something broke"},
		},
		{
			name:     "environment down exits silently",
			err:      project.ErrEnvironmentDown,
			wantCode: 1,
		},
		{
			name:     "wrapped environment down exits silently",
			err:      fmt.Errorf("status: %w", project.ErrEnvironmentDown),
			wantCode: 1,
		},
		{
			name:     "proxy not registered exits silently",
			err:      project.ErrProxyNotRegistered,
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			ctx := logging.WithLogger(context.Background(), zap.New(core).Sugar())

			assert.Equal(t, tt.wantCode, exitCode(ctx, tt.err))

			var messages []string
			for _, entry := range logs.All() {
				assert.Equal(t, zapcore.ErrorLevel, entry.Level)
				messages = append(messages, entry.Message)
			}
			assert.Equal(t, tt.wantLog, messages)
		})
	}
}
