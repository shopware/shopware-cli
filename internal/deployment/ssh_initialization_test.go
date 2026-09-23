package deployment

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/envfile"
)

func TestSSHDeploymentInitializationWritesProtectedConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	argumentsLog := filepath.Join(t.TempDir(), "ssh-arguments")
	t.Setenv("SSH_ARGS_LOG", argumentsLog)
	state, err := e.inspectDeploymentInitialization(t.Context())
	require.NoError(t, err)
	assert.False(t, state.HasCurrent)
	assert.True(t, state.HasRuntimeConfig)
	assert.False(t, state.HasInstallConfig)
	assert.True(t, slices.Contains(state.RuntimeKeys, "DATABASE_URL"))

	err = e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
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
	for _, file := range []string{filepath.Join(root, "shared/.env.local"), filepath.Join(root, ".shopware-cli/install.env")} {
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	state, err = e.inspectDeploymentInitialization(t.Context())
	require.NoError(t, err)
	assert.True(t, state.HasInstallConfig)
	assert.ElementsMatch(t, []string{"INSTALL_ADMIN_EMAIL", "INSTALL_ADMIN_PASSWORD", "INSTALL_ADMIN_USERNAME"}, state.InstallKeys)
}

func TestSSHDeploymentInitializationPreservesUnrelatedRuntimeValues(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	runtimeFile := filepath.Join(root, "shared/.env.local")
	require.NoError(t, os.WriteFile(runtimeFile, []byte("# keep this comment\nCUSTOM_VALUE=keep\nAPP_URL=http://old.example.com\n"), 0o644))
	require.NoError(t, e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://new.example.com"},
	}))
	content, err := os.ReadFile(runtimeFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# keep this comment")
	assert.Contains(t, string(content), "CUSTOM_VALUE=keep")
	assert.Contains(t, string(content), `APP_URL="https://new.example.com"`)
	assert.NotContains(t, string(content), "old.example.com")
}

func TestSSHDeploymentInitializationRejectsUnsafeTargets(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	require.NoError(t, os.Remove(filepath.Join(root, "shared/.env.local")))
	require.NoError(t, os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "shared/.env.local")))
	_, err := e.inspectDeploymentInitialization(t.Context())
	require.ErrorContains(t, err, "regular file")
	err = e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
	})
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.Remove(filepath.Join(root, "shared/.env.local")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "shared/.env.local"), 0o755))
	_, err = e.inspectDeploymentInitialization(t.Context())
	require.ErrorContains(t, err, "regular file")
	err = e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
	})
	require.ErrorContains(t, err, "regular file")
}

func TestSSHDeploymentInitializationRejectsEmptyConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	require.ErrorContains(t, e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{}), "no values")
}

func TestSSHRolloutConsumesInstallationEnvironmentAfterSuccessfulHelper(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	require.NoError(t, e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
		InstallValues: map[string]string{
			"INSTALL_ADMIN_USERNAME": "configured-admin",
			"INSTALL_ADMIN_PASSWORD": `pa$$"word\value`,
		},
	}))
	archive := deploymentTestArchive(t, `
$root = dirname(getcwd(), 2);
if (getenv('INSTALL_ADMIN_USERNAME') !== 'configured-admin'
    || getenv('INSTALL_ADMIN_PASSWORD') !== 'pa$$"word\\value'
    || !is_file("$root/.shopware-cli/install.env")) {
    exit(9);
}`)
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/install.env"))
}

func TestSSHRolloutRetainsInstallationEnvironmentWhenHelperFails(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	require.NoError(t, e.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
		InstallValues: map[string]string{"INSTALL_ADMIN_PASSWORD": "configured-password"},
	}))
	archive := deploymentTestArchive(t, "exit(42);")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(root, ".shopware-cli/install.env"))
}
