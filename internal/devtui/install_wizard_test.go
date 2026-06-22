package devtui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func newTestInstallModel() Model {
	username := textinput.New()
	username.Placeholder = defaultUsername
	username.Prompt = "Username: "
	username.CharLimit = 50

	password := textinput.New()
	password.Placeholder = "shopware"
	password.Prompt = "Password: "
	password.CharLimit = 50
	password.EchoMode = textinput.EchoPassword

	return Model{
		phase: phaseInstallPrompt,
		install: installWizard{
			step:       installStepAsk,
			confirmYes: true,
			username:   username,
			password:   password,
		},
	}
}

func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func TestInstallStepAsk_LeftRightTogglesSelection(t *testing.T) {
	m := newTestInstallModel()
	m.install.confirmYes = true

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	assert.False(t, updated.(Model).install.confirmYes)

	updated, _ = updated.(Model).updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	assert.True(t, updated.(Model).install.confirmYes)
}

func TestInstallStepAsk_TabTogglesSelection(t *testing.T) {
	m := newTestInstallModel()
	m.install.confirmYes = true

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.False(t, updated.(Model).install.confirmYes)

	updated, _ = updated.(Model).updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.True(t, updated.(Model).install.confirmYes)
}

func TestInstallStepAsk_EnterYesAdvancesToLanguage(t *testing.T) {
	m := newTestInstallModel()
	m.install.confirmYes = true

	updated, cmd := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, installStepLanguage, mm.install.step)
	assert.Equal(t, 0, mm.install.cursor)
	assert.Equal(t, phaseInstallPrompt, mm.phase)
	assert.Nil(t, cmd)
}

func TestInstallStepAsk_QuitKey(t *testing.T) {
	m := newTestInstallModel()
	_, cmd := m.updateInstallPrompt(keyMsg('q'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestInstallStepLanguage_UpDownMovesCursor(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepLanguage
	m.install.cursor = 0

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, updated.(Model).install.cursor)

	updated, _ = updated.(Model).updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, updated.(Model).install.cursor)
}

func TestInstallStepLanguage_CursorClampedAtBounds(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepLanguage
	m.install.cursor = 0

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, updated.(Model).install.cursor, "up at 0 should stay at 0")

	m2 := newTestInstallModel()
	m2.install.step = installStepLanguage
	m2.install.cursor = len(installLanguages) - 1
	updated, _ = m2.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, len(installLanguages)-1, updated.(Model).install.cursor)
}

func TestInstallStepLanguage_EnterSelectsLanguageAdvancesToCurrency(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepLanguage
	m.install.cursor = 2 // de-DE

	updated, _ := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, "de-DE", mm.install.language)
	assert.Equal(t, installStepCurrency, mm.install.step)
	assert.Equal(t, 0, mm.install.cursor)
}

func TestInstallStepCurrency_UpDownAndEnter(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCurrency
	m.install.cursor = 0

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 1, updated.(Model).install.cursor)

	mm := updated.(Model)
	updated, cmd := mm.updateInstallPrompt(enterKey())
	out := updated.(Model)
	assert.Equal(t, "USD", out.install.currency)
	assert.Equal(t, installStepCredentials, out.install.step)
	assert.Equal(t, defaultUsername, out.install.username.Value())
	assert.Equal(t, "shopware", out.install.password.Value())
	assert.Equal(t, credFocusUsername, out.install.credFocus)
	assert.True(t, out.install.username.Focused())
	assert.NotNil(t, cmd)
}

func TestInstallStepCurrency_CursorClampedAtBounds(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCurrency
	m.install.cursor = len(installCurrencies) - 1

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, len(installCurrencies)-1, updated.(Model).install.cursor)
}

func TestInstallStepCredentials_EnterOnUsernameFocusesPassword(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusUsername
	m.install.username.SetValue("custom-admin")
	m.install.username.Focus()

	updated, cmd := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, installStepCredentials, mm.install.step)
	assert.Equal(t, credFocusPassword, mm.install.credFocus)
	assert.False(t, mm.install.username.Focused())
	assert.True(t, mm.install.password.Focused())
	assert.NotNil(t, cmd)
}

func TestInstallStepCredentials_TypedKeysGoToUsername(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusUsername
	m.install.username.SetValue("")
	m.install.username.Focus()

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	mm := updated.(Model)
	assert.Equal(t, installStepCredentials, mm.install.step)
	assert.Equal(t, "x", mm.install.username.Value())
}

func TestInstallStepCredentials_TabFromUsernameFocusesPassword(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusUsername

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	mm := updated.(Model)
	assert.Equal(t, credFocusPassword, mm.install.credFocus)
	assert.True(t, mm.install.password.Focused())
}

func TestInstallStepCredentials_TabFromPasswordFocusesCheckbox(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	m.install.password.Focus()

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	mm := updated.(Model)
	assert.Equal(t, credFocusShowPassword, mm.install.credFocus)
	assert.False(t, mm.install.password.Focused())
}

func TestInstallStepCredentials_DownFromPasswordFocusesCheckbox(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	m.install.password.Focus()

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, credFocusShowPassword, updated.(Model).install.credFocus)
}

func TestInstallStepCredentials_ShiftTabFromCheckboxFocusesPassword(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusShowPassword

	updated, cmd := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	mm := updated.(Model)
	assert.Equal(t, credFocusPassword, mm.install.credFocus)
	assert.True(t, mm.install.password.Focused())
	assert.NotNil(t, cmd)
}

