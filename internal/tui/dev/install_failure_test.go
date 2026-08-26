package dev

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// symfonyErrorOutput mimics what the deployment helper prints when it fails:
// the helper runs without a TTY, so Symfony wraps the error box at 80 columns
// and pads every row of it with spaces.
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

func TestClassifyInstallFailure_StitchesWrappedErrorBox(t *testing.T) {
	failure := classifyInstallFailure(symfonyErrorOutput(), assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "system:install", failure.failingStep)
	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] Connection refused", failure.detail)
	assert.True(t, failure.retryable)
}

// A rule can match the wrapped remainder of a message, not just its first
// line — the detail still has to start at the beginning of the message.
func TestClassifyInstallFailure_StitchesLinesBeforeTheMatch(t *testing.T) {
	output := []string{
		"  An exception occurred while executing a query: Base table or view not  ",
		"  found: 1146 Table 'shopware.migration' doesn't exist                   ",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureMigration, failure.category)
	assert.Equal(t, "An exception occurred while executing a query: Base table or view not found: 1146 Table 'shopware.migration' doesn't exist", failure.detail)
}

// Lines outside the error box must not be glued to the message.
func TestClassifyInstallFailure_StopsAtBoxBoundaries(t *testing.T) {
	failure := classifyInstallFailure(symfonyErrorOutput(), assert.AnError)

	assert.NotContains(t, failure.detail, "In ExceptionConverter.php")
	assert.NotContains(t, failure.detail, "--createDatabase")
}

// Unindented output is one record per line, so neighbouring lines belong to
// other messages and stitching them would invent an error that never happened.
func TestClassifyInstallFailure_KeepsUnindentedLinesSeparate(t *testing.T) {
	output := []string{
		"PHP Fatal error:  Uncaught Error: Call to undefined method Shopware\\Kernel::boot()",
		"Stack trace:",
		"#0 /var/www/html/bin/console(28): require()",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailurePHP, failure.category)
	assert.Equal(t, "PHP Fatal error:  Uncaught Error: Call to undefined method Shopware\\Kernel::boot()", failure.detail)
	assert.False(t, failure.retryable)
}

// The helper relays the console output of every step behind its own tag, which
// pushes the error box to the right. The tag is noise in the detail and must
// not stop the box from being recognized.
func TestClassifyInstallFailure_StitchesRelayedErrorBox(t *testing.T) {
	output := []string{
		"[deployment-helper] Start: system:install",
		"[deployment-helper] ",
		"[deployment-helper] In ExceptionConverter.php line 47:",
		"[deployment-helper]                                                            ",
		"[deployment-helper]   An exception occurred in the driver: SQLSTATE[HY000] [2002]",
		"[deployment-helper]   php_network_getaddresses: getaddrinfo for database failed",
		"[deployment-helper]                                                            ",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "system:install", failure.failingStep)
	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] php_network_getaddresses: getaddrinfo for database failed", failure.detail)
	assert.NotContains(t, failure.detail, "deployment-helper")
}

// The relayed message is the whole line when the helper does not box it.
func TestClassifyInstallFailure_DropsRelayPrefixFromSingleLine(t *testing.T) {
	output := []string{"[deployment-helper] An exception occurred in the driver: SQLSTATE[HY000] [2002] No such file or directory"}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] No such file or directory", failure.detail)
}

func TestClassifyInstallFailure_StripsANSIStyling(t *testing.T) {
	output := []string{
		"\x1b[37;41m  An exception occurred in the driver: SQLSTATE[HY000] [2002] No such  \x1b[39;49m",
		"\x1b[37;41m  file or directory                                                    \x1b[39;49m",
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.Equal(t, "An exception occurred in the driver: SQLSTATE[HY000] [2002] No such file or directory", failure.detail)
}

// Runaway indented output (a stack trace, a dumped query) must not grow the
// detail without limit.
func TestClassifyInstallFailure_BoundsTheStitchedDetail(t *testing.T) {
	output := []string{"  no space left on device"}
	for range 50 {
		output = append(output, "  more indented output")
	}

	failure := classifyInstallFailure(output, assert.AnError)

	assert.Equal(t, installFailureDiskSpace, failure.category)
	assert.Equal(t, installFailureMessageLines, strings.Count(failure.detail, "more indented output")+1)
}

// Without a matching rule the detail falls back to the message ending the
// output, which is wrapped just the same.
func TestClassifyInstallFailure_UnknownCategoryStitchesTrailingMessage(t *testing.T) {
	output := []string{
		"Start: system:install",
		"                                                                              ",
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

func TestInstallFailureRemediation_ConcreteWherePossible(t *testing.T) {
	cases := []struct {
		category installFailureCategory
		detail   string
		docker   bool
		want     string
	}{
		{installFailureDatabaseConnection, "SQLSTATE[HY000] [2002] Connection refused", true, "docker compose up -d database"},
		{installFailureDatabaseConnection, "SQLSTATE[HY000] [1045] Access denied", false, "Correct the user and password in DATABASE_URL"},
		{installFailureDatabaseConnection, "SQLSTATE[HY000] [1044] Access denied for database", false, "Grant that DATABASE_URL user rights"},
		{installFailurePHP, "PHP Fatal error: syntax error, unexpected", false, "Fix the PHP error in the logs"},
		{installFailurePHP, "Allowed memory size of 134217728 bytes exhausted", true, "Raise PHP memory_limit in the web container"},
		{installFailureAlreadyExists, "Username admin already exists.", false, "That admin user already exists"},
		{installFailureAlreadyExists, "install.lock already exists", false, "install.lock"},
		{installFailureInvalidInput, "The password must have at least 8 characters", false, "at least 8 characters"},
		{installFailureInvalidInput, "The transport does not exist", false, "MESSENGER_TRANSPORT_DSN"},
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

func TestInstallFailureRemediation_KnownCategoriesAreNonEmpty(t *testing.T) {
	for _, category := range []installFailureCategory{
		installFailureDiskSpace,
		installFailurePHP,
		installFailureEnvironmentConfig,
		installFailureDatabaseVersion,
		installFailureDatabaseConnection,
		installFailureMigration,
		installFailureAlreadyExists,
		installFailurePermission,
		installFailureInvalidInput,
		installFailureMissingPrerequisite,
		installFailureThemeCompile,
		installFailureTransport,
	} {
		assert.NotEmpty(t, installFailure{category: category}.remediation(true), string(category))
		assert.NotEmpty(t, installFailure{category: category}.remediation(false), string(category))
	}
	assert.Empty(t, installFailure{category: installFailureUnknown}.remediation(true))
}
