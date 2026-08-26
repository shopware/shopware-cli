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
	cmd := exec.Command("sh", "-c", "exit 3")
	err := cmd.Run()
	var exitErr *exec.ExitError
	assert.True(t, errors.As(err, &exitErr))

	failure := classifyInstallFailure(nil, err)

	assert.Equal(t, installFailureUnknown, failure.category)
	assert.Equal(t, installStartStep, failure.failingStep)
	assert.Equal(t, "deployment helper exited with code 3", failure.detail)
}
