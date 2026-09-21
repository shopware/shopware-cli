package executor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
	"github.com/shopware/shopware-cli/internal/oci/ocitest"
)

// fakeDatabaseRuntime returns an oci.Runtime fake that answers
// `compose exec ... printenv DATABASE_URL` with execOutput (or fails when
// empty) and `compose port <service> <port>` via portFunc.
func fakeDatabaseRuntime(execOutput string, portFunc func(c *ocitest.Cmd) error) oci.Runtime {
	return &ocitest.Runtime{NewCmd: func(c *ocitest.Cmd) {
		switch {
		case slices.Contains(c.ArgsV, "printenv"):
			c.RunFunc = func(c *ocitest.Cmd) error {
				if execOutput == "" {
					return errors.New("exec failed")
				}
				_, _ = fmt.Fprintln(c.StdoutV, execOutput)
				return nil
			}
		case slices.Contains(c.ArgsV, "port"):
			c.RunFunc = portFunc
		default:
			c.RunFunc = func(*ocitest.Cmd) error { return errors.New("unexpected command") }
		}
	}}
}

// withFakeDatabaseRuntime returns a context carrying the fake runtime.
func withFakeDatabaseRuntime(execOutput string, portFunc func(c *ocitest.Cmd) error) context.Context {
	return oci.WithRuntime(context.Background(), fakeDatabaseRuntime(execOutput, portFunc))
}

// portAnswer returns a portFunc writing answer to stdout.
func portAnswer(answer string) func(c *ocitest.Cmd) error {
	return func(c *ocitest.Cmd) error {
		_, _ = fmt.Fprintln(c.StdoutV, answer)
		return nil
	}
}

// portFailure returns a portFunc failing with msg on stderr.
func portFailure(msg string) func(c *ocitest.Cmd) error {
	return func(c *ocitest.Cmd) error {
		_, _ = fmt.Fprintln(c.StderrV, msg)
		return errors.New("exit status 1")
	}
}

func TestDockerDatabaseConnection(t *testing.T) {
	ctx := withFakeDatabaseRuntime("mysql://app:secret@database/shop", portAnswer("0.0.0.0:55001"))

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	conn, err := dockerExec.DatabaseConnection(ctx)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:55001", conn.Addr())
	assert.Equal(t, "app", conn.Username)
	assert.Equal(t, "secret", conn.Password)
	assert.Equal(t, "shop", conn.Database)
}

func TestDockerDatabaseConnectionEnvOverrideSkipsContainerLookup(t *testing.T) {
	// The exec branch fails, so passing proves the container env is not read.
	ctx := withFakeDatabaseRuntime("", portAnswer("[::]:56001"))

	dockerExec := &DockerExecutor{
		projectRoot: t.TempDir(),
		env:         map[string]string{"DATABASE_URL": "mysql://root:root@database:3306/override"},
	}

	conn, err := dockerExec.DatabaseConnection(ctx)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, "56001", conn.Port)
	assert.Equal(t, "override", conn.Database)
}

func TestDockerDatabaseConnectionExternalHostKept(t *testing.T) {
	ctx := withFakeDatabaseRuntime("mysql://app:pw@db.example.com:3307/prod", portFailure("no such service: db.example.com"))

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	conn, err := dockerExec.DatabaseConnection(ctx)
	require.NoError(t, err)

	assert.Equal(t, "db.example.com:3307", conn.Addr())
	assert.Equal(t, "prod", conn.Database)
}

func TestDockerDatabaseConnectionUnpublishedPort(t *testing.T) {
	ctx := withFakeDatabaseRuntime("mysql://root:root@database/shopware", portAnswer(""))

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "does not publish port")
}

func TestDockerDatabaseConnectionPortLookupFailure(t *testing.T) {
	ctx := withFakeDatabaseRuntime("mysql://root:root@database/shopware", portFailure("daemon not reachable"))

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "daemon not reachable")
}

func TestDockerDatabaseConnectionEnvironmentNotRunning(t *testing.T) {
	ctx := withFakeDatabaseRuntime("", portFailure("exit 1"))

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "could not read DATABASE_URL")
}
