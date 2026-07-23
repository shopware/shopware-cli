// Package upgradetui implements the interactive Shopware upgrade wizard
// (`shopware-cli project upgrade`): a full-screen Bubble Tea program that
// walks through readiness checks, target version selection, extension
// compatibility, plan review, and the guided upgrade execution. All backend
// logic lives in internal/shop/upgrade; this package renders panels as app.Content
// hosted by the internal/tui/app shell.
package upgradetui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

type panel int

const (
	panelIntro panel = iota
	panelCheck
	panelPrepare
	panelReview
	panelRun
	panelDone
)

// Options wires the wizard to a project.
type Options struct {
	ProjectRoot string
	// EnvName is the label shown in the header, e.g. "local".
	EnvName  string
	Executor executor.Executor
}

// Model is the wizard screen hosted by the app shell.
type Model struct {
	opts     Options
	host     app.Host
	upgrader *upgrade.ProjectUpgrader

	width      int
	mainHeight int

	panel panel

	intro   introState
	check   checkState
	prepare prepareState
	review  reviewState
	run     runState
	done    doneState

	// prepareGen counts preparation runs; see prepareState.gen.
	prepareGen int
}

// New creates the wizard model starting at the intro panel.
func New(opts Options) *Model {
	return &Model{
		opts:     opts,
		upgrader: upgrade.NewProjectUpgrader(opts.ProjectRoot, opts.Executor),
		panel:    panelIntro,
		intro:    newIntroState(),
		check:    newCheckState(),
	}
}

// NewApp assembles the wizard inside the application shell: wizard header as
// chrome, ctrl+c interception while the upgrade runs, and window titles.
func NewApp(opts Options) *app.App {
	shell, _ := newAppWithModel(opts)
	return shell
}

// newAppWithModel wires the model into the shell and also returns the model,
// so tests can inspect wizard state.
func newAppWithModel(opts Options) (*app.App, *Model) {
	m := New(opts)

	shell := app.New(app.Options{
		Content:           m,
		Header:            m.headerView,
		Footer:            m.footerView,
		WindowTitleFunc:   m.windowTitle,
		FullscreenOverlay: app.Ptr(false),
	})
	// During a running upgrade, ctrl+c cancels the runner instead of quitting.
	shell.RegisterCommand(app.Command{
		ID:    app.CmdQuit,
		Title: "Quit",
		Run:   func(*app.App) tea.Cmd { return m.handleQuit() },
	})
	m.host = shell
	return shell, m
}

func (m *Model) Init() tea.Cmd {
	return nil
}

// handleQuit implements the quit-key behavior: during a running upgrade,
// ctrl+c cancels the runner (which rolls back) instead of quitting.
func (m *Model) handleQuit() tea.Cmd {
	if m.panel == panelRun && !m.run.finished {
		m.run.cancel()
		return nil
	}
	return tea.Quit
}

func (m *Model) Update(msg tea.Msg) (app.Content, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = msg.Width
		m.mainHeight = msg.Height - chromeRows
		return m, nil
	}

	switch m.panel {
	case panelIntro:
		return m.updateIntro(msg)
	case panelCheck:
		return m.updateCheck(msg)
	case panelPrepare:
		return m.updatePrepare(msg)
	case panelReview:
		return m.updateReview(msg)
	case panelRun:
		return m.updateRun(msg)
	case panelDone:
		return m.updateDone(msg)
	}
	return m, nil
}
