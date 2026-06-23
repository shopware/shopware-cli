package envfile

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

// writeEnvFile writes the given content to a file named name inside dir.
func writeEnvFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(path.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func TestLoadSymfonyEnvFileNoFiles(t *testing.T) {
	dir := t.TempDir()

	// Ensure the key is restored after the test and starts out empty.
	t.Setenv("ENVFILE_TEST_MISSING", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	assert.Empty(t, os.Getenv("ENVFILE_TEST_MISSING"))
}

func TestLoadSymfonyEnvFileSetsValues(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env", "ENVFILE_TEST_FOO=bar\nENVFILE_TEST_BAZ=qux\n")

	t.Setenv("ENVFILE_TEST_FOO", "")
	t.Setenv("ENVFILE_TEST_BAZ", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	assert.Equal(t, "bar", os.Getenv("ENVFILE_TEST_FOO"))
	assert.Equal(t, "qux", os.Getenv("ENVFILE_TEST_BAZ"))
}

func TestLoadSymfonyEnvFileExistingEnvTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env", "ENVFILE_TEST_EXISTING=from_file\n")

	// A non-empty value already set in the environment must not be overwritten.
	t.Setenv("ENVFILE_TEST_EXISTING", "from_env")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	assert.Equal(t, "from_env", os.Getenv("ENVFILE_TEST_EXISTING"))
}

func TestLoadSymfonyEnvFileLocalOverridesBase(t *testing.T) {
	dir := t.TempDir()
	// godotenv.Read merges files in order, so files later in the precedence
	// list win. .env.local must therefore override .env and .env.dist.
	writeEnvFile(t, dir, ".env.dist", "ENVFILE_TEST_OVERRIDE=from_dist\n")
	writeEnvFile(t, dir, ".env", "ENVFILE_TEST_OVERRIDE=from_env\n")
	writeEnvFile(t, dir, ".env.local", "ENVFILE_TEST_OVERRIDE=from_local\n")

	t.Setenv("ENVFILE_TEST_OVERRIDE", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	assert.Equal(t, "from_local", os.Getenv("ENVFILE_TEST_OVERRIDE"))
}

func TestLoadSymfonyEnvFileAppEnvSpecificFile(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env", "ENVFILE_TEST_APP=from_env\n")
	writeEnvFile(t, dir, ".env.prod", "ENVFILE_TEST_APP=from_prod\n")

	t.Setenv("APP_ENV", "prod")
	t.Setenv("ENVFILE_TEST_APP", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	// .env.prod comes after .env in the precedence list and wins.
	assert.Equal(t, "from_prod", os.Getenv("ENVFILE_TEST_APP"))
}

func TestLoadSymfonyEnvFileDefaultsToDev(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "ENVFILE_TEST_DEV=from_dev\n")

	// With no APP_ENV set the loader falls back to the dev environment.
	t.Setenv("APP_ENV", "")
	t.Setenv("ENVFILE_TEST_DEV", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	assert.Equal(t, "from_dev", os.Getenv("ENVFILE_TEST_DEV"))
}

func TestLoadSymfonyEnvFileEnvLocalOverride(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env."+"prod", "ENVFILE_TEST_LOCAL=from_prod\n")
	writeEnvFile(t, dir, ".env.prod.local", "ENVFILE_TEST_LOCAL=from_prod_local\n")

	t.Setenv("APP_ENV", "prod")
	t.Setenv("ENVFILE_TEST_LOCAL", "")

	err := LoadSymfonyEnvFile(dir)
	assert.NoError(t, err)
	// .env.<env>.local has the highest precedence.
	assert.Equal(t, "from_prod_local", os.Getenv("ENVFILE_TEST_LOCAL"))
}
