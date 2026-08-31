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
	assert.True(t, failure.retryable)
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
		line      string
		category  installFailureCategory
		retryable bool
	}{
		{"Allowed memory size of 134217728 bytes exhausted", installFailurePHP, true},
		{"PHP Fatal error: Call to undefined method", installFailurePHP, false},
		{"Environment variable DATABASE_URL is not defined", installFailureEnvironmentConfig, true},
		{"Requires at least MySQL 8.0", installFailureDatabaseVersion, true},
		{"SQLSTATE[HY000] [2002] Connection refused", installFailureDatabaseConnection, true},
		{"SQLSTATE[42S02]: Base table or view not found", installFailureMigration, true},
		{"install.lock already exists", installFailureAlreadyExists, true},
		{"Permission denied", installFailurePermission, true},
		{"The password must have at least 8 characters", installFailureInvalidInput, true},
		{"Could not find theme with name Storefront", installFailureMissingPrerequisite, true},
		{"Unable to compile the theme", installFailureThemeCompile, true},
		{"The transport does not exist", installFailureTransport, true},
		{"SQLSTATE[HY000] [2003] Can't connect to MySQL server", installFailureUnknown, true},
	}
	for _, tc := range cases {
		failure := classifyInstallFailure([]string{tc.line}, assert.AnError)
		assert.Equal(t, tc.category, failure.category, tc.line)
		assert.Equal(t, tc.retryable, failure.retryable, tc.line)
	}
}

func TestClassifyInstallFailure_UserCreateIsNotRetryable(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"Start: user:create",
		"The password must have at least 8 characters",
	}, assert.AnError)

	assert.Equal(t, installFailureInvalidInput, failure.category)
	assert.False(t, failure.retryable)
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

func TestInstallFailureRemediation(t *testing.T) {
	cases := []struct {
		category installFailureCategory
		detail   string
		docker   bool
		want     string
	}{
		{installFailurePHP, "PHP Fatal error", false, "Fix the PHP error"},
		{installFailureEnvironmentConfig, "", false, "DATABASE_URL"},
		{installFailureDatabaseVersion, "", true, "compose.yaml"},
		{installFailureDatabaseConnection, "SQLSTATE[HY000] [2002] Connection refused", true, "docker compose up -d database"},
		{installFailureMigration, "", false, "Drop the database"},
		{installFailureAlreadyExists, "install.lock already exists", false, "install.lock"},
		{installFailurePermission, "", false, "write access"},
		{installFailureInvalidInput, "The password must have at least 8 characters", false, "8 characters"},
		{installFailureMissingPrerequisite, "Could not find theme", false, "Storefront package"},
		{installFailureThemeCompile, "", false, "theme.json"},
		{installFailureTransport, "The transport does not exist", false, "MESSENGER_TRANSPORT_DSN"},
		{installFailureUnknown, "something odd", false, ""},
	}

	for _, tc := range cases {
		got := installFailure{category: tc.category, detail: tc.detail}.remediation(tc.docker)
		if tc.want == "" {
			assert.Empty(t, got, "%s %q", tc.category, tc.detail)
			continue
		}
		assert.Contains(t, got, tc.want, "%s %q", tc.category, tc.detail)
	}
}
