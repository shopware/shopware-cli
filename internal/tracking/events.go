package tracking

// Event names passed to Track. The "shopware_cli." prefix is added by Track
// itself. Every event and its tags are documented in docs/TELEMETRY.md; new
// events must be added there as well.
const (
	// EventCommand is sent after (almost) any sub-command finishes (cmd/root.go).
	EventCommand = "command"
	// EventProjectCreate is sent when a new Shopware project is scaffolded.
	EventProjectCreate = "project.create"
	// EventProjectUpgradeCheck is sent when an upgrade compatibility check runs.
	EventProjectUpgradeCheck = "project.upgrade_check"
	// EventProjectUpgrade is sent when the interactive upgrade wizard finishes
	// an upgrade run (internal/upgradetui).
	EventProjectUpgrade = "project.upgrade"

	// The project.dev.* events are sent by the interactive dev TUI (internal/devtui).
	EventDevSession         = "project.dev.session"
	EventDevInstall         = "project.dev.install"
	EventDevMigrationWizard = "project.dev.migration_wizard"
	EventDevDockerStart     = "project.dev.docker_start"
	EventDevAction          = "project.dev.action"
	EventDevWatcher         = "project.dev.watcher"
	EventDevHealth          = "project.dev.health"
)

// Tag keys used by the events above. Keys are shared across events wherever
// the semantic matches (TagResult, TagDurationMS) — the ClickHouse events
// table materializes columns from these shared keys, so a new event reusing
// them is aggregatable without schema changes.
const (
	// EventCommand
	TagCommandName = "command_name"
	TagResult      = "result"
	TagDurationMS  = "duration_ms"
	TagCLIVersion  = "cli_version"
	TagOS          = "os"
	TagIsTUI       = "is_tui"

	// EventProjectCreate
	TagVersion           = "version"
	TagDeployment        = "deployment"
	TagCI                = "ci"
	TagDocker            = "docker"
	TagWithElasticsearch = "with_elasticsearch"
	TagWithAMQP          = "with_amqp"
	TagInteractive       = "interactive"

	// EventProjectUpgradeCheck
	TagFromVersion   = "from_version"
	TagTargetVersion = "target_version"
	TagHasBlockers   = "has_blockers"

	// EventDevInstall
	TagAbandonedAt       = "abandoned_at"
	TagFailedStep        = "failed_step"
	TagLanguage          = "language"
	TagCurrency          = "currency"
	TagCustomCredentials = "custom_credentials"

	// EventDevMigrationWizard
	TagPHPVersion            = "php_version"
	TagDeploymentHelperAdded = "deployment_helper_added"

	// EventDevDockerStart
	TagTrigger = "trigger"

	// EventDevAction
	TagAction = "action"

	// EventDevWatcher
	TagWatcher = "watcher"

	// EventDevHealth
	TagCheck = "check"

	// EventDevSession
	TagExecutor     = "executor"
	TagTabsVisited  = "tabs_visited"
	TagActions      = "actions"
	TagWatchersUsed = "watchers_used"
)

// Values of the TagResult tag shared across events.
const (
	ResultSuccess   = "success"
	ResultFailure   = "failure"
	ResultCancelled = "cancelled"
	ResultSkipped   = "skipped"
	ResultCompleted = "completed"
	ResultFailed    = "failed"
)
