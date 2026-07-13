package devtui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

// credFocus identifies which element of a combined admin-account step
// (username field, password field, or the show-password checkbox) currently
// has focus. It is shared by the install wizard and the migration wizard.
type credFocus int

const (
	credFocusUsername credFocus = iota
	credFocusPassword
	credFocusShowPassword
)

// credentialStep holds the admin username/password entry shared by the install
// wizard and the migration wizard. Both embed it anonymously so its fields are
// promoted (e.g. wizard.username, wizard.credFocus) and its behaviour is shared.
type credentialStep struct {
	username    textinput.Model
	password    textinput.Model
	credFocus   credFocus
	passwordErr string
}

// newCredentialStep builds the username/password inputs pre-filled with the
// given defaults. The password starts masked.
func newCredentialStep(defaultUser, defaultPassword string) credentialStep {
	username := textinput.New()
	username.Placeholder = defaultUser
	username.CharLimit = 64
	username.Prompt = ""
	username.SetValue(defaultUser)

	password := textinput.New()
	password.Placeholder = defaultPassword
	password.CharLimit = 64
	password.Prompt = ""
	password.EchoMode = textinput.EchoPassword
	password.SetValue(defaultPassword)

	return credentialStep{username: username, password: password}
}

// focus moves focus to the given element, clamping to the valid range and
// syncing the text input focus. Returns the blink command for the newly
// focused input (nil for the checkbox).
func (c *credentialStep) focus(target credFocus) tea.Cmd {
	if target < credFocusUsername {
		target = credFocusUsername
	}
	if target > credFocusShowPassword {
		target = credFocusShowPassword
	}
	c.credFocus = target
	c.username.Blur()
	c.password.Blur()
	switch target {
	case credFocusUsername:
		c.username.Focus()
		return textinput.Blink
	case credFocusPassword:
		c.password.Focus()
		return textinput.Blink
	case credFocusShowPassword:
		// The checkbox has no text input to focus.
	}
	return nil
}

// updateInput routes a typed key to the focused input. Tab/shift-tab/up/down
// navigation and Enter are handled by the embedding wizard before this is
// called; updateInput only sees plain typing.
func (c *credentialStep) updateInput(msg tea.KeyPressMsg) tea.Cmd {
	switch c.credFocus {
	case credFocusUsername:
		var cmd tea.Cmd
		c.username, cmd = c.username.Update(msg)
		return cmd
	case credFocusPassword:
		c.passwordErr = ""
		var cmd tea.Cmd
		c.password, cmd = c.password.Update(msg)
		return cmd
	case credFocusShowPassword:
		// The checkbox swallows typed keys.
	}
	return nil
}

// toggleShowPassword flips the password between masked and plain text.
func (c *credentialStep) toggleShowPassword() {
	if c.password.EchoMode == textinput.EchoPassword {
		c.password.EchoMode = textinput.EchoNormal
	} else {
		c.password.EchoMode = textinput.EchoPassword
	}
}

// validatePassword runs the admin password length check, storing the message in
// passwordErr on failure. Returns true when the password is acceptable.
func (c *credentialStep) validatePassword() bool {
	if err := validateAdminPassword(c.password.Value()); err != nil {
		c.passwordErr = err.Error()
		return false
	}
	c.passwordErr = ""
	return true
}

// blur removes focus from both inputs, used when leaving the step.
func (c *credentialStep) blur() {
	c.username.Blur()
	c.password.Blur()
}

// render writes the username/password/show-password block into b. The caller is
// responsible for the surrounding step heading and footer.
func (c credentialStep) render(b *strings.Builder) {
	b.WriteString(valueStyle.Render("Choose a username"))
	b.WriteString("\n")
	b.WriteString(c.username.View())
	b.WriteString("\n\n")
	b.WriteString(valueStyle.Render("Choose a password"))
	b.WriteString(tui.DimStyle.Render("  at least 8 characters"))
	b.WriteString("\n")
	b.WriteString(c.password.View())
	if c.passwordErr != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(c.passwordErr))
	}
	b.WriteString("\n\n")
	b.WriteString(renderShowPasswordCheckbox(c.password.EchoMode == textinput.EchoNormal, c.credFocus == credFocusShowPassword))
}
