package tracking

// Event names passed to Track. The "shopware_cli." prefix is added by Track
// itself. Every event and its tags are documented in docs/TELEMETRY.md; new
// events must be added there as well.
const (
	// EventToolSubprocess measures duration and captured output for each tool subprocess attempt.
	EventToolSubprocess = "tool.subprocess"
	// EventCommand is sent after (almost) any sub-command finishes (cmd/root.go).
	EventCommand = "command"
	// EventProjectCreate is sent when a new Shopware project is scaffolded.
	EventProjectCreate = "project.create"
	// EventProjectUpgrade is sent when an upgrade run ends or stops at a blocking check.
	EventProjectUpgrade = "project.upgrade"

	// The ai.* events are sent by the ai command group (cmd/ai) after the
	// integration is resolved against the directory, so TagIntegration is always a
	// known catalog value. An unrecognized name is covered by EventCommand and is
	// never transmitted here.
	EventAIAdd    = "ai.add"
	EventAIRemove = "ai.remove"

	// The project.dev.* events are sent by the interactive dev TUI (internal/tui/dev).
	EventDevSession         = "project.dev.session"
	EventDevSessionStart    = "project.dev.session_start"
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
	// EventToolSubprocess
	TagTool        = "tool"
	TagOperation   = "operation"
	TagExitCode    = "exit_code"
	TagStdoutBytes = "stdout_bytes"
	TagStderrBytes = "stderr_bytes"
	TagStarted     = "started"

	// Every event, set by Track
	TagCLIVersion = "cli_version"
	TagSessionID  = "session_id"

	// EventCommand
	TagCommandName = "command_name"
	TagResult      = "result"
	TagDurationMS  = "duration_ms"
	TagOS          = "os"
	TagIsTUI       = "is_tui"
	TagIsWSL       = "is_wsl"
	TagCIProvider  = "ci_provider"
	TagMissing     = "missing"
	// TagShopwareVersion is the project's Shopware version, on project commands run inside a project.
	TagShopwareVersion = "shopware_version"
	// Tools behind a failed tool report (validate, fix, format), sorted and comma-separated.
	TagFailedTools            = "failed_tools"
	TagToolsWithErrorFindings = "tools_with_error_findings"
	// TagFailedStep is the step a failed or cancelled run stopped at, from FailedAt here.
	// Also on EventProjectUpgrade, EventDevInstall and EventDevWatcher.
	TagFailedStep = "failed_step"
	// Input counts of validate, fix and format, set by RunTools with SetCommandTag.
	// Configured bundles are not extensions, so they only count as directories.
	TagExtensionCount     = "extension_count"
	TagSourceDirCount     = "source_dir_count"
	TagAdminDirCount      = "admin_dir_count"
	TagStorefrontDirCount = "storefront_dir_count"
	// The Shopware version extension validate checked against and its
	// --check-against mode, set by PHPStan with SetCommandTag.
	TagCheckedShopwareVersion = "checked_shopware_version"
	TagCheckAgainst           = "check_against"
	// CPU use of the same runs: the tool subprocesses' CPU time and the logical CPUs the CLI can run on.
	// TagCPUMS is also on EventToolSubprocess, for that one subprocess, and is not sent on Windows.
	TagCPUMS    = "cpu_ms"
	TagCPUCount = "cpu_count"

	// EventProjectCreate
	TagVersion           = "version"
	TagDeployment        = "deployment"
	TagCI                = "ci"
	TagDocker            = "docker"
	TagWithElasticsearch = "with_elasticsearch"
	TagWithAMQP          = "with_amqp"
	TagInteractive       = "interactive"

	// EventProjectUpgrade (reuses TagFailedStep)
	TagFromVersion    = "from_version"
	TagTargetVersion  = "target_version"
	TagBlockingChecks = "blocking_checks"

	// EventAIAdd, EventAIRemove
	TagIntegration = "integration" // directory name, e.g. shopware-cli / meteor
	TagType        = "type"        // skill / mcp
	TagAgent       = "agent"       // skills only, enumerated client id (claude-code)
	TagScope       = "scope"       // project / global
	TagPinned      = "pinned"      // add only, skills only: a @<tag> was requested
	TagDryRun      = "dry_run"     // preview run; nothing changed on disk
	TagRemoved     = "removed"     // remove only: the integration was actually present

	// EventDevInstall (reuses TagFailedStep)
	TagAbandonedAt       = "abandoned_at"
	TagFailureCategory   = "failure_category"
	TagLanguage          = "language"
	TagCurrency          = "currency"
	TagCustomCredentials = "custom_credentials"

	// EventDevMigrationWizard
	TagPHPVersion            = "php_version"
	TagDeploymentHelperAdded = "deployment_helper_added"

	// EventDevDockerStart (reuses TagFailureCategory)
	TagTrigger = "trigger"

	// EventDevAction
	TagAction = "action"

	// EventDevWatcher (reuses TagFailedStep)
	TagWatcher = "watcher"
	TagProxied = "proxied"
	TagPhase   = "phase"

	// EventDevHealth
	TagCheck = "check"

	// EventDevSession and EventDevSessionStart
	TagExecutor = "executor"

	// EventDevSession
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
