package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeDocker puts a docker stub on PATH that answers
// `docker compose exec ... printenv DATABASE_URL` with execOutput (or fails
// when empty) and `docker compose port <service> <port>` with portScript.
func writeFakeDocker(t *testing.T, execOutput, portScript string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake docker binary requires a POSIX shell")
	}

	shPath, err := exec.LookPath("sh")
	require.NoError(t, err)

	execBranch := "exit 1"
	if execOutput != "" {
		execBranch = fmt.Sprintf("echo %q", execOutput)
	}

	script := fmt.Sprintf(`#!%s
if [ "$1" = "compose" ] && [ "$2" = "exec" ]; then
  %s
elif [ "$1" = "compose" ] && [ "$2" = "port" ]; then
  %s
else
  exit 1
fi
`, shPath, execBranch, portScript)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir)
}

func TestDockerDatabaseConnection(t *testing.T) {
	writeFakeDocker(t, "mysql://app:secret@database/shop", `echo "0.0.0.0:55001"`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	conn, err := dockerExec.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:55001", conn.Addr())
	assert.Equal(t, "app", conn.Username)
	assert.Equal(t, "secret", conn.Password)
	assert.Equal(t, "shop", conn.Database)
}

func TestDockerDatabaseConnectionEnvOverrideSkipsContainerLookup(t *testing.T) {
	// The exec branch fails, so passing proves the container env is not read.
	writeFakeDocker(t, "", `echo "[::]:56001"`)

	dockerExec := &DockerExecutor{
		projectRoot: t.TempDir(),
		env:         map[string]string{"DATABASE_URL": "mysql://root:root@database:3306/override"},
	}

	conn, err := dockerExec.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, "56001", conn.Port)
	assert.Equal(t, "override", conn.Database)
}

func TestDockerDatabaseConnectionExternalHostKept(t *testing.T) {
	writeFakeDocker(t, "mysql://app:pw@db.example.com:3307/prod", `echo "no such service: db.example.com" >&2; exit 1`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	conn, err := dockerExec.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "db.example.com:3307", conn.Addr())
	assert.Equal(t, "prod", conn.Database)
}

func TestDockerDatabaseConnectionUnpublishedPort(t *testing.T) {
	writeFakeDocker(t, "mysql://root:root@database/shopware", `echo ""`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(t.Context())
	require.Error(t, err)

	assert.Contains(t, err.Error(), "does not publish port")
}

func TestDockerDatabaseConnectionPortLookupFailure(t *testing.T) {
	writeFakeDocker(t, "mysql://root:root@database/shopware", `echo "daemon not reachable" >&2; exit 1`)

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(t.Context())
	require.Error(t, err)

	assert.Contains(t, err.Error(), "daemon not reachable")
}

func TestDockerDatabaseConnectionEnvironmentNotRunning(t *testing.T) {
	writeFakeDocker(t, "", "exit 1")

	dockerExec := &DockerExecutor{projectRoot: t.TempDir()}

	_, err := dockerExec.DatabaseConnection(t.Context())
	require.Error(t, err)

	assert.Contains(t, err.Error(), "could not read DATABASE_URL")
}
