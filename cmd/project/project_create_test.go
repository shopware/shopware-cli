package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	versions := []*version.Version{
		version.Must(version.NewVersion("6.6.1.0-rc1")),
		version.Must(version.NewVersion("6.6.0.0")),
		version.Must(version.NewVersion("6.5.8.0")),
		version.Must(version.NewVersion("6.5.7.0")),
	}

	t.Run("latest selects most recent stable version", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion(versionLatest, versions)
		assert.Equal(t, "6.6.0.0", result)
	})

	t.Run("latest falls back to RC if no stable", func(t *testing.T) {
		t.Parallel()
		rcOnly := []*version.Version{
			version.Must(version.NewVersion("6.7.0.0-rc2")),
			version.Must(version.NewVersion("6.7.0.0-rc1")),
		}
		result := resolveVersion(versionLatest, rcOnly)
		assert.Equal(t, "6.7.0.0-rc2", result)
	})

	t.Run("latest returns empty for empty list", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion(versionLatest, []*version.Version{})
		assert.Equal(t, "", result)
	})

	t.Run("exact version match", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion("6.5.8.0", versions)
		assert.Equal(t, "6.5.8.0", result)
	})

	t.Run("version not found returns empty", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion("6.4.0.0", versions)
		assert.Equal(t, "", result)
	})

	t.Run("dev version passes through", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion("dev-trunk", versions)
		assert.Equal(t, "dev-trunk", result)
	})

	t.Run("dev version with branch name", func(t *testing.T) {
		t.Parallel()
		result := resolveVersion("dev-6.6", versions)
		assert.Equal(t, "dev-6.6", result)
	})
}

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	validNames := []string{
		"my-shopware-project",
		"myshop",
		"my_shop",
		"shop123",
		"123shop",
		"a",
		"path/to/my-shop",
	}

	for _, name := range validNames {
		t.Run("valid: "+name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateProjectName(name))
		})
	}

	invalidNames := []string{
		"MyShop",
		"myShop",
		"SHOP",
		"müller",
		"über-shop",
		"Müller-Shop",
		"café",
		"straße",
		"my shop",
		"my.shop",
		"shop!",
		"-shop",
		"_shop",
		"ä",
		"",
		"path/to/müller",
		"path/to/MyShop",
	}

	for _, name := range invalidNames {
		t.Run("invalid: "+name, func(t *testing.T) {
			t.Parallel()
			err := validateProjectName(name)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid project name")
		})
	}
}

func TestProjectCreateRejectsInvalidNameArgument(t *testing.T) {
	// A name provided directly as an argument must be rejected up front,
	// before the interactive form or any network call, the same way the
	// interactive name prompt rejects it live.
	invalidNames := []string{"myShop", "MyShop", "müller", "my shop"}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			projectCreateCmd.SetContext(t.Context())
			err := projectCreateCmd.RunE(projectCreateCmd, []string{name})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid project name")
		})
	}
}

func TestProjectNameFieldDescription(t *testing.T) {
	t.Parallel()

	t.Run("empty shows help text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, projectNameHelp, projectNameFieldDescription(""))
	})

	t.Run("valid name shows help text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, projectNameHelp, projectNameFieldDescription("my-shop"))
	})

	t.Run("uppercase name shows the rule", func(t *testing.T) {
		t.Parallel()
		desc := projectNameFieldDescription("MyShop")
		assert.NotEqual(t, projectNameHelp, desc)
		assert.Contains(t, desc, projectNameRule)
	})

	t.Run("umlaut name shows the rule", func(t *testing.T) {
		t.Parallel()
		desc := projectNameFieldDescription("müller")
		assert.NotEqual(t, projectNameHelp, desc)
		assert.Contains(t, desc, projectNameRule)
	})
}

func TestSetupDeployment(t *testing.T) {
	t.Parallel()
	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, packagist.DeploymentNone)
		assert.NoError(t, err)

		entries, err := os.ReadDir(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("deployer creates deploy.php", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, packagist.DeploymentDeployer)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, "deploy.php"))
		content, err := os.ReadFile(filepath.Join(tmpDir, "deploy.php"))
		assert.NoError(t, err)
		assert.Equal(t, deployerTemplate, string(content))
	})

	t.Run("shopware-paas creates application.yaml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, packagist.DeploymentShopwarePaaS)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, "application.yaml"))
		content, err := os.ReadFile(filepath.Join(tmpDir, "application.yaml"))
		assert.NoError(t, err)
		assert.Contains(t, string(content), "php:")
		assert.Contains(t, string(content), "mysql:")
	})

	t.Run("platformsh creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, packagist.DeploymentPlatformSH)
		assert.NoError(t, err)

		entries, err := os.ReadDir(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestSetupCI(t *testing.T) {
	t.Parallel()
	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "none", packagist.DeploymentNone)
		assert.NoError(t, err)

		entries, err := os.ReadDir(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("github creates workflow directory and ci.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "github", packagist.DeploymentNone)
		assert.NoError(t, err)

		assert.DirExists(t, filepath.Join(tmpDir, ".github", "workflows"))
		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.NoFileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("github with deployer creates deploy.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "github", packagist.DeploymentDeployer)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("gitlab creates .gitlab-ci.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "gitlab", packagist.DeploymentNone)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, ".gitlab-ci.yml"))
	})

	t.Run("gitlab with deployer includes deploy config", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "gitlab", packagist.DeploymentDeployer)
		assert.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(tmpDir, ".gitlab-ci.yml"))
		assert.NoError(t, err)
		assert.Contains(t, string(content), "deploy")
	})
}

func TestValidDeploymentMethods(t *testing.T) {
	t.Parallel()
	validDeployments := map[string]bool{
		packagist.DeploymentNone:         true,
		packagist.DeploymentDeployer:     true,
		packagist.DeploymentPlatformSH:   true,
		packagist.DeploymentShopwarePaaS: true,
	}

	t.Run("all deployment constants are valid", func(t *testing.T) {
		t.Parallel()
		assert.True(t, validDeployments[packagist.DeploymentNone])
		assert.True(t, validDeployments[packagist.DeploymentDeployer])
		assert.True(t, validDeployments[packagist.DeploymentPlatformSH])
		assert.True(t, validDeployments[packagist.DeploymentShopwarePaaS])
	})

	t.Run("invalid deployment is rejected", func(t *testing.T) {
		t.Parallel()
		assert.False(t, validDeployments["invalid"])
		assert.False(t, validDeployments[""])
	})
}

func TestValidCISystems(t *testing.T) {
	t.Parallel()
	validCISystems := map[string]bool{
		"none":   true,
		"github": true,
		"gitlab": true,
	}

	t.Run("all CI constants are valid", func(t *testing.T) {
		t.Parallel()
		assert.True(t, validCISystems["none"])
		assert.True(t, validCISystems["github"])
		assert.True(t, validCISystems["gitlab"])
	})

	t.Run("invalid CI system is rejected", func(t *testing.T) {
		t.Parallel()
		assert.False(t, validCISystems["jenkins"])
		assert.False(t, validCISystems[""])
	})
}
