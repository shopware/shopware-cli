package deployment

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/envfile"
)

// TestSSHDeploymentInitializationWritesProtectedConfiguration checks secret transport, dotenv round trips and restrictive file permissions.
func TestSSHDeploymentInitializationWritesProtectedConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	argumentsLog := filepath.Join(t.TempDir(), "ssh-arguments")
	t.Setenv("SSH_ARGS_LOG", argumentsLog)
	state, err := e.initStore.Inspect(t.Context())
	require.NoError(t, err)
	assert.False(t, state.Initialized)
	assert.False(t, state.HasCurrent)
	assert.True(t, state.HasRuntimeConfig)
	assert.False(t, state.HasInstallConfig)
	assert.True(t, slices.Contains(state.RuntimeKeys, "DATABASE_URL"))
	assert.True(t, state.HasSharedDirectory)
	runtimeContent, err := os.ReadFile(filepath.Join(root, "shared/.env.local"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256(runtimeContent)), state.RuntimeSHA256)

	err = e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{
			"APP_ENV": "prod", "APP_URL": "https://shop.example.com", "APP_SECRET": "secret",
			"DATABASE_URL": "mysql://user:p%40ss@db.example.com:3306/shopware?ssl=true",
		},
		InstallValues: map[string]string{
			"INSTALL_ADMIN_USERNAME": "admin",
			"INSTALL_ADMIN_PASSWORD": `pa$$"word\value`,
			"INSTALL_ADMIN_EMAIL":    "admin@example.com",
		},
	})
	require.NoError(t, err)
	arguments, err := os.ReadFile(argumentsLog)
	require.NoError(t, err)
	assert.NotContains(t, string(arguments), "pa$$")
	assert.NotContains(t, string(arguments), "p%40ss")
	runtimeValues, err := envfile.ReadValues(filepath.Join(root, "shared"), "APP_ENV", "APP_URL", "APP_SECRET", "DATABASE_URL")
	require.NoError(t, err)
	assert.Equal(t, "prod", runtimeValues["APP_ENV"])
	assert.Equal(t, "https://shop.example.com", runtimeValues["APP_URL"])
	assert.Equal(t, "secret", runtimeValues["APP_SECRET"])
	assert.Equal(t, "mysql://user:p%40ss@db.example.com:3306/shopware?ssl=true", runtimeValues["DATABASE_URL"])
	installValues, err := godotenv.Read(filepath.Join(root, ".shopware-cli/install.env"))
	require.NoError(t, err)
	assert.Equal(t, "admin", installValues["INSTALL_ADMIN_USERNAME"])
	assert.Equal(t, `pa$$"word\value`, installValues["INSTALL_ADMIN_PASSWORD"])
	assert.Equal(t, "admin@example.com", installValues["INSTALL_ADMIN_EMAIL"])
	for _, file := range []string{filepath.Join(root, "shared/.env.local"), filepath.Join(root, ".shopware-cli/install.env"), filepath.Join(root, ".shopware-cli/initialized")} {
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	state, err = e.initStore.Inspect(t.Context())
	require.NoError(t, err)
	assert.True(t, state.Initialized)
	assert.True(t, state.HasInstallConfig)
	runtimeContent, err = os.ReadFile(filepath.Join(root, "shared/.env.local"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256(runtimeContent)), state.RuntimeSHA256)
	assert.ElementsMatch(t, []string{"INSTALL_ADMIN_EMAIL", "INSTALL_ADMIN_PASSWORD", "INSTALL_ADMIN_USERNAME"}, state.InstallKeys)
}

// TestSSHDeploymentInitializationPreservesEscapesWhenReplacingValues guards against replacement-string interpretation of dotenv escapes.
func TestSSHDeploymentInitializationPreservesEscapesWhenReplacingValues(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	runtimeFile := filepath.Join(root, "shared/.env.local")
	require.NoError(t, os.WriteFile(runtimeFile, []byte("DATABASE_URL=old\n"), 0o600))
	value := `mysql://user:a\nb$HOME${USER}$1\1"quoted"@localhost/shopware`
	require.NoError(t, e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"DATABASE_URL": value},
	}))
	values, err := envfile.ReadValues(filepath.Join(root, "shared"), "DATABASE_URL")
	require.NoError(t, err)
	assert.Equal(t, value, values["DATABASE_URL"])
}

