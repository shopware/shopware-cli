package devtui

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMain disables telemetry for the whole package: tests drive Model.Update
// through real tracking call sites, and no test run should emit usage events.
func TestMain(m *testing.M) {
	os.Setenv("DO_NOT_TRACK", "1")
	os.Exit(m.Run())
}

func TestInstallTagsOnlyIncludesMadeChoices(t *testing.T) {
	tel := newTelemetryState(true)

	w := installWizard{credentialStep: newInstallCredentialStep(), step: installStepLanguage}
	tags := tel.installTags("cancelled", w)

	assert.Equal(t, "cancelled", tags["result"])
	assert.NotContains(t, tags, "language")
	assert.NotContains(t, tags, "currency")
	assert.NotContains(t, tags, "custom_credentials")
	assert.NotContains(t, tags, "duration_ms")
}

func TestInstallTagsNeverContainCredentialValues(t *testing.T) {
	tel := newTelemetryState(true)
	tel.beginInstall()

	w := installWizard{credentialStep: newInstallCredentialStep(), step: installStepCredentials, language: "de-DE", currency: "EUR"}
	w.username.SetValue("hidden-admin-name")
	w.password.SetValue("super-secret-password")
	tags := tel.installTags("success", w)

	assert.Equal(t, "de-DE", tags["language"])
	assert.Equal(t, "EUR", tags["currency"])
	assert.Equal(t, "true", tags["custom_credentials"])
	assert.Contains(t, tags, "duration_ms")
	for _, v := range tags {
		assert.NotContains(t, v, "hidden-admin-name")
		assert.NotContains(t, v, "super-secret-password")
	}
}

func TestInstallTagsDefaultCredentials(t *testing.T) {
	w := installWizard{credentialStep: newInstallCredentialStep(), step: installStepCredentials}
	w.username.SetValue("admin")
	w.password.SetValue("shopware")

	tags := (*telemetryState)(nil).installTags("failure", w)
	assert.Equal(t, "false", tags["custom_credentials"])
}

func TestInstallFailedStepClampsToLastPattern(t *testing.T) {
	assert.Equal(t, "system:install", installFailedStep(0))
	assert.Equal(t, "plugin:refresh", installFailedStep(len(installStepPatterns)+5))
}

func TestMigrationWizardTagsCancelledOnWelcome(t *testing.T) {
	sg := migrationWizard{step: migrationStepWelcome, phpVersions: []string{"8.2"}}
	tags := migrationWizardTags("cancelled", sg)

	assert.Equal(t, "cancelled", tags["result"])
	assert.Equal(t, "welcome", tags["abandoned_at"])
	assert.NotContains(t, tags, "duration_ms")
	assert.NotContains(t, tags, "php_version")
}

func TestMigrationWizardTagsCompleted(t *testing.T) {
	sg := migrationWizard{
		step:                  migrationStepDone,
		phpVersions:           []string{"8.2", "8.3"},
		phpCursor:             1,
		startedAt:             time.Now().Add(-2 * time.Second),
		deploymentHelperAdded: true,
	}
	tags := migrationWizardTags("completed", sg)

	assert.Equal(t, "completed", tags["result"])
	assert.Equal(t, "8.3", tags["php_version"])
	assert.Equal(t, "true", tags["deployment_helper_added"])
	assert.Contains(t, tags, "duration_ms")
	assert.NotContains(t, tags, "abandoned_at")
}

func TestWatcherEndTagsFireOncePerRun(t *testing.T) {
	tel := newTelemetryState(true)
	tel.watcherStarted(watcherAdmin)

	tags, ok := tel.watcherEndTags(watcherAdmin, "user_stopped")
	assert.True(t, ok)
	assert.Equal(t, "admin", tags["watcher"])
	assert.Equal(t, "user_stopped", tags["result"])
	assert.Contains(t, tags, "uptime_ms")

	// A trailing logDoneMsg for the same run must not produce a second event.
	_, ok = tel.watcherEndTags(watcherAdmin, "crashed")
	assert.False(t, ok)
}

func TestWatcherEndTagsWithoutStart(t *testing.T) {
	tel := newTelemetryState(false)
	_, ok := tel.watcherEndTags(watcherStorefront, "crashed")
	assert.False(t, ok)

	var nilTel *telemetryState
	_, ok = nilTel.watcherEndTags(watcherAdmin, "crashed")
	assert.False(t, ok)
}

