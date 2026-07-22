package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShopwareProjectScaffold(t *testing.T) {
	t.Parallel()

	t.Run("creates the base project structure", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "shop")
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:       projectFolder,
			Version:             "6.6.10.0",
			DeploymentMethod:    DeploymentNone,
			CISystem:            CINone,
			UseElasticsearch:    true,
			UseAMQP:             true,
			NoAudit:             true,
			SymfonyCLIInstalled: true,
		}

		require.NoError(t, scaffold.Scaffold(t.Context()))

		assert.FileExists(t, filepath.Join(projectFolder, "composer.json"))
		assert.FileExists(t, filepath.Join(projectFolder, ".env"))
		assert.FileExists(t, filepath.Join(projectFolder, ".env.local"))
		assert.FileExists(t, filepath.Join(projectFolder, ".gitignore"))
		assert.FileExists(t, filepath.Join(projectFolder, "php.ini"))
		assert.DirExists(t, filepath.Join(projectFolder, "custom", "plugins"))
		assert.DirExists(t, filepath.Join(projectFolder, "custom", "static-plugins"))

		composerJSON := readScaffoldFile(t, projectFolder, "composer.json")
		assert.Contains(t, composerJSON, `"shopware/core": "6.6.10.0"`)
		assert.Contains(t, composerJSON, `"shopware/elasticsearch": "*"`)
		assert.Contains(t, composerJSON, `"symfony/amqp-messenger": "*"`)
		assert.Contains(t, composerJSON, `"block-insecure": false`)
		assert.Empty(t, readScaffoldFile(t, projectFolder, ".env"))
		assert.Empty(t, readScaffoldFile(t, projectFolder, ".env.local"))
		assert.Equal(t, "/.idea\n/.shopware-cli\n/vendor", readScaffoldFile(t, projectFolder, ".gitignore"))
		assert.Equal(t, "memory_limit=512M", readScaffoldFile(t, projectFolder, "php.ini"))
	})

	t.Run("configures Docker without Symfony CLI php.ini", func(t *testing.T) {
		t.Parallel()

		projectFolder := t.TempDir()
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:       projectFolder,
			Version:             "6.6.10.0",
			DeploymentMethod:    DeploymentNone,
			CISystem:            CINone,
			UseDocker:           true,
			SymfonyCLIInstalled: true,
		}

		require.NoError(t, scaffold.Scaffold(t.Context()))

		assert.Equal(t, "APP_ENV=dev\n", readScaffoldFile(t, projectFolder, ".env.local"))
		assert.NoFileExists(t, filepath.Join(projectFolder, "php.ini"))
	})

	t.Run("creates the selected deployment and CI files", func(t *testing.T) {
		t.Parallel()

		projectFolder := t.TempDir()
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:    projectFolder,
			Version:          "6.6.10.0",
			DeploymentMethod: DeploymentDeployer,
			CISystem:         CIGitHub,
		}

		require.NoError(t, scaffold.Scaffold(t.Context()))

		assert.FileExists(t, filepath.Join(projectFolder, "deploy.php"))
		assert.FileExists(t, filepath.Join(projectFolder, ".github", "workflows", "ci.yml"))
		assert.FileExists(t, filepath.Join(projectFolder, ".github", "workflows", "deploy.yml"))
	})

	t.Run("returns an error when the project path is a file", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "shop")
		require.NoError(t, os.WriteFile(projectFolder, []byte("occupied"), 0o600))

		scaffold := ShopwareProjectScaffold{
			ProjectFolder: projectFolder,
			Version:       "6.6.10.0",
		}

		assert.Error(t, scaffold.Scaffold(t.Context()))
	})

	t.Run("does not overwrite a non-empty project", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "shop")
		require.NoError(t, os.Mkdir(projectFolder, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(projectFolder, "existing"), []byte("keep"), 0o600))

		scaffold := ShopwareProjectScaffold{
			ProjectFolder: projectFolder,
			Version:       "6.6.10.0",
		}

		assert.ErrorContains(t, scaffold.Scaffold(t.Context()), "not empty")
		assert.NoFileExists(t, filepath.Join(projectFolder, "composer.json"))
		assert.Equal(t, "keep", readScaffoldFile(t, projectFolder, "existing"))
	})
}

func TestSetupDeployment(t *testing.T) {
	t.Parallel()

	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupDeployment(tmpDir, DeploymentNone))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("deployer creates deploy.php", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupDeployment(tmpDir, DeploymentDeployer))

		assert.FileExists(t, filepath.Join(tmpDir, "deploy.php"))
		assert.Equal(t, deployerTemplate, readScaffoldFile(t, tmpDir, "deploy.php"))
	})

	t.Run("shopware-paas creates application.yaml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupDeployment(tmpDir, DeploymentShopwarePaaS))

		content := readScaffoldFile(t, tmpDir, "application.yaml")
		assert.Contains(t, content, "php:")
		assert.Contains(t, content, "mysql:")
	})

	t.Run("platformsh creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupDeployment(tmpDir, DeploymentPlatformSH))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestSetupCI(t *testing.T) {
	t.Parallel()

	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CINone, DeploymentNone))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("github creates the validation workflow", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentNone))

		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.NoFileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("github with deployer creates the deploy workflow", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentDeployer))

		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("gitlab creates the validation pipeline", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentNone))

		content := readScaffoldFile(t, tmpDir, ".gitlab-ci.yml")
		assert.Contains(t, content, "project-validate")
		assert.NotContains(t, content, "project-deploy")
	})

	t.Run("gitlab with deployer includes the deploy pipeline", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentDeployer))

		content := readScaffoldFile(t, tmpDir, ".gitlab-ci.yml")
		assert.Contains(t, content, "project-deploy")
	})
}

func readScaffoldFile(t *testing.T, root string, path ...string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
	require.NoError(t, err)

	return string(content)
}
