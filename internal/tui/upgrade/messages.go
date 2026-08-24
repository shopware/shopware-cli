package upgrade

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/executor"
	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
)

type checksDoneMsg struct {
	readiness backend.Readiness
}

type catalogLoadedMsg struct {
	catalog *backend.Catalog
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
	result backend.ResolveResult
	err    error
}

type compatDoneMsg struct {
	gen     int
	results []backend.ExtensionResult
}

// changelogsMsg carries the Store release notes of the planned extension
// updates for the report.
type changelogsMsg struct {
	gen        int
	changelogs []backend.ExtensionChangelog
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
type runEventMsg backend.StepEvent

type runClosedMsg struct{}

func runChecksCmd(ctx context.Context, u *backend.ProjectUpgrader) tea.Cmd {
	return func() tea.Msg {
		return checksDoneMsg{readiness: u.RunReadinessChecks(ctx)}
	}
}

func loadCatalogCmd(ctx context.Context, u *backend.ProjectUpgrader, readiness backend.Readiness) tea.Cmd {
	if readiness.CurrentVersion == nil {
		return nil
	}
	current := readiness.CurrentVersion
	return func() tea.Msg {
		catalog, err := u.LoadCatalog(ctx, current)
		return catalogLoadedMsg{catalog: catalog, err: err}
	}
}

func envStatusCmd(ctx context.Context, exec executor.Executor, gen int) tea.Cmd {
	return func() tea.Msg {
		running, err := exec.EnvironmentStatus(ctx)
		if errors.Is(err, executor.ErrNotSupported) {
			// Environments without lifecycle management (plain local PHP)
			// count as available.
			return envStatusMsg{gen: gen, running: true}
		}
		return envStatusMsg{gen: gen, running: running, err: err}
	}
}

func packagistCmd(ctx context.Context, u *backend.ProjectUpgrader, gen int) tea.Cmd {
	return func() tea.Msg {
		return packagistMsg{gen: gen, reachable: u.PackagistReachable(ctx)}
	}
}

func resolveCmd(ctx context.Context, u *backend.ProjectUpgrader, target string, gen int) tea.Cmd {
	return func() tea.Msg {
		result, err := u.CheckComposerResolvable(ctx, target)
		return resolveDoneMsg{gen: gen, result: result, err: err}
	}
}

func compatCmd(ctx context.Context, u *backend.ProjectUpgrader, current, target *version.Version, extensions []backend.InstalledExtension, gen int) tea.Cmd {
	return func() tea.Msg {
		return compatDoneMsg{gen: gen, results: u.CheckExtensions(ctx, current, target, extensions)}
	}
}

func changelogsCmd(ctx context.Context, u *backend.ProjectUpgrader, target string, results []backend.ExtensionResult, gen int) tea.Cmd {
	// The command goroutine must not share the slice the panel keeps updating.
	results = slices.Clone(results)
	return func() tea.Msg {
		return changelogsMsg{gen: gen, changelogs: u.LoadExtensionChangelogs(ctx, target, results)}
	}
}

func phpInfoCmd(ctx context.Context, u *backend.ProjectUpgrader, target *version.Version, gen int) tea.Cmd {
	return func() tea.Msg {
		return phpInfoMsg{
			gen:         gen,
			requirement: u.TargetPHPRequirement(ctx, target),
			installed:   u.InstalledPHPVersion(ctx),
		}
	}
}

func exportReportCmd(u *backend.ProjectUpgrader, data backend.ReportData) tea.Cmd {
	return func() tea.Msg {
		path, err := u.WriteReport(data)
		return reportWrittenMsg{path: path, err: err}
	}
}

// readRunEventCmd pulls the next runner event; re-issue it after each event.
func readRunEventCmd(events <-chan backend.StepEvent) tea.Cmd {
	if events == nil {
		return func() tea.Msg {
			return runClosedMsg{}
		}
	}
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return runClosedMsg{}
		}
		return runEventMsg(ev)
	}
}
