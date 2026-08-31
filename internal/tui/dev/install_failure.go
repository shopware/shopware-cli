package dev

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/shopware/shopware-cli/internal/shop/install"
)

type installFailureCategory string

const (
	installFailurePHP                 installFailureCategory = "php"
	installFailureEnvironmentConfig   installFailureCategory = "env_config"
	installFailureDatabaseVersion     installFailureCategory = "db_version"
	installFailureDatabaseConnection  installFailureCategory = "db_connection"
	installFailureMigration           installFailureCategory = "migration"
	installFailureAlreadyExists       installFailureCategory = "already_exists"
	installFailurePermission          installFailureCategory = "permission"
	installFailureInvalidInput        installFailureCategory = "invalid_input"
	installFailureMissingPrerequisite installFailureCategory = "missing_prerequisite"
	installFailureThemeCompile        installFailureCategory = "theme_compile"
	installFailureTransport           installFailureCategory = "transport"
	installFailureUnknown             installFailureCategory = "unknown"

	installStartStep      = "install_start"
	installUserCreateStep = "user:create"

	// installFailureDetailLines bounds how many output lines make up one
	// detail, so a stack trace or a dumped query cannot grow it without limit.
	installFailureDetailLines = 3
)

// installFailure is the classified result of one failed helper run. The
// failure screen, resume logic, and telemetry all read this instead of
// scanning the raw output again. detail is kept for matching hints and tests;
// it is not shown on the failure card and is never sent as a telemetry tag.
type installFailure struct {
	failingStep string
	category    installFailureCategory
	detail      string
	retryable   bool
}

type installFailureRule struct {
	category installFailureCategory
	patterns []*regexp.Regexp
}

// installFailureRules lists known failure patterns. Output lines are checked
// from the end, so a terminal error wins over an earlier warning.
var installFailureRules = []installFailureRule{
	{
		category: installFailurePHP,
		patterns: installFailurePatterns(
			`allowed memory size`,
			`outofmemoryerror`,
			`php fatal error`,
			`syntax error, unexpected`,
		),
	},
	{
		category: installFailureEnvironmentConfig,
		patterns: installFailurePatterns(
			`environment variable .* is not defined`,
			`connection information is not valid\. missing parameter`,
		),
	},
	{
		category: installFailureDatabaseVersion,
		patterns: installFailurePatterns(
			`requires at least mysql`,
			`failed to select database version`,
		),
	},
	{
		category: installFailureDatabaseConnection,
		patterns: installFailurePatterns(
			`sqlstate\[hy000\] \[2002\]`,
			`sqlstate\[hy000\] \[1045\]`,
			`sqlstate\[hy000\] \[1044\]`,
		),
	},
	{
		category: installFailureMigration,
		patterns: installFailurePatterns(
			`\[42s01\]`,
			`\[42s02\]`,
			`\[42s22\]`,
			`base table or view not found`,
		),
	},
	{
		category: installFailureAlreadyExists,
		patterns: installFailurePatterns(
			`username .* already exists\.`,
			`install\.lock already exists`,
		),
	},
	{
		category: installFailurePermission,
		patterns: installFailurePatterns(
			`permission denied`,
			`could not create directory`,
		),
	},
	{
		category: installFailureInvalidInput,
		patterns: installFailurePatterns(
			`the password must have at least`,
		),
	},
	{
		category: installFailureMissingPrerequisite,
		patterns: installFailurePatterns(
			`snippet set with isocode`,
			`could not get id of`,
			`could not find theme with`,
			`invalid theme name`,
		),
	},
	{
		category: installFailureThemeCompile,
		patterns: installFailurePatterns(
			`unable to compile the theme`,
			`error while trying to concatenate styles`,
			`unable to .* theme\.json`,
			`unable to find setter for config field`,
			`error loading runtime config for theme`,
			`error while trying to write compiled files`,
		),
	},
	{
		category: installFailureTransport,
		patterns: installFailurePatterns(
			`while setting up the .* transport`,
			`transport does not exist`,
		),
	},
}

// installFailurePatterns compiles a list of string patterns into a list of
// regular expressions. Each pattern is treated as case-insensitive.
func installFailurePatterns(patterns ...string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(`(?i)`+pattern))
	}
	return compiled
}

