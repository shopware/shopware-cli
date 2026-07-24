package upgradetui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop/upgrade"
)

type checksDoneMsg struct {
	readiness upgrade.Readiness
}

type catalogLoadedMsg struct {
	catalog *upgrade.Catalog
	err     error
}

// overlayClosedMsg is emitted when an overlay closes without a result.
type overlayClosedMsg struct{}

// Panel 3 preparation results. Each arrives independently and carries the
// generation of the preparation run that started it, so results from a
// superseded run (different target) are dropped instead of mixed in.
type envStatusMsg struct {
	gen     int
	running bool
	err     error
}

type packagistMsg struct {
	gen       int
	reachable bool
}

type resolveDoneMsg struct {
	gen    int
	result upgrade.ResolveResult
	err    error
}

type compatDoneMsg struct {
	gen     int
	results []upgrade.ExtensionResult
}

type phpInfoMsg struct {
	gen         int
	requirement string
	installed   string
}

// reportWrittenMsg is the outcome of "Export report".
type reportWrittenMsg struct {
	path string
	err  error
}

// runEventMsg wraps one runner progress event; runClosedMsg fires when the
// runner's event stream ends.
type runEventMsg upgrade.StepEvent

type runClosedMsg struct{}

func runChecksCmd(u *upgrade.ProjectUpgrader) tea.Cmd {
	return func() tea.Msg {
		return checksDoneMsg{readiness: u.RunReadinessChecks(context.Background())}
	}
}

func loadCatalogCmd(u *upgrade.ProjectUpgrader, readiness upgrade.Readiness) tea.Cmd {
	if readiness.CurrentVersion == nil {
		return nil
	}
	current := readiness.CurrentVersion
	return func() tea.Msg {
		catalog, err := u.LoadCatalog(context.Background(), current)
		return catalogLoadedMsg{catalog: catalog, err: err}
	}
}

func envStatusCmd(exec executor.Executor, gen int) tea.Cmd {
	return func() tea.Msg {
		running, err := exec.EnvironmentStatus(context.Background())
		if errors.Is(err, executor.ErrNotSupported) {
			// Environments without lifecycle management (plain local PHP)
			// count as available.
			return envStatusMsg{gen: gen, running: true}
		}
		return envStatusMsg{gen: gen, running: running, err: err}
	}
}

func packagistCmd(u *upgrade.ProjectUpgrader, gen int) tea.Cmd {
	return func() tea.Msg {
		return packagistMsg{gen: gen, reachable: u.PackagistReachable(context.Background())}
	}
}

func resolveCmd(u *upgrade.ProjectUpgrader, target string, gen int) tea.Cmd {
	return func() tea.Msg {
		result, err := u.CheckComposerResolvable(context.Background(), target)
		return resolveDoneMsg{gen: gen, result: result, err: err}
	}
}

func compatCmd(u *upgrade.ProjectUpgrader, current, target *version.Version, extensions []upgrade.InstalledExtension, gen int) tea.Cmd {
	return func() tea.Msg {
		return compatDoneMsg{gen: gen, results: u.CheckExtensions(context.Background(), current, target, extensions)}
	}
}

func phpInfoCmd(u *upgrade.ProjectUpgrader, target *version.Version, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		return phpInfoMsg{
			gen:         gen,
			requirement: u.TargetPHPRequirement(ctx, target),
			installed:   u.InstalledPHPVersion(ctx),
		}
	}
}

func exportReportCmd(u *upgrade.ProjectUpgrader, data upgrade.ReportData) tea.Cmd {
	return func() tea.Msg {
		path, err := u.WriteReport(data)
		return reportWrittenMsg{path: path, err: err}
	}
}

// readRunEventCmd pulls the next runner event; re-issue it after each event.
func readRunEventCmd(events <-chan upgrade.StepEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return runClosedMsg{}
		}
		return runEventMsg(ev)
	}
}
