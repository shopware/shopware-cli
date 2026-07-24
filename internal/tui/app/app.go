// Package app provides a terminal application shell on top of Bubble Tea v2:
// chrome (header/footer), an overlay stack, keybindings mapped to named
// commands, and quit handling. It hosts a Content screen and routes input
// following the conventions shared by the shopware-cli TUIs: overlays capture
// input and close by returning nil plus an Emit(result) command; ctrl+c runs
// the quit command, which screens may override.
//
// The package is a trimmed copy of lattice's app shell
// (github.com/shyim/lattice/app) with theme, focus, jobs, toasts, and drawer
// support stripped — colors come from the flat internal/tui package instead.
package app

import (
	tea "charm.land/bubbletea/v2"
)

// Options configure a new App shell.
type Options struct {
	// Content is the main screen (required for a useful app).
	Content Content
	// Header renders above the main content each frame (optional).
	Header func(ctx Context) string
	// Footer renders below the main content each frame (optional).
	Footer func(ctx Context) string
	// WindowTitle sets the terminal window title when non-empty.
	WindowTitle string
	// WindowTitleFunc overrides WindowTitle each frame when set.
	WindowTitleFunc func(ctx Context) string
	// QuitKeys binds extra keys to the quit command (ctrl+c is always bound
	// unless DisableDefaultKeys is set).
	QuitKeys []string
	// DisableDefaultKeys skips the default ctrl+c quit binding.
	DisableDefaultKeys bool
	// Bindings are extra key bindings.
	Bindings []Binding
	// FullscreenOverlay when true (default) replaces the entire view with the
	// top overlay. When false, the overlay replaces only the main region and
	// the header/footer chrome stays visible.
	FullscreenOverlay *bool
	// AltScreen enables the alternate screen buffer (default true).
	AltScreen *bool
	// Mouse enables cell-motion mouse when true.
	Mouse bool
}

// App is a Bubble Tea model that hosts content, chrome, and overlays.
type App struct {
	content Content

	width  int
	height int

	headerFn    func(Context) string
	footerFn    func(Context) string
	windowTitle string
	titleFn     func(Context) string
	fullOverlay bool
	altScreen   bool
	mouse       bool

	overlays OverlayStack
	keys     KeyMap
	commands *CommandRegistry

	// status is an optional one-line status message apps can set.
	status string
}

// New constructs an App shell. The returned value implements tea.Model.
func New(opts Options) *App {
	alt := true
	if opts.AltScreen != nil {
		alt = *opts.AltScreen
	}
	fullOverlay := true
	if opts.FullscreenOverlay != nil {
		fullOverlay = *opts.FullscreenOverlay
	}

	a := &App{
		content:     opts.Content,
		headerFn:    opts.Header,
		footerFn:    opts.Footer,
		windowTitle: opts.WindowTitle,
		titleFn:     opts.WindowTitleFunc,
		fullOverlay: fullOverlay,
		altScreen:   alt,
		mouse:       opts.Mouse,
		commands:    NewCommandRegistry(),
	}

	if !opts.DisableDefaultKeys {
		a.keys.Bind(DefaultQuitBindings()...)
	}
	if len(opts.QuitKeys) > 0 {
		a.keys.Bind(Binding{Keys: opts.QuitKeys, Command: CmdQuit, Help: "quit", Global: true})
	}
	a.keys.Bind(opts.Bindings...)

	a.registerBuiltinCommands()
	return a
}

func (a *App) registerBuiltinCommands() {
	a.commands.Register(Command{
		ID:    CmdQuit,
		Title: "Quit",
		Run: func(*App) tea.Cmd {
			return tea.Quit
		},
	})
	a.commands.Register(Command{
		ID:    CmdPopOverlay,
		Title: "Close overlay",
		Run: func(app *App) tea.Cmd {
			app.overlays.Pop()
			return nil
		},
	})
}

// Size returns the last known terminal size.
func (a *App) Size() (width, height int) { return a.width, a.height }

// Commands returns the command registry.
func (a *App) Commands() *CommandRegistry { return a.commands }

// Keys returns a pointer to the keymap for additional Bind calls.
func (a *App) Keys() *KeyMap { return &a.keys }

// SetStatus sets a short status string (available to chrome helpers).
func (a *App) SetStatus(s string) { a.status = s }

// Status returns the status string.
func (a *App) Status() string { return a.status }

// PushOverlay shows an overlay on top (captures input).
func (a *App) PushOverlay(o Overlay) tea.Cmd { return a.overlays.Push(o) }

