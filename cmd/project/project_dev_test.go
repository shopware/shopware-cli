package project

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// stubDevExecutor embeds the interface so only the methods devEnvironment.start
// actually touches need implementations; the embedded nil is never called.
type stubDevExecutor struct {
	executor.Executor
	startErr error
}

func (s *stubDevExecutor) Type() string {
	return "local"
}

func (s *stubDevExecutor) StartEnvironment(ctx context.Context) error {
	return s.startErr
}

// newPortConflictFlagCommand returns a command carrying only the
// on-port-conflict flag set to mode.
func newPortConflictFlagCommand(t *testing.T, mode string) *cobra.Command {
	cmd := &cobra.Command{Use: "dev"}
	cmd.Flags().String("on-port-conflict", mode, "")
	cmd.SetContext(t.Context())
	return cmd
}

func TestDevStart_OnPortConflictFlagValidation(t *testing.T) {
	env := &devEnvironment{
		cfg:      &shop.Config{},
		executor: &stubDevExecutor{startErr: errors.New("must not be reached")},
	}

	err := env.start(newPortConflictFlagCommand(t, "bogus"))

	assert.ErrorContains(t, err, `invalid value "bogus" for --on-port-conflict, must be "fail" or "random"`)
}

func TestDevStart_ValidPortConflictModePassesValidation(t *testing.T) {
	startErr := errors.New("sentinel from StartEnvironment")
	env := &devEnvironment{
		executor: &stubDevExecutor{startErr: startErr},
	}

	for _, mode := range []string{portConflictModeFail, portConflictModeRandom} {
		t.Run(mode, func(t *testing.T) {
			err := env.start(newPortConflictFlagCommand(t, mode))

			// The sentinel proves the run got past flag validation and
			// conflict resolution (a no-op for non-docker executors) into
			// the actual environment start.
			assert.ErrorIs(t, err, startErr)
			assert.ErrorContains(t, err, "starting environment")
		})
	}
}
