package ocitest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecordsCommands(t *testing.T) {
	t.Parallel()

	rt := &Runtime{}
	cmd := rt.ComposeCommand(t.Context(), "up", "-d")
	cmd.SetDir("/project")

	require.Len(t, rt.Cmds, 1)
	assert.Same(t, cmd, rt.Last())
	assert.Equal(t, []string{"docker", "compose", "up", "-d"}, rt.Cmds[0].ArgsV)
	assert.Equal(t, "/project", rt.Cmds[0].DirV)
}

func TestRuntimeCustomBinary(t *testing.T) {
	t.Parallel()

	rt := &Runtime{BinaryName: "podman", AvailableV: true}

	assert.Equal(t, "podman", rt.Binary())
	assert.True(t, rt.Available())

	rt.Command(t.Context(), "info")
	assert.Equal(t, []string{"podman", "info"}, rt.Last().ArgsV)
}

func TestCmdDefaults(t *testing.T) {
	t.Parallel()

	c := &Cmd{}

	assert.NoError(t, c.Run())
	assert.True(t, c.Ran)

	out, err := c.Output()
	assert.NoError(t, err)
	assert.Empty(t, out)

	out, err = c.CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, out)

	assert.NoError(t, c.Start())
	assert.True(t, c.Started)
	assert.NoError(t, c.Wait())
	assert.True(t, c.Waited)

	assert.Nil(t, c.Process())
	assert.NoError(t, c.Err())
}

func TestCmdHooks(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	c := &Cmd{
		RunFunc:    func(*Cmd) error { return wantErr },
		OutputFunc: func(*Cmd) ([]byte, error) { return []byte("out"), nil },
	}

	assert.ErrorIs(t, c.Run(), wantErr)

	out, err := c.Output()
	assert.NoError(t, err)
	assert.Equal(t, "out", string(out))
}

func TestCmdStreamConfiguration(t *testing.T) {
	t.Parallel()

	c := &Cmd{}
	c.SetEnv([]string{"A=1"})
	assert.Equal(t, []string{"A=1"}, c.Env())

	c.SetStdin(nil)
	assert.Nil(t, c.Stdin())

	_, err := c.StdoutPipe()
	assert.Error(t, err)
	_, err = c.StderrPipe()
	assert.Error(t, err)
}
