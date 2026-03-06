package devtui

import (
	"context"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

type mockExecutor struct {
	execType string
}

func (m *mockExecutor) ConsoleCommand(_ context.Context, _ ...string) *exec.Cmd  { return nil }
func (m *mockExecutor) ComposerCommand(_ context.Context, _ ...string) *exec.Cmd { return nil }
func (m *mockExecutor) PHPCommand(_ context.Context, _ ...string) *exec.Cmd      { return nil }
func (m *mockExecutor) Type() string                                             { return m.execType }

func TestNew(t *testing.T) {
	cfg := &shop.Config{
		URL: "http://localhost:8000",
		AdminApi: &shop.ConfigAdminApi{
			Username: "admin",
			Password: "shopware",
		},
	}
	envCfg := &shop.EnvironmentConfig{
		Type: "docker",
	}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})

	assert.Equal(t, tabGeneral, m.activeTab)
	assert.Equal(t, "docker", m.general.envType)
	assert.Equal(t, "http://localhost:8000", m.general.shopURL)
	assert.Equal(t, "admin", m.general.username)
	assert.True(t, m.dockerMode)
}

func TestNew_EnvAdminApiOverride(t *testing.T) {
	cfg := &shop.Config{
		URL: "http://localhost:8000",
	}
	envCfg := &shop.EnvironmentConfig{
		Type: "docker",
		URL:  "http://docker-host:8000",
		AdminApi: &shop.ConfigAdminApi{
			Username: "env-admin",
			Password: "env-pass",
		},
	}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})

	assert.Equal(t, "env-admin", m.general.username)
	assert.Equal(t, "env-pass", m.general.password)
	assert.Equal(t, "http://docker-host:8000", m.general.shopURL)
}

func TestNew_LocalMode(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "local"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "local"},
	})

	assert.False(t, m.dockerMode)
}

func TestTabSwitching(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "local"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "local"},
	})

	// Switch to tab 2
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))
	model := result.(Model)
	assert.Equal(t, tabLogs, model.activeTab)

	// Switch to tab 1
	result, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: '1', Text: "1"}))
	model = result.(Model)
	assert.Equal(t, tabGeneral, model.activeTab)
}

func TestTabSwitchingBlockedDuringOverlay(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "local"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "local"},
	})
	m.overlay = overlayStarting

	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))
	model := result.(Model)
	assert.Equal(t, tabGeneral, model.activeTab)
}

func TestWindowSizeMsg(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "local"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "local"},
	})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	assert.Equal(t, 120, model.width)
	assert.Equal(t, 40, model.height)
}

func TestDockerAlreadyRunning(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})

	result, cmd := m.Update(dockerAlreadyRunningMsg{})
	model := result.(Model)
	assert.Equal(t, overlayNone, model.overlay)
	assert.NotNil(t, cmd)
}

func TestDockerStartedSuccess(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayStarting

	result, cmd := m.Update(dockerStartedMsg{})
	model := result.(Model)
	assert.Equal(t, overlayNone, model.overlay)
	assert.Nil(t, model.overlayLines)
	assert.NotNil(t, cmd)
}

func TestDockerStartedFailure(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayStarting

	result, cmd := m.Update(dockerStartedMsg{err: assert.AnError})
	model := result.(Model)
	assert.Equal(t, overlayStarting, model.overlay)
	assert.NotEmpty(t, model.overlayLines)
	assert.Nil(t, cmd)
}

func TestOverlayRendering(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayStarting
	m.overlayLines = []string{"Pulling images..."}

	view := m.View()
	assert.Contains(t, view.Content, "Starting Docker containers...")
	assert.Contains(t, view.Content, "Pulling images...")
}

func TestShopwareNotInstalled_ShowsPrompt(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})

	result, cmd := m.Update(shopwareNotInstalledMsg{})
	model := result.(Model)
	assert.Equal(t, overlayInstallPrompt, model.overlay)
	assert.Equal(t, installStepAsk, model.install.step)
	assert.Nil(t, cmd)
}

func TestShopwareInstalled_StartsDashboard(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})

	result, cmd := m.Update(shopwareInstalledMsg{})
	model := result.(Model)
	assert.Equal(t, overlayNone, model.overlay)
	assert.NotNil(t, cmd)
}

func TestInstallPrompt_DeclineSkipsToDashboard(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepAsk}

	result, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	model := result.(Model)
	assert.Equal(t, overlayNone, model.overlay)
	assert.NotNil(t, cmd)
}

func TestInstallPromptRendering(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepAsk}

	view := m.View()
	assert.Contains(t, view.Content, "Shopware is not installed")
	assert.Contains(t, view.Content, "y: install")
	assert.Contains(t, view.Content, "n: skip")
}

func TestInstallWizard_AcceptGoesToLanguage(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepAsk}

	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	model := result.(Model)
	assert.Equal(t, overlayInstallPrompt, model.overlay)
	assert.Equal(t, installStepLanguage, model.install.step)
	assert.Equal(t, 0, model.install.cursor)
}

func TestInstallWizard_LanguageSelection(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepLanguage, cursor: 0}

	// Move down
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown, Text: ""}))
	model := result.(Model)
	assert.Equal(t, 1, model.install.cursor)

	// Confirm en-US (index 1)
	result, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: ""}))
	model = result.(Model)
	assert.Equal(t, installStepCurrency, model.install.step)
	assert.Equal(t, "en-US", model.install.language)
	assert.Equal(t, 0, model.install.cursor)
}

func TestInstallWizard_LanguageRendering(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepLanguage, cursor: 0}

	view := m.View()
	assert.Contains(t, view.Content, "Select default language")
	assert.Contains(t, view.Content, "> English (UK)")
	assert.Contains(t, view.Content, "  Deutsch")
}

func TestInstallWizard_CurrencyRendering(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepCurrency, language: "en-GB", cursor: 1}

	view := m.View()
	assert.Contains(t, view.Content, "Select default currency")
	assert.Contains(t, view.Content, "  EUR")
	assert.Contains(t, view.Content, "> USD")
	assert.Contains(t, view.Content, "  GBP")
}

func TestInstallWizard_CursorBounds(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstallPrompt
	m.install = installWizard{step: installStepLanguage, cursor: 0}

	// Try to go above 0
	result, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp, Text: ""}))
	model := result.(Model)
	assert.Equal(t, 0, model.install.cursor)

	// Go to last item and try to go past
	model.install.cursor = len(installLanguages) - 1
	result, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown, Text: ""}))
	model = result.(Model)
	assert.Equal(t, len(installLanguages)-1, model.install.cursor)
}

func TestInstallDoneSuccess(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstalling

	result, cmd := m.Update(shopwareInstallDoneMsg{})
	model := result.(Model)
	assert.Equal(t, overlayNone, model.overlay)
	assert.NotNil(t, cmd)
}

func TestInstallDoneFailure(t *testing.T) {
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg := &shop.EnvironmentConfig{Type: "docker"}

	m := New(Options{
		ProjectRoot: "/tmp/project",
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    &mockExecutor{execType: "docker"},
	})
	m.overlay = overlayInstalling

	result, cmd := m.Update(shopwareInstallDoneMsg{err: assert.AnError})
	model := result.(Model)
	assert.Equal(t, overlayInstalling, model.overlay)
	assert.NotEmpty(t, model.overlayLines)
	assert.Nil(t, cmd)
}
