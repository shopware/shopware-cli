package executor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
)

// fakeDatabaseRuntime returns an oci.Runtime stub that answers
// `compose exec ... printenv DATABASE_URL` with execOutput (or fails when
// empty) and `compose port <service> <port>` with portScript.
func fakeDatabaseRuntime(t *testing.T, execOutput, portScript string) oci.Runtime {
	t.Helper()

	execBranch := "exit 1"
	if execOutput != "" {
		execBranch = fmt.Sprintf("echo %q", execOutput)
	}

	return writeStubRuntime(t, fmt.Sprintf(`if [ "$1" = "compose" ] && [ "$2" = "exec" ]; then
  %s
elif [ "$1" = "compose" ] && [ "$2" = "port" ]; then
  %s
else
  exit 1
fi
`, execBranch, portScript))
}

// withFakeDatabaseRuntime returns a context carrying the stub runtime.
func withFakeDatabaseRuntime(t *testing.T, execOutput, portScript string) context.Context {
	t.Helper()
	return oci.WithRuntime(t.Context(), fakeDatabaseRuntime(t, execOutput, portScript))
}

func TestDockerDatabaseConnection(t *testing.T) {
	ctx := withFakeDatabaseRuntime(t, "mysql://app:secret@database/shop", `echo "0.0.0.0:55001"`)

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
	ctx := withFakeDatabaseRuntime(t, "", `echo "[::]:56001"`)

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
	ctx := withFakeDatabaseRuntime(t, "mysql://app:pw@db.example.com:3307/prod", `echo "no such service: db.example.com" >&2; exit 1`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	conn, err := dockerExec.DatabaseConnection(ctx)
	require.NoError(t, err)

	assert.Equal(t, "db.example.com:3307", conn.Addr())
	assert.Equal(t, "prod", conn.Database)
}

func TestDockerDatabaseConnectionUnpublishedPort(t *testing.T) {
	ctx := withFakeDatabaseRuntime(t, "mysql://root:root@database/shopware", `echo ""`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "does not publish port")
}

func TestDockerDatabaseConnectionPortLookupFailure(t *testing.T) {
	ctx := withFakeDatabaseRuntime(t, "mysql://root:root@database/shopware", `echo "daemon not reachable" >&2; exit 1`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "daemon not reachable")
}

func TestDockerDatabaseConnectionEnvironmentNotRunning(t *testing.T) {
	ctx := withFakeDatabaseRuntime(t, "", "exit 1")

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(ctx)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "could not read DATABASE_URL")
}
