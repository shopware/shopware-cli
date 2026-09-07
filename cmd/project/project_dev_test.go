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

// stubDevExecutor embeds the interface so only the methods start() touches need
// implementations.
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

func newPortConflictFlagCommand(t *testing.T, mode string) *cobra.Command {
	t.Helper()

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

func TestDevStart_ValidPortConflictModeStartsEnvironment(t *testing.T) {
	startErr := errors.New("start failed")
	env := &devEnvironment{
		executor: &stubDevExecutor{startErr: startErr},
	}

	for _, mode := range []string{portConflictModeFail, portConflictModeRandom} {
		t.Run(mode, func(t *testing.T) {
			err := env.start(newPortConflictFlagCommand(t, mode))

			assert.ErrorIs(t, err, startErr)
		})
	}
}