func TestSessionTagsSentOnce(t *testing.T) {
	tel := newTelemetryState(true)
	tel.markTab(tabConfig)
	tel.markTab(tabInstance)
	tel.countAction()
	tel.watcherStarted(watcherStorefront)
	tel.setExitChoice("keep_running")

	tags, ok := tel.sessionTags()
	assert.True(t, ok)
	assert.Equal(t, "docker", tags["executor"])
	assert.Equal(t, "config,instance,overview", tags["tabs_visited"])
	assert.Equal(t, "1", tags["actions"])
	assert.Equal(t, "storefront", tags["watchers_used"])
	assert.Equal(t, "keep_running", tags["exit"])

	_, ok = tel.sessionTags()
	assert.False(t, ok)
}

func TestSessionTagsDefaults(t *testing.T) {
	tel := newTelemetryState(false)
	tags, ok := tel.sessionTags()
	assert.True(t, ok)
	assert.Equal(t, "local", tags["executor"])
	assert.Equal(t, "overview", tags["tabs_visited"])
	assert.Equal(t, "quit", tags["exit"])
	assert.NotContains(t, tags, "watchers_used")
}

func TestTaskTagsRoundTrip(t *testing.T) {
	tel := newTelemetryState(true)

	_, ok := tel.taskTags("success")
	assert.False(t, ok, "no task began")

	tel.beginTask("cache-clear")
	tags, ok := tel.taskTags("failure")
	assert.True(t, ok)
	assert.Equal(t, "cache-clear", tags["action"])
	assert.Equal(t, "failure", tags["result"])
	assert.Contains(t, tags, "duration_ms")

	_, ok = tel.taskTags("success")
	assert.False(t, ok, "task already reported")
}

func TestDockerStartTagsRequireBegin(t *testing.T) {
	tel := newTelemetryState(true)
	_, ok := tel.dockerStartTags(nil)
	assert.False(t, ok)

	tel.beginDockerStart()
	tags, ok := tel.dockerStartTags(assert.AnError)
	assert.True(t, ok)
	assert.Equal(t, "initial", tags["trigger"])
	assert.Equal(t, "failure", tags["result"])

	tel.beginConfigRestart()
	tags, ok = tel.configRestartTags(nil)
	assert.True(t, ok)
	assert.Equal(t, "config_change", tags["trigger"])
	assert.Equal(t, "success", tags["result"])
}

func TestHealthTagsFlattenChecks(t *testing.T) {
	checks := []healthCheck{
		{Name: "PHP version", Level: healthCritical},
		{Name: "Memory limit", Level: healthOK},
		{Name: "Admin Worker", Level: healthWarn},
		{Name: "Flow Builder log level", Level: healthWarn},
	}

	tags := healthTags(checks)
	assert.Equal(t, map[string]string{
		"php_version":            "critical",
		"memory_limit":           "ok",
		"admin_worker":           "warn",
		"flow_builder_log_level": "warn",
	}, tags)
}

func TestInstallOnceLatches(t *testing.T) {
	tel := newTelemetryState(true)
	assert.True(t, tel.installOnce())
	// Quitting the failure screen must not report the install a second time.
	assert.False(t, tel.installOnce())
	assert.False(t, (*telemetryState)(nil).installOnce())
}

func TestHealthOnceLatches(t *testing.T) {
	tel := newTelemetryState(true)
	assert.True(t, tel.healthOnce())
	assert.False(t, tel.healthOnce())
	assert.False(t, (*telemetryState)(nil).healthOnce())
}

func TestNilTelemetryStateIsSafe(t *testing.T) {
	var tel *telemetryState
	tel.markTab(tabConfig)
	tel.countAction()
	tel.setExitChoice("quit")
	tel.beginInstall()
	tel.beginDockerStart()
	tel.beginConfigRestart()
	tel.beginTask("cache-clear")
	tel.watcherStarted(watcherAdmin)

	_, ok := tel.sessionTags()
	assert.False(t, ok)
	_, ok = tel.taskTags("success")
	assert.False(t, ok)
	_, ok = tel.dockerStartTags(nil)
	assert.False(t, ok)
	_, ok = tel.configRestartTags(nil)
	assert.False(t, ok)
}
