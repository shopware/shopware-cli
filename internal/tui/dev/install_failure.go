package dev

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type installFailureCategory string

const (
	installFailureDiskSpace           installFailureCategory = "disk_space"
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
)

// installFailure is the shared description of one failed helper run. Later
// telemetry, diagnostic UI, and retry work can consume this record without
// having to classify the raw process output again.
type installFailure struct {
	failingStep string
	category    installFailureCategory
	detail      string
	retryable   bool
}

type installFailureRule struct {
	category installFailureCategory
	patterns []*regexp.Regexp // compiled regex patterns to match against the output of a failed helper run
}

// installFailureRules is a list of known failure patterns that can be used to
// classify the output of a failed helper run. The first matching rule is used
// to classify the failure.
var installFailureRules = []installFailureRule{
	{
		category: installFailureDiskSpace,
		patterns: installFailurePatterns(
			`no space left on device`,
		),
	},
	{
		category: installFailurePHP,
		patterns: installFailurePatterns(
			`allowed memory size`,
			`outofmemoryerror`,
			`php fatal error`,
			`uncaught error`,
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
			`\[2002\]`,
			`\[1045\]`,
			`\[1044\]`,
		),
	},
	{
		category: installFailureMigration,
		patterns: installFailurePatterns(
			`\[42s01\]`,
			`\[42s02\]`,
			`table .* doesn't exist`,
			`sqlstate\[`,
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
			`transport does not exist`,
		),
	},
	{
		category: installFailureMissingPrerequisite,
		patterns: installFailurePatterns(
			`snippet set with isocode`,
			`could not get id of`,
			`could not find theme with`,
			`invalid theme name`,
			`from plugin registry`,
		),
	},
	{
		category: installFailureThemeCompile,
		patterns: installFailurePatterns(
			`unable to compile the theme`,
			`error while trying to concatenate styles`,
			`unable to resolve file`,
			`unable to .* theme\.json`,
			`is not valid for type`,
			`unable to find setter for config field`,
			`error loading runtime config for theme`,
			`error while trying to write compiled files`,
		),
	},
	{
		category: installFailureTransport,
		patterns: installFailurePatterns(
			`while setting up the .* transport`,
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

const installStartStep = "install_start"


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
// returns a structured description of the failure. The first matching rule is
// used to classify the failure.
func classifyInstallFailure(output []string, processErr error) installFailure {
	failure := installFailure{
		failingStep: installFailureStep(output),
		category:    installFailureUnknown,
		detail:      installFailureDetail(output, processErr),
		retryable:   true,
	}

	for _, line := range output {
		for _, rule := range installFailureRules {
			if matchesInstallFailureRule(line, rule.patterns) {
				failure.category = rule.category
				failure.detail = strings.TrimSpace(line)
				// Syntax and parse errors require a code/configuration change,
				// so immediately re-running the same command cannot recover.
				normalized := strings.ToLower(line)
				if rule.category == installFailurePHP &&
					(strings.Contains(normalized, "fatal error") || strings.Contains(normalized, "syntax error")) {
					failure.retryable = false
				}
				return failure
			}
		}
	}

	return failure
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
		for _, step := range installStepPatterns {
			if strings.Contains(line, step.pattern) {
				failingStep = step.pattern
				break
			}
		}
	}
	return failingStep
}

// installFailureDetail returns the last non-empty line of the output as a
// human-readable description of the failure. If no non-empty line is found, it
// returns the error message from the process exit error, if any. If there is no
// output and no process error, it returns a generic message indicating that the
// helper failed without diagnostic output.
func installFailureDetail(output []string, processErr error) string {
	for i := len(output) - 1; i >= 0; i-- {
		if detail := strings.TrimSpace(output[i]); detail != "" {
			return detail
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