// PopOverlay closes the top overlay if any.
func (a *App) PopOverlay() { a.overlays.Pop() }

// OverlayOpen reports whether an overlay is active.
func (a *App) OverlayOpen() bool { return a.overlays.Open() }

// TopOverlay returns the top overlay, or nil when none is open.
func (a *App) TopOverlay() Overlay { return a.overlays.Top() }

// Content returns the current content.
func (a *App) Content() Content { return a.content }

// SetContent replaces the main content without initializing it.
func (a *App) SetContent(c Content) { a.content = c }

// RegisterCommand adds a command and optional key binding. Registering with a
// built-in ID (e.g. CmdQuit) replaces the built-in behavior.
func (a *App) RegisterCommand(c Command, keys ...string) {
	a.commands.Register(c)
	if len(keys) > 0 {
		a.keys.Bind(Binding{Keys: keys, Command: c.ID, Help: c.Title})
	}
}

// RunCommand executes a command by ID.
func (a *App) RunCommand(id string) tea.Cmd {
	c, ok := a.commands.Get(id)
	if !ok || c.Run == nil {
		return nil
	}
	return c.Run(a)
}

// Context builds the current frame context.
func (a *App) Context() Context {
	header, footer := a.chrome()
	r := ComputeRegion(max(a.width, 1), max(a.height, 1), header, footer)
	return Context{
		Width:       a.width,
		Height:      a.height,
		MainHeight:  r.Main,
		OverlayOpen: a.overlays.Open(),
		Chrome:      r,
	}
}

func (a *App) chrome() (header, footer string) {
	ctx := Context{
		Width:       a.width,
		Height:      a.height,
		OverlayOpen: a.overlays.Open(),
	}
	if a.headerFn != nil {
		header = a.headerFn(ctx)
	}
	if a.footerFn != nil {
		footer = a.footerFn(ctx)
	}
	return header, footer
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	if a.content == nil {
		return nil
	}
	return a.content.Init()
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = msg.Width
		a.height = msg.Height
		ctx := a.Context()
		PropagateSize(a.content, ctx.Width, ctx.MainHeight)
	}

	var cmds []tea.Cmd

	// Quit wins over everything, including open overlays.
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if cmdID, matched := a.keys.Match(KeyString(key), true); matched && cmdID == CmdQuit {
			return a, a.RunCommand(CmdQuit)
		}
	}

	// An open overlay captures input messages. Non-input messages are also
	// offered to the overlay for its private async work, then continue to
	// Content (resize and background results must not disappear merely
	// because a modal is open).
	if a.overlays.Open() {
		cmd, _ := a.overlays.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if isInputMsg(msg) {
			return a, tea.Batch(cmds...)
		}
	}

	// Bound keys run their command instead of reaching Content.
	if key, ok := msg.(tea.KeyPressMsg); ok && !a.overlays.Open() {
		if cmdID, matched := a.keys.Match(KeyString(key), false); matched {
			return a, tea.Batch(append(cmds, a.RunCommand(cmdID))...)
		}
	}

	if a.content != nil {
		var cmd tea.Cmd
		a.content, cmd = a.content.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return a, tea.Batch(cmds...)
}

// isInputMsg reports messages exclusively owned by an open overlay.
func isInputMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg, tea.PasteMsg, tea.PasteStartMsg, tea.PasteEndMsg:
		return true
	default:
		return false
	}
}

// View implements tea.Model.
func (a *App) View() tea.View {
	if a.width == 0 || a.height == 0 {
		return tea.NewView("")
	}

	ctx := a.Context()
	w, h := max(a.width, 1), max(a.height, 1)

	var body string
	if a.overlays.Open() && a.fullOverlay {
		body = a.overlays.View(w, h)
	} else {
		header, footer := a.chrome()
		main := ""
		switch {
		case a.overlays.Open():
			main = a.overlays.View(w, ctx.MainHeight)
		case a.content != nil:
			main = a.content.View(ctx)
		}
		body = Frame(w, h, header, main, footer)
	}

	v := tea.NewView(body)
	v.AltScreen = a.altScreen
	title := a.windowTitle
	if a.titleFn != nil {
		if t := a.titleFn(ctx); t != "" {
			title = t
		}
	}
	if title != "" {
		v.WindowTitle = title
	}
	if a.mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// Run is a convenience for tea.NewProgram(a).Run().
func (a *App) Run() (tea.Model, error) {
	return tea.NewProgram(a).Run()
}