func TestInstallStepCredentials_UpFromCheckboxFocusesPassword(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusShowPassword

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	mm := updated.(Model)
	assert.Equal(t, credFocusPassword, mm.install.credFocus)
	assert.True(t, mm.install.password.Focused())
}

func TestInstallStepCredentials_NavigationClampsAtBounds(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusUsername

	// Up/shift-tab at the first element should stay on username.
	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, credFocusUsername, updated.(Model).install.credFocus)

	// Tab past the checkbox should stay on the checkbox.
	m2 := newTestInstallModel()
	m2.install.step = installStepCredentials
	m2.install.credFocus = credFocusShowPassword
	updated, _ = m2.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, credFocusShowPassword, updated.(Model).install.credFocus)
}

func TestInstallStepCredentials_EnterOnCheckboxTogglesEcho(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusShowPassword
	m.install.password.EchoMode = textinput.EchoPassword

	updated, _ := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, textinput.EchoNormal, mm.install.password.EchoMode)
	assert.Equal(t, installStepCredentials, mm.install.step, "should stay on credentials step")

	updated, _ = mm.updateInstallPrompt(enterKey())
	assert.Equal(t, textinput.EchoPassword, updated.(Model).install.password.EchoMode)
}

func TestInstallStepCredentials_CheckboxFocusedSwallowsTypedKeys(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusShowPassword
	m.install.password.SetValue("orig")

	updated, cmd := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	mm := updated.(Model)
	assert.Equal(t, "orig", mm.install.password.Value(), "checkbox-focused state must not forward typing to input")
	assert.Nil(t, cmd)
}

func TestValidateAdminPassword(t *testing.T) {
	assert.NoError(t, validateAdminPassword("shopware"))
	assert.NoError(t, validateAdminPassword("12345678"))
	assert.Error(t, validateAdminPassword("shopwar"))
	assert.Error(t, validateAdminPassword(""))
	// Length is counted in runes, not bytes.
	assert.Error(t, validateAdminPassword("äöü"))
}

func TestInstallStepCredentials_EnterWithShortPasswordBlocks(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	m.install.password.SetValue("shopwar")
	m.install.password.Focus()

	updated, cmd := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, installStepCredentials, mm.install.step, "should stay on the credentials step")
	assert.Equal(t, phaseInstallPrompt, mm.phase, "should not start installing")
	assert.NotEmpty(t, mm.install.passwordErr, "should set a validation error")
	assert.Nil(t, cmd)
}

func TestInstallStepCredentials_EnterWithValidPasswordStartsInstall(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	m.install.password.SetValue("shopware")
	m.install.password.Focus()

	updated, cmd := m.updateInstallPrompt(enterKey())
	mm := updated.(Model)
	assert.Equal(t, phaseInstalling, mm.phase)
	assert.Empty(t, mm.install.passwordErr)
	assert.NotNil(t, cmd)
}

func TestInstallStepCredentials_TypingClearsError(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	m.install.passwordErr = "password must be at least 8 characters long"
	m.install.password.Focus()

	updated, _ := m.updateInstallPrompt(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	assert.Empty(t, updated.(Model).install.passwordErr)
}

func TestRenderInstallPrompt_PasswordErrorShown(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStepCredentials
	m.install.passwordErr = "password must be at least 8 characters long"

	var b strings.Builder
	m.renderInstallPrompt(&b)
	assert.Contains(t, b.String(), "password must be at least 8 characters long")
}

func TestRenderInstallPrompt_AllStepsDoNotPanic(t *testing.T) {
	m := newTestInstallModel()

	steps := []struct {
		step    installStep
		expects []string
	}{
		{installStepAsk, []string{"Shopware is not initialized yet", "Initialize now"}},
		{installStepLanguage, []string{"Step 1/3", "Default Language"}},
		{installStepCurrency, []string{"Step 2/3", "Default Currency"}},
		{installStepCredentials, []string{"Step 3/3", "Admin Account", "Choose a username", "Choose a password"}},
	}

	for _, s := range steps {
		m.install.step = s.step
		var b strings.Builder
		assert.NotPanics(t, func() {
			m.renderInstallPrompt(&b)
		})
		out := b.String()
		for _, want := range s.expects {
			assert.Contains(t, out, want, "step %d view should contain %q", s.step, want)
		}
	}
}

func TestInstallFooterHint_PerStep(t *testing.T) {
	m := newTestInstallModel()

	m.install.step = installStepAsk
	assert.Contains(t, m.installFooterHint(), "Confirm")

	m.install.step = installStepLanguage
	assert.Contains(t, m.installFooterHint(), "Select")

	m.install.step = installStepCurrency
	assert.Contains(t, m.installFooterHint(), "Select")

	m.install.step = installStepCredentials
	m.install.credFocus = credFocusPassword
	assert.Contains(t, m.installFooterHint(), "Install")
	assert.Contains(t, m.installFooterHint(), "Navigate")

	m.install.credFocus = credFocusShowPassword
	assert.Contains(t, m.installFooterHint(), "Toggle")
	assert.Contains(t, m.installFooterHint(), "Navigate")
}

func TestInstallFooterHint_UnknownStepReturnsEmpty(t *testing.T) {
	m := newTestInstallModel()
	m.install.step = installStep(999)
	assert.Empty(t, m.installFooterHint())
}

func TestInstallStepPatterns_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, installStepPatterns)
	for _, sp := range installStepPatterns {
		assert.NotEmpty(t, sp.pattern)
		assert.NotEmpty(t, sp.label)
	}
}
