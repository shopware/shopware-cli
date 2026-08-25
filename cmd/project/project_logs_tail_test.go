//go:build !windows

package project

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pins the argument construction and the stream wiring to the cobra command,
// the only behavior tailFollow owns. The follow happy path needs a running
// tail process and is deliberately left out as flaky.
func TestTailFollowSurfacesMissingFile(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetContext(t.Context())

	err := tailFollow(cmd, filepath.Join(t.TempDir(), "missing.log"), 10)
	require.Error(t, err)
	assert.Contains(t, errOut.String(), "missing.log")
	assert.Empty(t, out.String())
}