// matchesInstallFailureRule returns true if the given value matches any of the
// regular expressions in the given list of patterns.
func matchesInstallFailureRule(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

// classifyInstallFailure analyzes the output of a failed helper run and
// returns a structured description of the failure. Output is scanned from the
// end so the error that stopped the process wins over earlier warnings.
func classifyInstallFailure(output []string, processErr error) installFailure {
	lines := cleanInstallOutput(output)

	failure := installFailure{
		failingStep: installFailureStep(lines),
		category:    installFailureUnknown,
		detail:      installFailureDetail(lines, processErr),
	}

	for i := len(lines) - 1; i >= 0; i-- {
		for _, rule := range installFailureRules {
			if matchesInstallFailureRule(lines[i], rule.patterns) {
				failure.category = rule.category
				failure.detail = installFailureMessage(lines, i)
				failure.retryable = isRetryableInstallFailure(failure)
				return failure
			}
		}
	}

	failure.retryable = isRetryableInstallFailure(failure)
	return failure
}

// isRetryableInstallFailure reports whether it is safe to re-run the failed
// step. user:create is never retried: that would put the admin password on
// the command line. PHP fatal/syntax errors need a code change first.
func isRetryableInstallFailure(f installFailure) bool {
	if f.failingStep == installUserCreateStep {
		return false
	}
	if f.category == installFailurePHP {
		detail := strings.ToLower(f.detail)
		if strings.Contains(detail, "fatal error") || strings.Contains(detail, "syntax error") {
			return false
		}
	}
	return true
}

// installFailureStep returns the last known step of the failed helper run
// based on the output lines. If no step can be determined, the start step is
// returned.
func installFailureStep(output []string) string {
	failingStep := installStartStep
	for _, line := range output {
		if !strings.HasPrefix(line, "Start: ") {
			continue
		}
		for _, step := range install.Steps {
			if strings.Contains(line, step.Pattern) {
				failingStep = step.Pattern
				break
			}
		}
	}
	return failingStep
}

// installFailureDetail returns the message ending the output as a
// human-readable description of the failure. If no non-empty line is found, it
// returns the error message from the process exit error, if any. If there is no
// output and no process error, it returns a generic message indicating that the
// helper failed without diagnostic output.
func installFailureDetail(output []string, processErr error) string {
	for i := len(output) - 1; i >= 0; i-- {
		if output[i] != "" {
			return installFailureMessage(output, i)
		}
	}

	var exitErr *exec.ExitError
	if errors.As(processErr, &exitErr) {
		return fmt.Sprintf("deployment helper exited with code %d", exitErr.ExitCode())
	}
	if processErr != nil {
		return processErr.Error()
	}
	return "deployment helper failed without diagnostic output"
}

// installRelayPrefix matches the tag the deployment helper puts in front of
// every line it relays from the console commands it runs.
var installRelayPrefix = regexp.MustCompile(`^\[deployment-helper] ?`)

// cleanInstallOutput removes ANSI styling, the relay prefix, and surrounding
// whitespace from the captured helper output, so rule matching and the stored
// detail work on the plain message. The log view keeps rendering the original
// lines.
func cleanInstallOutput(output []string) []string {
	cleaned := make([]string, len(output))
	for i, line := range output {
		cleaned[i] = cleanInstallLine(line)
	}
	return cleaned
}

func cleanInstallLine(line string) string {
	line = installRelayPrefix.ReplaceAllString(ansi.Strip(line), "")
	return strings.TrimSpace(line)
}

// installFailureMessage returns the message the line at idx belongs to. Symfony
// wraps long errors over several output lines, so one line on its own is often
// half a sentence. The neighbouring non-empty lines are joined back into one
// message; a blank line ends it.
func installFailureMessage(lines []string, idx int) string {
	start, end := idx, idx
	for start > 0 && lines[start-1] != "" && end-start+1 < installFailureDetailLines {
		start--
	}
	for end < len(lines)-1 && lines[end+1] != "" && end-start+1 < installFailureDetailLines {
		end++
	}
	return strings.Join(lines[start:end+1], " ")
}

// label returns a human-readable description of the category for the failure
// screen. The raw category values stay reserved for telemetry tags.
func (c installFailureCategory) label() string {
	switch c {
	case installFailurePHP:
		return "PHP error"
	case installFailureEnvironmentConfig:
		return "Incomplete environment configuration"
	case installFailureDatabaseVersion:
		return "Unsupported database version"
	case installFailureDatabaseConnection:
		return "Database connection failed"
	case installFailureMigration:
		return "Database migration failed"
	case installFailureAlreadyExists:
		return "Shopware is already installed"
	case installFailurePermission:
		return "Missing file permissions"
	case installFailureInvalidInput:
		return "Invalid input"
	case installFailureMissingPrerequisite:
		return "Missing prerequisite"
	case installFailureThemeCompile:
		return "Theme compilation failed"
	case installFailureTransport:
		return "Message transport setup failed"
	case installFailureUnknown:
		return "Unknown error"
	}
	return "Unknown error"
}

// remediation returns a concrete next action for the failure screen, or empty
// when the output does not point to a fix we can name. docker distinguishes
// compose-based setups from a local PHP install.
func (f installFailure) remediation(docker bool) string {
	detail := strings.ToLower(f.detail)
	switch f.category {
	case installFailurePHP:
		if strings.Contains(detail, "allowed memory size") || strings.Contains(detail, "outofmemory") {
			if docker {
				return "Raise PHP memory_limit in the web container, recreate it, then retry."
			}
			return "Raise PHP memory_limit in php.ini, then retry."
		}
		return "Fix the PHP error in the logs. Retrying will fail again until the code or configuration changes."
	case installFailureEnvironmentConfig:
		return "Fill in DATABASE_URL and the other required values in .env, then retry."
	case installFailureDatabaseVersion:
		if docker {
			return "Use a MySQL or MariaDB version Shopware supports in compose.yaml, recreate the database service, then use Start over."
		}
		return "Upgrade MySQL or MariaDB to a version Shopware supports, then use Start over."
	case installFailureDatabaseConnection:
		switch {
		case strings.Contains(detail, "[1045]"):
			return "Correct the user and password in DATABASE_URL in .env, then retry."
		case strings.Contains(detail, "[1044]"):
			return "Grant that DATABASE_URL user rights to create and use the database, then retry."
		case docker:
			return "Start the database container (docker compose up -d database), check that DATABASE_URL uses host \"database\", then retry."
		default:
			return "Start MySQL/MariaDB and check that DATABASE_URL in .env points at it, then retry."
		}
	case installFailureMigration:
		return "The schema was left half-applied. Drop the database and use Start over, or fix the migration shown in the logs first."
	case installFailureAlreadyExists:
		if strings.Contains(detail, "username") {
			return "That admin user already exists. Drop the database and use Start over, or keep the existing user and continue from the next step."
		}
		return "Shopware is already installed (install.lock). Remove install.lock and drop the database only if you want a fresh install, then use Start over."
	case installFailurePermission:
		if docker {
			return "Give the container user write access to var/, custom/, and files/ (check the compose user mapping), then retry."
		}
		return "Give the PHP user write access to var/, custom/, and files/, then retry."
	case installFailureInvalidInput:
		return "Choose an admin password of at least 8 characters and use Start over."
	case installFailureMissingPrerequisite:
		switch {
		case strings.Contains(detail, "snippet set") || strings.Contains(detail, "isocode"):
			return "Pick a language Shopware ships with (for example en-GB or de-DE) and use Start over."
		case strings.Contains(detail, "theme"):
			return "Install the Storefront package (shopware/storefront) and make sure its theme.json is present, then retry."
		default:
			return "Install the missing plugin or package named in the logs, then retry."
		}
	case installFailureThemeCompile:
		return "Fix the theme.json / SCSS error in the logs and ensure var/ is writable, then retry."
	case installFailureTransport:
		return "Set MESSENGER_TRANSPORT_DSN in .env to a working transport and start that service, then retry."
	case installFailureUnknown:
		return ""
	}
	return ""
}

// installFailureStepLabel maps a failing step back to the label shown in the
// install progress list, so both screens name the same step identically.
func installFailureStepLabel(step string) string {
	for _, sp := range install.Steps {
		if sp.Pattern == step {
			return sp.Label
		}
	}
	return "Starting installation"
}
