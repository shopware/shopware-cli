package dev

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// symfonyErrorOutput mimics what the deployment helper prints when it fails:
// the helper runs without a TTY, so Symfony wraps the error box at 80 columns
// and pads it with whitespace-only rows.
func symfonyErrorOutput() []string {
	return []string{
		"Start: system:install",
		"",
		"In ExceptionConverter.php line 47:",
		"                                                                              ",
		"  An exception occurred in the driver: SQLSTATE[HY000] [2002] Connection      ",
		"  refused                                                                     ",
		"                                                                              ",
		"",
		"system:install [--createDatabase] [--dropDatabase] [-f|--force]",
	}
}

func TestClassifyInstallFailure(t *testing.T) {
	failure := classifyInstallFailure(symfonyErrorOutput(), assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "system:install", failure.failingStep)
	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] Connection refused", failure.detail)
}

func TestClassifyInstallFailure_BoundsTheDetail(t *testing.T) {
	output := []string{
		"permission denied while writing var/cache",
		"second line",
		"third line",
		"fourth line",
		"fifth line",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailurePermission, failure.category)
	assert.Equal(t, "permission denied while writing var/cache second line third line", failure.detail)
}

func TestClassifyInstallFailure_LastMatchingErrorWins(t *testing.T) {
	output := []string{
		"Warning: permission denied while checking an optional directory",
		"",
		"SQLSTATE[HY000] [2002] Connection refused",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "SQLSTATE[HY000] [2002] Connection refused", failure.detail)
}

func TestClassifyInstallFailure_UsesLastStartedStep(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"Start: system:install",
		"Start: theme:change",
		"Unable to compile the theme",
	}, assert.AnError)

	assert.Equal(t, "theme:change", failure.failingStep)
}

func TestClassifyInstallFailure_KnownPatterns(t *testing.T) {
	cases := []struct {
		name     string
		step     string
		line     string
		category installFailureCategory
	}{
		{"memory limit", "theme:change", "PHP Fatal error: Allowed memory size of 134217728 bytes exhausted", installFailurePHP},
		{"fatal PHP error", "theme:change", "PHP Fatal error: Call to undefined method", installFailurePHP},
		{"environment", "theme:change", "Environment variable DATABASE_URL is not defined", installFailureEnvironmentConfig},
		{"database version", "theme:change", "Requires at least MySQL 8.0", installFailureDatabaseVersion},
		{"database connection", "theme:change", "SQLSTATE[HY000] [2002] Connection refused", installFailureDatabaseConnection},
		{"migration", "theme:change", "SQLSTATE[42S02]: Base table or view not found", installFailureMigration},
		{"already installed", "theme:change", "install.lock already exists", installFailureAlreadyExists},
		{"permission", "theme:change", "Permission denied", installFailurePermission},
		{"invalid input", "theme:change", "The password must have at least 8 characters", installFailureInvalidInput},
		{"missing prerequisite", "theme:change", "Could not find theme with name Storefront", installFailureMissingPrerequisite},
		{"theme compile", "theme:change", "Unable to compile the theme", installFailureThemeCompile},
		{"transport", "theme:change", "The transport does not exist", installFailureTransport},
		{"unknown", "theme:change", "SQLSTATE[HY000] [2003] Can't connect to MySQL server", installFailureUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure := classifyInstallFailure([]string{"Start: " + tc.step, tc.line}, assert.AnError)
			assert.Equal(t, tc.category, failure.category)
		})
	}
}

func TestClassifyInstallFailure_CleansHelperOutput(t *testing.T) {
	output := []string{
		"\x1b[31m[deployment-helper] Start: system:install\x1b[0m",
		"[deployment-helper] ",
		"[deployment-helper]   An exception occurred in the driver: SQLSTATE[HY000] [2002] No such",
		"[deployment-helper]   file or directory",
		"[deployment-helper] ",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "system:install", failure.failingStep)
	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] No such file or directory", failure.detail)
}

func TestCleanInstallOutput_PreservesOtherBracketedPrefixes(t *testing.T) {
	assert.Equal(t, []string{"[error] permission denied"}, cleanInstallOutput([]string{
		"[error] permission denied",
	}))
}

func TestClassifyInstallFailure_UnknownCategoryUsesTrailingMessage(t *testing.T) {
	output := []string{
		"Start: system:install",
		"",
		"  Something entirely unexpected happened while the deployment helper was      ",
		"  finishing the installation                                                  ",
		"                                                                              ",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureUnknown, failure.category)
	assert.Equal(t, "Something entirely unexpected happened while the deployment helper was finishing the installation", failure.detail)
}

func TestClassifyInstallFailure_WithoutOutputReportsExitCode(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "exit 3")
	err := cmd.Run()
	var exitErr *exec.ExitError
	assert.True(t, errors.As(err, &exitErr))

	failure := classifyInstallFailure(nil, err)

	assert.Equal(t, installFailureUnknown, failure.category)
	assert.Equal(t, installStartStep, failure.failingStep)
	assert.Equal(t, "deployment helper exited with code 3", failure.detail)
}
