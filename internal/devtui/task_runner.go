package devtui

import (
	"context"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

func (m *Model) runTask(title string, taskFn func() (*exec.Cmd, error)) tea.Cmd {
	m.phase = phaseTask
	m.task = tui.NewTask(title)
	return m.task.Start(taskFn)
}

func (m *Model) runAdminBuild() tea.Cmd {
	return m.runSelfCommand("Building Administration...", "project", "admin-build")
}

func (m *Model) runStorefrontBuild() tea.Cmd {
	return m.runSelfCommand("Building Storefront...", "project", "storefront-build")
}

func (m *Model) runSelfCommand(title string, args ...string) tea.Cmd {
	projectRoot := m.projectRoot
	dockerMode := m.dockerMode

	return m.runTask(title, func() (*exec.Cmd, error) {
		selfBin, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(context.Background(), selfBin, append(args, projectRoot)...)
		if dockerMode {
			cmd.Dir = projectRoot
		}
		return cmd, nil
	})
}

func (m *Model) runCacheClear() tea.Cmd {
	e := m.executor
	return m.runTask("Clearing Cache...", func() (*exec.Cmd, error) {
		return e.ConsoleCommand(context.Background(), "cache:clear").Cmd, nil
	})
}
