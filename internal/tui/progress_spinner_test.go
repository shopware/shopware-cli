package tui

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteFailureOutput(t *testing.T) {
	var output bytes.Buffer

	writeFailureOutput(
		&output,
		"Installing dependencies",
		errors.New("exit status 1"),
		[]string{"Loading composer repositories", "Your requirements could not be resolved."},
	)

	assert.Equal(t, "\n✗ Installing dependencies\n\n  Command failed: exit status 1\n  Loading composer repositories\n  Your requirements could not be resolved.\n", output.String())
}

func TestWriteFailureOutputWithoutCommandLogs(t *testing.T) {
	var output bytes.Buffer

	writeFailureOutput(&output, "Installing dependencies", errors.New("signal: killed"), nil)

	assert.Equal(t, "\n✗ Installing dependencies\n\n  Command failed: signal: killed\n", output.String())
}
