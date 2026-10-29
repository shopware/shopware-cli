package extension

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/system"
)

func TestPrepareAdminWatcherReportsTheFailingStep(t *testing.T) {
	e := &storefrontWatchExecutor{process: &executor.Process{Cmd: exec.CommandContext(t.Context(), "sh", "-c", "exit 1")}}

	_, err := PrepareAdminWatcher(t.Context(), t.TempDir(), e, nil)

	var stepErr *WatcherStepError
	require.ErrorAs(t, err, &stepErr)
	assert.Equal(t, WatcherStepFeatureDump, stepErr.Step)
	assert.Equal(t, []string{"feature:dump"}, e.args)
	var exitErr *exec.ExitError
	assert.ErrorAs(t, err, &exitErr, "the step keeps the original error")
}

func TestFailedStepKeepsNil(t *testing.T) {
	assert.NoError(t, failedStep(WatcherStepFeatureDump, nil))
}

func TestPrepareStorefrontWatcherReportsTheFailingStep(t *testing.T) {
	t.Run("theme selection", func(t *testing.T) {
		ctx := system.WithInteraction(t.Context(), false)

		_, err := PrepareStorefrontWatcher(ctx, t.TempDir(), &storefrontWatchExecutor{}, StorefrontWatcherOptions{}, nil, nil)

		var stepErr *WatcherStepError
		require.ErrorAs(t, err, &stepErr)
		assert.Equal(t, WatcherStepThemeSelect, stepErr.Step)
	})

	t.Run("feature dump", func(t *testing.T) {
		e := &storefrontWatchExecutor{process: &executor.Process{Cmd: exec.CommandContext(t.Context(), "sh", "-c", "exit 1")}}

		_, err := PrepareStorefrontWatcher(t.Context(), t.TempDir(), e, StorefrontWatcherOptions{ThemeID: "theme-id"}, nil, nil)

		var stepErr *WatcherStepError
		require.ErrorAs(t, err, &stepErr)
		assert.Equal(t, WatcherStepFeatureDump, stepErr.Step)
	})
}
