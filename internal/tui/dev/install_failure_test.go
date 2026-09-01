package dev

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyInstallFailure(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"Start: system:install",
		"[deployment-helper] \x1b[31mSQLSTATE[42S01]: Base table or view already exists\x1b[0m",
	}, assert.AnError)

	assert.Equal(t, "system:install", failure.failingStep)
	assert.Equal(t, installFailureMigration, failure.category)
	assert.False(t, failure.retryable)
	assert.NotContains(t, failure.detail, "\x1b")
}

func TestClassifyInstallFailureUsesTerminalError(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"Start: system:install",
		"SQLSTATE[HY000] [2002] Connection refused",
		"PHP Fatal error: syntax error, unexpected token",
	}, assert.AnError)

	assert.Equal(t, installFailurePHP, failure.category)
	assert.False(t, failure.retryable)
}

func TestClassifyInstallFailureBeforeFirstStep(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"[deployment-helper] SQLSTATE[HY000] [2002] Connection refused",
	}, assert.AnError)

	assert.Equal(t, installStartStep, failure.failingStep)
	assert.Equal(t, installFailureDatabaseConnection, failure.category)
	assert.False(t, failure.retryable)
}

func TestClassifyInstallFailureUnknownAfterSafeStep(t *testing.T) {
	failure := classifyInstallFailure([]string{
		"Start: messenger:setup-transports",
		"unexpected helper failure",
	}, assert.AnError)

	assert.Equal(t, installFailureUnknown, failure.category)
	assert.True(t, failure.retryable)
}
