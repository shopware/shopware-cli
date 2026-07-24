package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// CredFocus identifies which element of a credential step (username field,
// password field, or the show-password checkbox) currently has focus.
type CredFocus int

const (
	CredFocusUsername CredFocus = iota
	CredFocusPassword
	CredFocusShowPassword
)

// CredentialStepOptions configure NewCredentialStep.
type CredentialStepOptions struct {
	// Username and Password pre-fill the inputs.
	Username string
	Password string
	// Placeholders shown when the inputs are empty.
	UsernamePlaceholder string
	PasswordPlaceholder string
	// Prompts rendered in front of the inputs (default none).
	UsernamePrompt string
	PasswordPrompt string
	// CharLimit bounds both inputs (default 64).
	CharLimit int
	// ValidatePassword rejects a password on submit; nil accepts anything.
	// The error text is rendered under the password field.
	ValidatePassword func(password string) error
}

// CredentialStep is a username/password/show-password fieldset for wizard
// steps. Embed it in a wizard and route the step's keys through HandleKey;
// the wizard decides what a submit means.
type CredentialStep struct {
	username    textinput.Model
	password    textinput.Model
	focus       CredFocus
	passwordErr string
	validate    func(string) error
}

// NewCredentialStep builds the username/password inputs. The password starts
// masked.
func NewCredentialStep(opts CredentialStepOptions) CredentialStep {
	if opts.CharLimit <= 0 {
		opts.CharLimit = 64
	}

	username := textinput.New()
	username.Placeholder = opts.UsernamePlaceholder
	username.CharLimit = opts.CharLimit
	username.Prompt = opts.UsernamePrompt
	if opts.Username != "" {
		username.SetValue(opts.Username)
	}

	password := textinput.New()
	password.Placeholder = opts.PasswordPlaceholder
	password.CharLimit = opts.CharLimit
	password.Prompt = opts.PasswordPrompt
	password.EchoMode = textinput.EchoPassword
	if opts.Password != "" {
		password.SetValue(opts.Password)
	}

	return CredentialStep{username: username, password: password, validate: opts.ValidatePassword}
}

// Username returns the current username value.
func (c CredentialStep) Username() string { return c.username.Value() }

// Password returns the current password value.
func (c CredentialStep) Password() string { return c.password.Value() }

// SetUsername replaces the username value.
func (c *CredentialStep) SetUsername(v string) { c.username.SetValue(v) }

// SetPassword replaces the password value.
func (c *CredentialStep) SetPassword(v string) { c.password.SetValue(v) }

// FocusTarget reports which element currently has focus.
func (c CredentialStep) FocusTarget() CredFocus { return c.focus }

// PasswordErr returns the current validation message, empty when valid.
func (c CredentialStep) PasswordErr() string { return c.passwordErr }

// PasswordMasked reports whether the password renders masked.
func (c CredentialStep) PasswordMasked() bool {
	return c.password.EchoMode == textinput.EchoPassword
}

// Focus moves focus to the given element, clamping to the valid range and
// syncing the text input focus. Returns the blink command for the newly
// focused input (nil for the checkbox).
func (c *CredentialStep) Focus(target CredFocus) tea.Cmd {
	if target < CredFocusUsername {
		target = CredFocusUsername
	}
	if target > CredFocusShowPassword {
		target = CredFocusShowPassword
	}
	c.focus = target
	c.username.Blur()
	c.password.Blur()
	switch target {
	case CredFocusUsername:
		c.username.Focus()
		return textinput.Blink
	case CredFocusPassword:
		c.password.Focus()
		return textinput.Blink
	case CredFocusShowPassword:
		// The checkbox has no text input to focus.
	}
	return nil
}

// HandleKey applies the shared credential-step key handling: tab/down and
// shift+tab/up move focus, enter advances from the username, toggles the
// checkbox, or submits from the password (after validation), and any other
// key types into the focused input. It reports submitted=true when the
// password passed validation and the step is complete — what a submit means
// is the embedding wizard's decision.
func (c *CredentialStep) HandleKey(msg tea.KeyPressMsg) (cmd tea.Cmd, submitted bool) {
	switch KeyString(msg) {
	case KeyEnter:
		switch c.focus {
		case CredFocusUsername:
			return c.Focus(CredFocusPassword), false
		case CredFocusShowPassword:
			c.ToggleShowPassword()
			return nil, false
		case CredFocusPassword:
			// Enter on the password submits; handled below.
		}
		if !c.ValidatePassword() {
			return nil, false
		}
		c.Blur()
		return nil, true
	case KeyTab, KeyDown:
		return c.Focus(c.focus + 1), false
	case KeyShiftTab, KeyUp:
		return c.Focus(c.focus - 1), false
	}
	return c.updateInput(msg), false
}

// updateInput routes a typed key to the focused input.
func (c *CredentialStep) updateInput(msg tea.KeyPressMsg) tea.Cmd {
	switch c.focus {
	case CredFocusUsername:
		var cmd tea.Cmd
		c.username, cmd = c.username.Update(msg)
		return cmd
	case CredFocusPassword:
		c.passwordErr = ""
		var cmd tea.Cmd
		c.password, cmd = c.password.Update(msg)
		return cmd
	case CredFocusShowPassword:
		// The checkbox swallows typed keys.
	}
	return nil
}

// ToggleShowPassword flips the password between masked and plain text.
func (c *CredentialStep) ToggleShowPassword() {
	if c.password.EchoMode == textinput.EchoPassword {
		c.password.EchoMode = textinput.EchoNormal
	} else {
		c.password.EchoMode = textinput.EchoPassword
	}
}

// ValidatePassword runs the configured password check, storing the message in
// PasswordErr on failure. Returns true when the password is acceptable.
func (c *CredentialStep) ValidatePassword() bool {
	if c.validate == nil {
		c.passwordErr = ""
		return true
	}
	if err := c.validate(c.password.Value()); err != nil {
		c.passwordErr = err.Error()
		return false
	}
	c.passwordErr = ""
	return true
}

// Blur removes focus from both inputs, used when leaving the step.
func (c *CredentialStep) Blur() {
	c.username.Blur()
	c.password.Blur()
}

// Render writes the username/password/show-password block into b. The caller
// is responsible for the surrounding step heading and footer.
func (c CredentialStep) Render(b *strings.Builder) {
	labelStyle := lipgloss.NewStyle().Foreground(TextColor)
	errStyle := lipgloss.NewStyle().Foreground(ErrorColor)

	b.WriteString(labelStyle.Render("Choose a username"))
	b.WriteString("\n")
	b.WriteString(c.username.View())
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Choose a password"))
	b.WriteString(DimStyle.Render("  at least 8 characters"))
	b.WriteString("\n")
	b.WriteString(c.password.View())
	if c.passwordErr != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render(c.passwordErr))
	}
	b.WriteString("\n\n")
	b.WriteString(Checkbox(!c.PasswordMasked(), c.focus == CredFocusShowPassword, "Show password"))
}

// FooterHint returns the shortcut bar for the step: navigation plus what
// enter does for the current focus (toggle on the checkbox, submitLabel
// otherwise).
func (c CredentialStep) FooterHint(submitLabel string) string {
	if c.focus == CredFocusShowPassword {
		return ShortcutBar(
			Shortcut{Key: "↑/↓/tab", Label: "Navigate"},
			Shortcut{Key: "enter", Label: "Toggle"},
		)
	}
	return ShortcutBar(
		Shortcut{Key: "↑/↓/tab", Label: "Navigate"},
		Shortcut{Key: "enter", Label: submitLabel},
	)
}