// TestSSHDeploymentInitializationRejectsSharedDirectorySymlink checks that initialization cannot write through a shared-directory symlink.
func TestSSHDeploymentInitializationRejectsSharedDirectorySymlink(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	shared := filepath.Join(root, "shared")
	outside := filepath.Join(t.TempDir(), "shared")
	require.NoError(t, os.Rename(shared, outside))
	require.NoError(t, os.Symlink(outside, shared))
	_, err := e.initStore.Inspect(t.Context())
	require.ErrorContains(t, err, "Expected a real directory")
	err = e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_SECRET": "must-not-write"},
	})
	require.ErrorContains(t, err, "Expected a real directory")
}

// TestSSHDeploymentInitializationInspectBootstrapsOnlyManagement checks that inspection creates management storage without runtime configuration.
func TestSSHDeploymentInitializationInspectBootstrapsOnlyManagement(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	require.NoError(t, os.RemoveAll(root))
	state, err := e.initStore.Inspect(t.Context())
	require.NoError(t, err)
	assert.False(t, state.HasSharedDirectory)
	assert.False(t, state.HasRuntimeConfig)
	assert.Empty(t, state.RuntimeSHA256)
	assert.DirExists(t, filepath.Join(root, ".shopware-cli"))
	assert.NoDirExists(t, filepath.Join(root, "shared"))
	assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/install.env"))
}

// TestSSHDeploymentInitializationPreservesUnrelatedRuntimeValues checks that updating a key retains unrelated settings and comments.
func TestSSHDeploymentInitializationPreservesUnrelatedRuntimeValues(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	runtimeFile := filepath.Join(root, "shared/.env.local")
	require.NoError(t, os.WriteFile(runtimeFile, []byte("# keep this comment\nCUSTOM_VALUE=keep\nAPP_URL=http://old.example.com\n"), 0o644))
	require.NoError(t, e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://new.example.com"},
	}))
	content, err := os.ReadFile(runtimeFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# keep this comment")
	assert.Contains(t, string(content), "CUSTOM_VALUE=keep")
	assert.Contains(t, string(content), `APP_URL="https://new.example.com"`)
	assert.NotContains(t, string(content), "old.example.com")
}

// TestSSHDeploymentInitializationRejectsUnsafeTargets checks that config paths cannot be directories or symlinks.
func TestSSHDeploymentInitializationRejectsUnsafeTargets(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	require.NoError(t, os.Remove(filepath.Join(root, "shared/.env.local")))
	require.NoError(t, os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "shared/.env.local")))
	_, err := e.initStore.Inspect(t.Context())
	require.ErrorContains(t, err, "regular file")
	err = e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
	})
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.Remove(filepath.Join(root, "shared/.env.local")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "shared/.env.local"), 0o755))
	_, err = e.initStore.Inspect(t.Context())
	require.ErrorContains(t, err, "regular file")
	err = e.initStore.Apply(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
	})
	require.ErrorContains(t, err, "regular file")
}

// TestSSHDeploymentInitializationRejectsEmptyConfiguration checks that an empty configuration cannot be applied.
func TestSSHDeploymentInitializationRejectsEmptyConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	require.ErrorContains(t, e.initStore.Apply(t.Context(), sshDeploymentInitConfig{}), "no values")
}

