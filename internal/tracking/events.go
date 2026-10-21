package tracking

// Event names passed to Track. The "shopware_cli." prefix is added by Track
// itself. Every event and its tags are documented in docs/TELEMETRY.md; new
// events must be added there as well.
const (
	// EventCommand is sent after (almost) any sub-command finishes (cmd/root.go).
	EventCommand = "command"
	// EventProjectCreate is sent when a new Shopware project is scaffolded.
	EventProjectCreate = "project.create"
	// EventProjectUpgrade is sent when the interactive upgrade wizard finishes
	// an upgrade run (internal/tui/upgrade).
	EventProjectUpgrade = "project.upgrade"

	// The ai.* events are sent by the ai command group (cmd/ai) after the
	// integration is resolved against the directory, so TagIntegration is always a
	// known catalog value. An unrecognized name is covered by EventCommand and is
	// never transmitted here.
	EventAIAdd    = "ai.add"
	EventAIRemove = "ai.remove"

	// The project.dev.* events are sent by the interactive dev TUI (internal/tui/dev).
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

	// EventProjectUpgrade
	TagFromVersion   = "from_version"
	TagTargetVersion = "target_version"

	// EventAIAdd, EventAIRemove
	TagIntegration = "integration" // directory name, e.g. shopware-cli / meteor
	TagType        = "type"        // skill / mcp
	TagAgent       = "agent"       // skills only, enumerated client id (claude-code)
	TagScope       = "scope"       // project / global
	TagPinned      = "pinned"      // add only, skills only: a @<tag> was requested
	TagDryRun      = "dry_run"     // preview run; nothing changed on disk
	TagRemoved     = "removed"     // remove only: the integration was actually present

	// EventDevInstall
	TagAbandonedAt       = "abandoned_at"
	TagFailedStep        = "failed_step"
	TagFailureCategory   = "failure_category"
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
