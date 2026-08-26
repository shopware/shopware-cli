package tui

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamCmdsOutputWithCapture_RunsInOrderAndCloses(t *testing.T) {
	ch := make(chan string, 8)
	cmd1 := exec.Command("sh", "-c", "echo one")
	cmd2 := exec.Command("sh", "-c", "echo two")

	lines, err := StreamCmdsOutputWithCapture([]*exec.Cmd{cmd1, cmd2}, ch, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, lines)
	assert.Equal(t, []string{"one", "two"}, drainClosed(t, ch))
}

func TestStreamCmdsOutputWithCapture_StopsOnFirstFailure(t *testing.T) {
	ch := make(chan string, 8)
	fail := exec.Command("sh", "-c", "echo failed; exit 3")
	skip := exec.Command("sh", "-c", "echo skipped")

	lines, err := StreamCmdsOutputWithCapture([]*exec.Cmd{fail, skip}, ch, true)
	require.Error(t, err)
	assert.Equal(t, []string{"failed"}, lines)
	assert.Equal(t, []string{"failed"}, drainClosed(t, ch))
}

func drainClosed(t *testing.T, ch <-chan string) []string {
	t.Helper()
	var got []string
	for line := range ch {
		got = append(got, line)
	}
	return got
}
