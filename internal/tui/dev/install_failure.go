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

	installStartStep = "install_start"

	// installFailureDetailLines bounds how many output lines make up one
	// detail, so a stack trace or a dumped query cannot grow it without limit.
	installFailureDetailLines = 3
)

// installFailure is the classified result of one failed helper run. Telemetry
// reads this instead of scanning the raw output again. detail is kept for
// classification tests; it is not shown on the failure card and is never sent
// as a telemetry tag.
type installFailure struct {
	failingStep string
	category    installFailureCategory
	detail      string
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

func installFailurePatterns(patterns ...string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(`(?i)`+pattern))
	}
	return compiled
}

// Output is scanned from the end so the error that stopped the process wins
// over earlier warnings.
func classifyInstallFailure(output []string, processErr error) installFailure {
	lines := cleanInstallOutput(output)

	failure := installFailure{
		failingStep: installFailureStep(lines),
		category:    installFailureUnknown,
		detail:      installFailureDetail(lines, processErr),
	}

	for i := len(lines) - 1; i >= 0; i-- {
		for _, rule := range installFailureRules {
			for _, pattern := range rule.patterns {
				if pattern.MatchString(lines[i]) {
					failure.category = rule.category
					failure.detail = installFailureMessage(lines, i)
					return failure
				}
			}
		}
	}

	return failure
}

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
