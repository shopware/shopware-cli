package oci

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerRuntimeCommand(t *testing.T) {
	t.Parallel()

	cmd := DockerRuntime{}.Command(t.Context(), "info")

	assert.Equal(t, []string{"docker", "info"}, cmd.Args())
}

func TestDockerRuntimeComposeCommand(t *testing.T) {
	t.Parallel()

	cmd := DockerRuntime{}.ComposeCommand(t.Context(), "up", "-d")

	assert.Equal(t, []string{"docker", "compose", "up", "-d"}, cmd.Args())
}

func TestFromContextDefaultsToDocker(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DockerRuntime{}, FromContext(t.Context()))
	assert.Equal(t, DockerRuntime{}, Default())
}

// fakeRuntime records the invocations a Runtime would shell out for, without
// executing anything.
type fakeRuntime struct {
	binary string
	calls  [][]string
}

func (f *fakeRuntime) Binary() string { return f.binary }

func (f *fakeRuntime) Available() bool { return true }

func (f *fakeRuntime) Command(_ context.Context, args ...string) Cmd {
	f.calls = append(f.calls, append([]string{f.binary}, args...))
	return WrapCommand(exec.CommandContext(context.Background(), "true"))
}

func (f *fakeRuntime) ComposeCommand(ctx context.Context, args ...string) Cmd {
	return f.Command(ctx, append([]string{"compose"}, args...)...)
}

func TestWithRuntimeInjectsRuntime(t *testing.T) {
	t.Parallel()

	fake := &fakeRuntime{binary: "podman"}
	ctx := WithRuntime(t.Context(), fake)

	assert.Same(t, fake, FromContext(ctx))
	assert.Equal(t, "podman", FromContext(ctx).Binary())

	FromContext(ctx).ComposeCommand(ctx, "ps")
	assert.Equal(t, [][]string{{"podman", "compose", "ps"}}, fake.calls)
}

func TestWithRuntimeNilFallsBackToDefault(t *testing.T) {
	t.Parallel()

	ctx := WithRuntime(t.Context(), nil)
	assert.Equal(t, DockerRuntime{}, FromContext(ctx))
}

func TestWrapCommandMirrorsExecCmd(t *testing.T) {
	t.Parallel()

	inner := exec.CommandContext(t.Context(), "echo", "hello")
	cmd := WrapCommand(inner)

	assert.Equal(t, []string{"echo", "hello"}, cmd.Args())

	cmd.SetDir("/tmp")
	assert.Equal(t, "/tmp", cmd.Dir())
	assert.Equal(t, "/tmp", inner.Dir)

	cmd.SetEnv([]string{"FOO=bar"})
	assert.Equal(t, []string{"FOO=bar"}, cmd.Env())
	assert.Equal(t, []string{"FOO=bar"}, inner.Env)

	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}
