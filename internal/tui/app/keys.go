package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

// Command is a named action the app can run from a keybinding or palette.
type Command struct {
	// ID is a stable command identifier (e.g. "app.quit").
	ID string
	// Title is a short human label for help / palettes.
	Title string
	// Run executes the command. May return a tea.Cmd.
	// The App pointer is the host shell.
	Run func(a *App) tea.Cmd
}

// Binding maps one or more key strings (as in tea.KeyPressMsg.String()) to a
// command ID.
type Binding struct {
	Keys    []string
	Command string
	Help    string
	// Global bindings are always checked, even when an overlay is open.
	Global bool
}

// KeyMap is a set of bindings.
type KeyMap struct {
	bindings []Binding
}

// NewKeyMap creates a keymap.
func NewKeyMap(bindings ...Binding) KeyMap {
	return KeyMap{bindings: append([]Binding{}, bindings...)}
}

// Bind appends bindings.
func (k *KeyMap) Bind(b ...Binding) {
	k.bindings = append(k.bindings, b...)
}

// DefaultQuitBindings returns ctrl+c → app.quit. Unlike plain dashboards, the
// shopware-cli TUIs contain type-to-filter inputs, so "q" stays a character.
func DefaultQuitBindings() []Binding {
	return []Binding{
		{Keys: []string{"ctrl+c"}, Command: CmdQuit, Help: "quit", Global: true},
	}
}

// KeyString normalizes a key press for matching. With Caps Lock on, terminals
// send "ctrl+C" instead of "ctrl+c"; lower-casing keeps shortcuts working.
func KeyString(msg tea.KeyPressMsg) string {
	return tui.KeyString(msg)
}

// Match returns the command ID for a key press, if any.
// When overlayOpen is true, only Global bindings match.
// Keys are compared case-insensitively (see KeyString).
func (k KeyMap) Match(key string, overlayOpen bool) (commandID string, ok bool) {
	key = strings.ToLower(key)
	for _, b := range k.bindings {
		if overlayOpen && !b.Global {
			continue
		}
		for _, kk := range b.Keys {
			if strings.ToLower(kk) == key {
				return b.Command, true
			}
		}
	}
	return "", false
}

// HelpLines returns key/help pairs for display.
func (k KeyMap) HelpLines() [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	for _, b := range k.bindings {
		if b.Help == "" || seen[b.Command] {
			continue
		}
		seen[b.Command] = true
		keys := strings.Join(b.Keys, "/")
		out = append(out, [2]string{keys, b.Help})
	}
	return out
}

// Built-in command IDs.
const (
	CmdQuit       = "app.quit"
	CmdPopOverlay = "app.overlay.pop"
)

// CommandRegistry holds runnable commands by ID.
type CommandRegistry struct {
	cmds map[string]Command
}

// NewCommandRegistry creates an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{cmds: map[string]Command{}}
}

// Register adds or replaces a command. Re-registering a built-in (e.g.
// CmdQuit) overrides its behavior — that is how screens intercept quit to
// cancel a running job instead of exiting.
func (r *CommandRegistry) Register(c Command) {
	if r.cmds == nil {
		r.cmds = map[string]Command{}
	}
	if c.ID == "" {
		return
	}
	r.cmds[c.ID] = c
}

// Get returns a command by ID.
func (r *CommandRegistry) Get(id string) (Command, bool) {
	if r == nil || r.cmds == nil {
		return Command{}, false
	}
	c, ok := r.cmds[id]
	return c, ok
}

// All returns registered commands.
func (r *CommandRegistry) All() []Command {
	if r == nil || len(r.cmds) == 0 {
		return nil
	}
	out := make([]Command, 0, len(r.cmds))
	for _, c := range r.cmds {
		out = append(out, c)
	}
	return out
}

// Emit returns a Cmd that delivers msg on the next Update. Overlays use it to
// hand results to the hosting Content while closing (return nil, Emit(result)).
func Emit(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
