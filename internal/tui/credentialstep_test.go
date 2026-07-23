package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCreds() CredentialStep {
	return NewCredentialStep(CredentialStepOptions{
		Username: "admin",
		Password: "shopware",
		ValidatePassword: func(p string) error {
			if len(p) < 8 {
				return errors.New("password must be at least 8 characters long")
			}
			return nil
		},
	})
}

func credKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func TestCredentialStep_FocusMovesAndClamps(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusUsername)

	_, submitted := c.HandleKey(credKey(tea.KeyTab))
	assert.False(t, submitted)
	assert.Equal(t, CredFocusPassword, c.FocusTarget())

	c.HandleKey(credKey(tea.KeyTab))
	assert.Equal(t, CredFocusShowPassword, c.FocusTarget())

	// Tab past the checkbox stays on the checkbox.
	c.HandleKey(credKey(tea.KeyTab))
	assert.Equal(t, CredFocusShowPassword, c.FocusTarget())

	c.HandleKey(credKey(tea.KeyUp))
	assert.Equal(t, CredFocusPassword, c.FocusTarget())

	// Up past the username stays on the username.
	c.HandleKey(credKey(tea.KeyUp))
	c.HandleKey(credKey(tea.KeyUp))
	assert.Equal(t, CredFocusUsername, c.FocusTarget())
}

func TestCredentialStep_EnterFlow(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusUsername)

	// Enter on the username advances to the password.
	_, submitted := c.HandleKey(credKey(tea.KeyEnter))
	assert.False(t, submitted)
	assert.Equal(t, CredFocusPassword, c.FocusTarget())

	// Enter on the password with a valid value submits and blurs.
	_, submitted = c.HandleKey(credKey(tea.KeyEnter))
	assert.True(t, submitted)
	assert.Empty(t, c.PasswordErr())
}

func TestCredentialStep_ShortPasswordBlocksSubmit(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusPassword)
	c.SetPassword("short")

	_, submitted := c.HandleKey(credKey(tea.KeyEnter))
	assert.False(t, submitted)
	assert.NotEmpty(t, c.PasswordErr())

	// Typing into the password clears the error.
	c.HandleKey(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	assert.Empty(t, c.PasswordErr())
}

func TestCredentialStep_CheckboxTogglesEcho(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusShowPassword)
	require.True(t, c.PasswordMasked(), "password starts masked")

	_, submitted := c.HandleKey(credKey(tea.KeyEnter))
	assert.False(t, submitted)
	assert.False(t, c.PasswordMasked())

	c.HandleKey(credKey(tea.KeyEnter))
	assert.True(t, c.PasswordMasked())

	// Typed keys are swallowed while the checkbox has focus.
	c.HandleKey(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	assert.Equal(t, "shopware", c.Password())
}

func TestCredentialStep_TypingReachesFocusedInput(t *testing.T) {
	c := NewCredentialStep(CredentialStepOptions{})
	c.Focus(CredFocusUsername)
	c.HandleKey(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	assert.Equal(t, "a", c.Username())
}

func TestCredentialStep_NilValidatorAcceptsAnything(t *testing.T) {
	c := NewCredentialStep(CredentialStepOptions{})
	c.Focus(CredFocusPassword)
	_, submitted := c.HandleKey(credKey(tea.KeyEnter))
	assert.True(t, submitted)
}

func TestCredentialStep_Render(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusPassword)
	c.SetPassword("short")
	c.HandleKey(credKey(tea.KeyEnter))

	var b strings.Builder
	c.Render(&b)
	out := ansi.Strip(b.String())
	assert.Contains(t, out, "Choose a username")
	assert.Contains(t, out, "Choose a password")
	assert.Contains(t, out, "password must be at least 8 characters long")
	assert.Contains(t, out, "[ ] Show password")
	assert.NotContains(t, out, "short", "masked password must not leak into the view")
}

func TestCredentialStep_FooterHint(t *testing.T) {
	c := testCreds()
	c.Focus(CredFocusPassword)
	assert.Contains(t, ansi.Strip(c.FooterHint("Install")), "Install")

	c.Focus(CredFocusShowPassword)
	assert.Contains(t, ansi.Strip(c.FooterHint("Install")), "Toggle")
}
