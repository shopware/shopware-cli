package devtui

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// Streaming, completion, and spinner mechanics are covered by the tui.Task
// tests; here only devtui's task wiring is pinned: runTask enters phaseTask
// and the command factories carry the right titles.

func TestRunTask_EntersTaskPhase(t *testing.T) {
	m := &Model{phase: phaseDashboard}

	cmd := m.runTask("Building...", func() (*exec.Cmd, error) { return nil, errors.New("not executed") })
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building...", m.task.Title)
	assert.False(t, m.task.Done())
	assert.NotNil(t, cmd)
}

func TestRunSelfCommand_ConstructsTaskWithoutExecuting(t *testing.T) {
	m := &Model{projectRoot: t.TempDir(), dockerMode: true}

	cmd := m.runSelfCommand("Building Administration...", "project", "admin-build")
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Administration...", m.task.Title)
	assert.NotNil(t, cmd)
}

func TestRunAdminBuild_SetsExpectedTitleAndPhase(t *testing.T) {
	m := &Model{projectRoot: t.TempDir()}
	cmd := m.runAdminBuild()
	assert.NotNil(t, cmd)
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Administration...", m.task.Title)
}

func TestRunStorefrontBuild_SetsExpectedTitleAndPhase(t *testing.T) {
	m := &Model{projectRoot: t.TempDir()}
	cmd := m.runStorefrontBuild()
	assert.NotNil(t, cmd)
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Storefront...", m.task.Title)
}

func TestTaskView_ShowsSpinnerWhileRunningNotAfterDone(t *testing.T) {
	m := Model{width: 100, height: 30, phase: phaseTask}
	m.task = tui.NewTask("Building Administration...")
	ctx := app.Context{Width: 100, Height: 30, MainHeight: 26}

	running := m.View(ctx)
	assert.Contains(t, running, "Building Administration...")

	m.task, _ = m.task.Update(tui.TaskDoneMsg{})
	done := m.View(ctx)
	assert.Contains(t, done, "Building Administration...")
	assert.Contains(t, done, "Done. Press any key to close.")

	// The spinner glyph adds leading runes before the title only while running,
	// so the rendered task header differs between the two states.
	assert.NotEqual(t, running, done)
}
