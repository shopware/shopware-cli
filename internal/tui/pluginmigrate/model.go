// Package pluginmigrate implements the interactive `project autofix
// composer-plugins` wizard: it scans custom/ for extensions Composer does not
// manage, classifies them against the Shopware Store packagist, and migrates
// them (Store requires or path repositories) with rollback on failure. All
// backend logic lives in internal/shop/pluginmigrate, imported here as
// `migrate`; this package renders panels as app.Content hosted by the
// internal/tui/app shell.
package pluginmigrate

import (
	"context"
	"path/filepath"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	migrate "github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

type panel int

const (
	panelWelcome panel = iota
	panelToken
	panelReview
	panelRun
	panelDone
)

// Options wire the wizard to a project.
type Options struct {
	ProjectRoot string
	Executor    executor.Executor
	// Context is the parent context for TUI-launched commands. It is marked
	// with system.WithTUI so docker compose exec keeps -T.
	Context context.Context
}

// Model is the wizard screen hosted by the app shell.
type Model struct {
	opts     Options
	host     app.Host
	migrator *migrate.PluginMigrator
	header   tui.Header

	width      int
	mainHeight int

	panel panel

	scan        []migrate.ScannedExtension
	scanDone    bool
	welcomeYes  bool
	tokenInput  textinput.Model
	tokenFocus  int // 0 input, 1 "continue without token"
	tokenBusy   bool
	tokenErr    string
	token       string
	plan        migrate.Plan
	reviewApply bool

	run  runState
	done doneState

	ctx context.Context
}

type doneState struct {
	succeeded bool
	err       error
}

// New creates the wizard model starting at the welcome panel.
func New(opts Options) *Model {
	ti := textinput.New()
	ti.Placeholder = "Shopware Packagist token"
	ti.CharLimit = 128
	ti.EchoMode = textinput.EchoPassword
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	return &Model{
		opts:       opts,
		migrator:   migrate.NewPluginMigrator(opts.ProjectRoot, opts.Executor),
		header:     tui.NewHeader(),
		panel:      panelWelcome,
		welcomeYes: true,
		tokenInput: ti,
		ctx:        system.WithTUI(ctx),
	}
}

func (m *Model) commandContext() context.Context {
	if m != nil && m.ctx != nil {
		return m.ctx
	}

	return system.WithTUI(context.Background())
}

// NewApp assembles the wizard inside the application shell.
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
	// During a running migration, ctrl+c cancels the run instead of quitting.
	shell.RegisterCommand(app.Command{
		ID:    app.CmdQuit,
		Title: "Quit",
		Run:   func(*app.App) tea.Cmd { return m.handleQuit() },
	})
	m.host = shell
	return shell, m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.header.Init(), scanCmd(m.migrator))
}

// handleQuit implements the quit-key behavior: during a running migration,
// ctrl+c cancels the runner (which rolls back) instead of quitting.
func (m *Model) handleQuit() tea.Cmd {
	if m.panel == panelRun && !m.run.finished {
		if m.run.cancel != nil {
			m.run.cancel()
		}
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.mainHeight = msg.Height - chromeRows
		return m, nil

	case scanDoneMsg:
		m.scanDone = true
		m.scan = msg.extensions
		return m, nil

	case availabilityMsg:
		return m.handleAvailability(msg)
	}

	switch m.panel {
	case panelWelcome:
		return m.updateWelcome(msg)
	case panelToken:
		return m.updateToken(msg)
	case panelReview:
		return m.updateReview(msg)
	case panelRun:
		return m.updateRun(msg)
	case panelDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func projectName(projectRoot string) string {
	return filepath.Base(projectRoot)
}