// TestSSHInitializationUsesConfiguredActiveLink checks that initialization respects custom active links without modifying releases.
func TestSSHInitializationUsesConfiguredActiveLink(t *testing.T) {
	for _, name := range []string{"current", "shop", "live's shop"} {
		t.Run(name, func(t *testing.T) {
			backend := localDeploymentSSHWithLink(t, name)
			state, err := backend.initStore.Inspect(t.Context())
			require.NoError(t, err)
			assert.False(t, state.HasCurrent)
			root := filepath.Dir(backend.directory)
			release := filepath.Join(root, "releases", "existing")
			require.NoError(t, os.MkdirAll(release, 0o755))
			require.NoError(t, os.Symlink(release, backend.directory))
			state, err = backend.initStore.Inspect(t.Context())
			require.NoError(t, err)
			assert.True(t, state.HasCurrent)
			require.NoError(t, backend.initStore.Apply(t.Context(), sshDeploymentInitConfig{
				RuntimeValues: map[string]string{"APP_ENV": "prod"},
			}))
			target, err := os.Readlink(backend.directory)
			require.NoError(t, err)
			assert.Equal(t, release, target)
			assert.NoFileExists(t, filepath.Join(release, ".env.local"))
			values, err := envfile.ReadValues(filepath.Join(root, "shared"), "APP_ENV")
			require.NoError(t, err)
			assert.Equal(t, "prod", values["APP_ENV"])
		})
	}
}

// TestSSHInitializationApplyDoesNotOverwriteCompletedConfiguration checks that a second writer cannot replace completed initialization.
func TestSSHInitializationApplyDoesNotOverwriteCompletedConfiguration(t *testing.T) {
	backend := localDeploymentSSH(t)
	config := sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_SECRET": "original"},
		InstallValues: map[string]string{"INSTALL_ADMIN_PASSWORD": "original"},
	}
	require.NoError(t, backend.initStore.Apply(t.Context(), config))
	root := filepath.Dir(backend.directory)
	before := make(map[string][]byte)
	for _, name := range []string{"shared/.env.local", ".shopware-cli/install.env", ".shopware-cli/initialized"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		before[name] = content
	}
	// The server must also protect against two clients that inspected before either wrote.
	config.RuntimeValues["APP_SECRET"] = "replacement"
	config.InstallValues["INSTALL_ADMIN_PASSWORD"] = "replacement"
	require.NoError(t, backend.initStore.Apply(t.Context(), config))
	for name, content := range before {
		after, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		assert.Equal(t, content, after)
	}
}

// TestSSHInitializationFailedWriteCanBeRetried checks that a failed write leaves initialization retryable.
func TestSSHInitializationFailedWriteCanBeRetried(t *testing.T) {
	backend := localDeploymentSSH(t)
	root := filepath.Dir(backend.directory)
	install := filepath.Join(root, ".shopware-cli/install.env")
	require.NoError(t, os.MkdirAll(install, 0o700))
	config := sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_ENV": "prod"},
		InstallValues: map[string]string{"INSTALL_ADMIN_USERNAME": "admin"},
	}
	require.Error(t, backend.initStore.Apply(t.Context(), config))
	assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/initialized"))
	require.NoError(t, os.Remove(install))
	state, err := backend.initStore.Inspect(t.Context())
	require.NoError(t, err)
	assert.False(t, state.Initialized)
	require.NoError(t, backend.initStore.Apply(t.Context(), config))
	state, err = backend.initStore.Inspect(t.Context())
	require.NoError(t, err)
	assert.True(t, state.Initialized)
}

// TestSSHInitializationRejectsUnsafeCompletionMarker checks that unsafe completion markers prevent configuration changes.
func TestSSHInitializationRejectsUnsafeCompletionMarker(t *testing.T) {
	for _, symlink := range []bool{true, false} {
		t.Run(fmt.Sprintf("symlink=%t", symlink), func(t *testing.T) {
			backend := localDeploymentSSH(t)
			root := filepath.Dir(backend.directory)
			marker := filepath.Join(root, ".shopware-cli/initialized")
			require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o700))
			if symlink {
				require.NoError(t, os.Symlink(filepath.Join(root, "outside"), marker))
			} else {
				require.NoError(t, os.Mkdir(marker, 0o700))
			}
			before, err := os.ReadFile(filepath.Join(root, "shared/.env.local"))
			require.NoError(t, err)
			_, err = backend.initStore.Inspect(t.Context())
			require.Error(t, err)
			require.Error(t, backend.initStore.Apply(t.Context(), sshDeploymentInitConfig{
				RuntimeValues: map[string]string{"APP_ENV": "prod"},
			}))
			after, err := os.ReadFile(filepath.Join(root, "shared/.env.local"))
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
