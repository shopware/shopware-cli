// Package upgrade implements the interactive Shopware upgrade wizard
// (`shopware-cli project upgrade`): a full-screen Bubble Tea program that
// walks through readiness checks, target version selection, extension
// compatibility, plan review, and the guided upgrade execution. All backend
// logic lives in internal/shop/upgrade, imported here as `backend`; this
// package renders panels as app.Content hosted by the internal/tui/app shell.
package upgrade

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
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
	// ctx is the command context of the CLI invocation (cancelled on
	// SIGINT/SIGTERM, carries the logger). tea.Cmd closures derive their
	// backend and subprocess contexts from it. Bubbletea's fixed Update(msg)
	// signature offers no parameter path into command builders, so the model
	// has to carry it.
	ctx      context.Context //nolint:containedctx
	opts     Options
	host     app.Host
	upgrader *backend.ProjectUpgrader
	header   tui.Header

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

// commandContext returns the context tea.Cmd closures should derive from.
// Tests construct Model literals without New, so a nil ctx falls back to
// Background.
func (m *Model) commandContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

// New creates the wizard model starting at the intro panel.
func New(ctx context.Context, opts Options) *Model {
	return &Model{
		ctx:      ctx,
		opts:     opts,
		upgrader: backend.NewProjectUpgrader(opts.ProjectRoot, opts.Executor),
		header:   tui.NewHeader(),
		panel:    panelIntro,
		intro:    newIntroState(),
		check:    newCheckState(),
	}
}

// NewApp assembles the wizard inside the application shell: wizard header as
// chrome, ctrl+c interception while the upgrade runs, and window titles.
func NewApp(ctx context.Context, opts Options) *app.App {
	shell, _ := newAppWithModel(ctx, opts)
	return shell
}

// newAppWithModel wires the model into the shell and also returns the model,
// so tests can inspect wizard state.
func newAppWithModel(ctx context.Context, opts Options) (*app.App, *Model) {
	m := New(ctx, opts)

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
	return m.header.Init()
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
	var headerCmd tea.Cmd
	m.header, headerCmd = m.header.Update(msg)

	content, cmd := m.updatePanel(msg)
	return content, tea.Batch(headerCmd, cmd)
}

// updatePanel routes a message to the active panel.
func (m *Model) updatePanel(msg tea.Msg) (app.Content, tea.Cmd) {
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
