package shop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/symfony"
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
		assertScaffoldSecurityPlugin(t, composerJSON)
		assert.Empty(t, readScaffoldFile(t, projectFolder, ".env"))
		assert.Empty(t, readScaffoldFile(t, projectFolder, ".env.local"))
		assert.Equal(t, "/.idea\n/.shopware-cli\n/vendor", readScaffoldFile(t, projectFolder, ".gitignore"))
		assert.Equal(t, "memory_limit=512M", readScaffoldFile(t, projectFolder, "php.ini"))
	})

	t.Run("creates a Dockerfile for a container deployment", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "shop")
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:    projectFolder,
			Version:          "6.7.12.1",
			DeploymentMethod: DeploymentContainer,
			CISystem:         CINone,
			UseDocker:        true,
			PHPVersion:       "8.4",
		}

		require.NoError(t, scaffold.Scaffold(t.Context()))

		assert.FileExists(t, filepath.Join(projectFolder, "Dockerfile"))
		assert.FileExists(t, filepath.Join(projectFolder, ".dockerignore"))
		assert.Contains(t, readScaffoldFile(t, projectFolder, "Dockerfile"), "ARG PHP_VERSION=8.4")
		// The container deployment needs no deployment-specific composer package.
		composerJSON := readScaffoldFile(t, projectFolder, "composer.json")
		assertScaffoldSecurityPlugin(t, composerJSON)
		assert.NotContains(t, composerJSON, "deployer/deployer")
		assert.NotContains(t, composerJSON, "shopware/paas-meta")
		assert.NotContains(t, composerJSON, "shopware/k8s-meta")
	})

	t.Run("configures Docker without Symfony CLI php.ini", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "demo-shop")
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

		envContent := readScaffoldFile(t, projectFolder, ".env")
		assert.True(t, strings.HasPrefix(envContent, envfile.ComposeProjectNameEnvKey+"=sw-demo-shop-"), "got %q", envContent)
		assert.True(t, strings.HasSuffix(envContent, "\n"))
		name := strings.TrimPrefix(strings.TrimSpace(envContent), envfile.ComposeProjectNameEnvKey+"=")
		assert.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`), name)

		// Two docker scaffolds with the same basename get different compose names.
		projectFolder2 := filepath.Join(t.TempDir(), "demo-shop")
		scaffold2 := scaffold
		scaffold2.ProjectFolder = projectFolder2
		require.NoError(t, scaffold2.Scaffold(t.Context()))
		env2 := readScaffoldFile(t, projectFolder2, ".env")
		assert.NotEqual(t, envContent, env2)
	})

	t.Run("disables the admin worker for Docker projects with a working queue worker", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			version   string
			useDocker bool
			enabled   bool
		}{
			{version: "6.7.12.1", useDocker: true, enabled: false},
			{version: "6.6.3.1", useDocker: true, enabled: true},
			{version: "6.7.12.1", useDocker: false, enabled: true},
		}
		for _, tt := range tests {
			projectFolder := t.TempDir()
			scaffold := ShopwareProjectScaffold{
				ProjectFolder:    projectFolder,
				Version:          tt.version,
				DeploymentMethod: DeploymentNone,
				CISystem:         CINone,
				UseDocker:        tt.useDocker,
			}
			require.NoError(t, scaffold.Scaffold(t.Context()))

			for _, env := range []string{"dev", "prod"} {
				enabled, err := symfony.IsAdminWorkerEnabledForProject(projectFolder, env)
				require.NoError(t, err)
				assert.Equal(t, tt.enabled, enabled, "version=%s docker=%v env=%s", tt.version, tt.useDocker, env)
			}
		}
	})

	t.Run("writes an explicit compose project name", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "My Shop")
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:      projectFolder,
			Version:            "6.6.10.0",
			DeploymentMethod:   DeploymentNone,
			CISystem:           CINone,
			UseDocker:          true,
			ComposeProjectName: "acme-live",
		}

		require.NoError(t, scaffold.Scaffold(t.Context()))
		assert.Equal(t, envfile.ComposeProjectNameEnvKey+"=acme-live\n", readScaffoldFile(t, projectFolder, ".env"))
	})

	t.Run("rejects an invalid compose project name before writing", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "shop")
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:      projectFolder,
			Version:            "6.6.10.0",
			DeploymentMethod:   DeploymentNone,
			CISystem:           CINone,
			UseDocker:          true,
			ComposeProjectName: "Acme Live",
		}

		require.Error(t, scaffold.Scaffold(t.Context()))
		_, err := os.Stat(projectFolder)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("non-docker leaves .env empty without compose project name", func(t *testing.T) {
		t.Parallel()

		projectFolder := filepath.Join(t.TempDir(), "plain-shop")
		scaffold := ShopwareProjectScaffold{
			ProjectFolder:    projectFolder,
			Version:          "6.6.10.0",
			DeploymentMethod: DeploymentNone,
			CISystem:         CINone,
			UseDocker:        false,
		}
		require.NoError(t, scaffold.Scaffold(t.Context()))
		assert.Empty(t, readScaffoldFile(t, projectFolder, ".env"))
		assert.NotContains(t, readScaffoldFile(t, projectFolder, ".env"), envfile.ComposeProjectNameEnvKey)
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

func TestShopwareProjectScaffoldRollback(t *testing.T) {
	t.Parallel()

	// DeploymentContainer with an unsupported PHP version only fails in
	// setupDeployment, after composer.json and the base files were written.
	failingScaffold := func(projectFolder string) ShopwareProjectScaffold {
		return ShopwareProjectScaffold{
			ProjectFolder:    projectFolder,
			Version:          "6.6.10.0",
			DeploymentMethod: DeploymentContainer,
			CISystem:         CINone,
			PHPVersion:       "9.9",
		}
	}

	t.Run("removes a newly created folder when scaffolding fails", func(t *testing.T) {
		t.Parallel()
		projectFolder := filepath.Join(t.TempDir(), "shop")
		scaffold := failingScaffold(projectFolder)

		assert.Error(t, scaffold.Scaffold(t.Context()))

		_, err := os.Stat(projectFolder)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("empties but keeps a pre-existing folder when scaffolding fails", func(t *testing.T) {
		t.Parallel()
		projectFolder := t.TempDir()
		scaffold := failingScaffold(projectFolder)

		assert.Error(t, scaffold.Scaffold(t.Context()))

		assert.DirExists(t, projectFolder)
		entries, err := os.ReadDir(projectFolder)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("keeps a non-empty folder untouched when validation fails", func(t *testing.T) {
		t.Parallel()
		projectFolder := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(projectFolder, "existing"), []byte("keep"), 0o600))
		scaffold := failingScaffold(projectFolder)

		assert.Error(t, scaffold.Scaffold(t.Context()))

		assert.Equal(t, "keep", readScaffoldFile(t, projectFolder, "existing"))
	})
}

func TestWriteComposerJsonDisablesAuditBlocking(t *testing.T) {
	t.Parallel()

	projectFolder := t.TempDir()
	scaffold := ShopwareProjectScaffold{
		ProjectFolder: projectFolder,
		Version:       "6.6.10.0",
	}

	require.NoError(t, scaffold.Scaffold(t.Context()))
	assert.NotContains(t, readScaffoldFile(t, projectFolder, "composer.json"), `"block-insecure"`)

	scaffold.NoAudit = true
	require.NoError(t, scaffold.WriteComposerJson(t.Context()))

	composerJSON := readScaffoldFile(t, projectFolder, "composer.json")
	assert.Contains(t, composerJSON, `"block-insecure": false`)
	assert.Contains(t, composerJSON, `"shopware/core": "6.6.10.0"`)
	assertScaffoldSecurityPlugin(t, composerJSON)
}

func assertScaffoldSecurityPlugin(t *testing.T, composerJSON string) {
	t.Helper()

	type repository struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	var manifest struct {
		Require      map[string]string `json:"require"`
		Repositories []repository      `json:"repositories"`
	}
	require.NoError(t, json.Unmarshal([]byte(composerJSON), &manifest))
	assert.Equal(t, ">=1.0.26", manifest.Require["store.shopware.com/swagplatformsecurity"])
	assert.Equal(t, []repository{
		{"path", "custom/plugins/*"},
		{"path", "custom/plugins/*/packages/*"},
		{"path", "custom/static-plugins/*"},
		{"composer", "https://shopware.github.io/conflicts/"},
		{"composer", "https://packages.shopware.com"},
	}, manifest.Repositories)
}

func TestSetupDeployment(t *testing.T) {
	t.Parallel()

	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, ShopwareProjectScaffold{ProjectFolder: tmpDir, DeploymentMethod: DeploymentNone}.setupDeployment(t.Context()))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("deployer creates deploy.php", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, ShopwareProjectScaffold{ProjectFolder: tmpDir, DeploymentMethod: DeploymentDeployer}.setupDeployment(t.Context()))

		assert.FileExists(t, filepath.Join(tmpDir, "deploy.php"))
		assert.Equal(t, deployerTemplate, readScaffoldFile(t, tmpDir, "deploy.php"))
	})

	t.Run("shopware-paas creates application.yaml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, ShopwareProjectScaffold{ProjectFolder: tmpDir, DeploymentMethod: DeploymentShopwarePaaS}.setupDeployment(t.Context()))

		content := readScaffoldFile(t, tmpDir, "application.yaml")
		assert.Contains(t, content, "php:")
		assert.Contains(t, content, "mysql:")
	})

	t.Run("container creates a Dockerfile pinned to the project PHP version", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		scaffold := ShopwareProjectScaffold{ProjectFolder: tmpDir, DeploymentMethod: DeploymentContainer, PHPVersion: "8.3"}
		require.NoError(t, scaffold.setupDeployment(t.Context()))

		dockerfile := readScaffoldFile(t, tmpDir, "Dockerfile")
		assert.Contains(t, dockerfile, "ARG PHP_VERSION=8.3")
		assert.Contains(t, dockerfile, "FROM ghcr.io/shopware/docker-base:${PHP_VERSION}-frankenphp AS base-image")
		assert.Contains(t, dockerfile, "FROM ghcr.io/shopware/shopware-cli:latest-php-${PHP_VERSION} AS shopware-cli")
		assert.Contains(t, dockerfile, "shopware-cli project ci /src")
		assert.NotContains(t, dockerfile, "{{")

		dockerignore := readScaffoldFile(t, tmpDir, ".dockerignore")
		assert.Contains(t, dockerignore, "vendor")
		// A dev-only APP_ENV must never end up in the production image.
		assert.Contains(t, dockerignore, ".env.local")
	})

	t.Run("platformsh creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, ShopwareProjectScaffold{ProjectFolder: tmpDir, DeploymentMethod: DeploymentPlatformSH}.setupDeployment(t.Context()))

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

		require.NoError(t, setupCI(t.Context(), tmpDir, CINone, DeploymentNone, ""))

		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("github creates the validation workflow", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentNone, ""))

		assert.NoFileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))

		var workflow struct {
			Concurrency struct {
				Group string `yaml:"group"`
			} `yaml:"concurrency"`
			Jobs map[string]struct {
				Needs any `yaml:"needs"`
				Steps []struct {
					Uses string `yaml:"uses"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(readScaffoldFile(t, tmpDir, ".github", "workflows", "ci.yml")), &workflow))

		assert.Equal(t, "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}", workflow.Concurrency.Group)
		assert.Contains(t, workflow.Jobs, "check")
		require.Contains(t, workflow.Jobs, "build")
		build := workflow.Jobs["build"]
		assert.Nil(t, build.Needs)

		uses := make([]string, 0, len(build.Steps))
		for _, step := range build.Steps {
			uses = append(uses, strings.Split(step.Uses, "@")[0])
		}
		assert.Contains(t, uses, "shopware/build-project-action")
	})

	t.Run("github with deployer creates the deploy workflow", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentDeployer, ""))

		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))

		var workflow struct {
			Jobs map[string]struct {
				Steps []struct {
					With map[string]string `yaml:"with"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(readScaffoldFile(t, tmpDir, ".github", "workflows", "deploy.yml")), &workflow))
		require.Contains(t, workflow.Jobs, "deploy")
		require.Len(t, workflow.Jobs["deploy"].Steps, 1)
		assert.Equal(t, "${{ secrets.SSH_PRIVATE_KEY }}", workflow.Jobs["deploy"].Steps[0].With["sshPrivateKey"])
	})

	t.Run("github uses the project PHP version", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentDeployer, "8.2"))

		assert.ElementsMatch(t, []string{"8.2", "8.2"}, githubPHPVersions(t, readScaffoldFile(t, tmpDir, ".github", "workflows", "ci.yml")))
		assert.ElementsMatch(t, []string{"8.2"}, githubPHPVersions(t, readScaffoldFile(t, tmpDir, ".github", "workflows", "deploy.yml")))
	})

	t.Run("github falls back to PHP 8.4", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitHub, DeploymentDeployer, ""))

		assert.ElementsMatch(t, []string{"8.4", "8.4"}, githubPHPVersions(t, readScaffoldFile(t, tmpDir, ".github", "workflows", "ci.yml")))
		assert.ElementsMatch(t, []string{"8.4"}, githubPHPVersions(t, readScaffoldFile(t, tmpDir, ".github", "workflows", "deploy.yml")))
	})

	t.Run("gitlab creates the validation pipeline", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentNone, ""))

		content := readScaffoldFile(t, tmpDir, ".gitlab-ci.yml")
		assert.Contains(t, content, "project-validate")
		assert.NotContains(t, content, "project-deploy")
		assertGitLabBuildJob(t, content)
	})

	t.Run("gitlab with deployer includes the deploy pipeline", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentDeployer, ""))

		content := readScaffoldFile(t, tmpDir, ".gitlab-ci.yml")
		assert.Contains(t, content, "project-deploy")
		assertGitLabBuildJob(t, content)
	})

	t.Run("gitlab uses the project PHP version", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentDeployer, "8.2"))

		inputs, image := gitlabPHPVersions(t, readScaffoldFile(t, tmpDir, ".gitlab-ci.yml"))
		assert.ElementsMatch(t, []string{"8.2", "8.2"}, inputs)
		assert.Equal(t, "ghcr.io/shopware/shopware-cli:latest-php-8.2", image)
	})

	t.Run("gitlab falls back to PHP 8.4", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		require.NoError(t, setupCI(t.Context(), tmpDir, CIGitLab, DeploymentNone, ""))

		inputs, image := gitlabPHPVersions(t, readScaffoldFile(t, tmpDir, ".gitlab-ci.yml"))
		assert.ElementsMatch(t, []string{"8.4"}, inputs)
		assert.Equal(t, "ghcr.io/shopware/shopware-cli:latest-php-8.4", image)
	})
}

// githubPHPVersions returns every PHP version a GitHub workflow passes to its actions.
func githubPHPVersions(t *testing.T, content string) []string {
	t.Helper()

	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &workflow))

	var versions []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			for _, key := range []string{"phpVersion", "php-version"} {
				if v, ok := step.With[key]; ok {
					versions = append(versions, v)
				}
			}
		}
	}

	return versions
}

// gitlabPHPVersions returns the php_version inputs of the included components
// and the image of the build job.
func gitlabPHPVersions(t *testing.T, content string) ([]string, string) {
	t.Helper()

	var pipeline struct {
		Include []struct {
			Inputs map[string]string `yaml:"inputs"`
		} `yaml:"include"`
		Build struct {
			Image struct {
				Name string `yaml:"name"`
			} `yaml:"image"`
		} `yaml:"build"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &pipeline))

	versions := make([]string, 0, len(pipeline.Include))
	for _, include := range pipeline.Include {
		versions = append(versions, include.Inputs["php_version"])
	}

	return versions, pipeline.Build.Image.Name
}

func assertGitLabBuildJob(t *testing.T, content string) {
	t.Helper()

	var pipeline struct {
		Build struct {
			Stage  string   `yaml:"stage"`
			Script []string `yaml:"script"`
		} `yaml:"build"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &pipeline))

	assert.Equal(t, "check", pipeline.Build.Stage)
	assert.True(t, slices.ContainsFunc(pipeline.Build.Script, func(line string) bool {
		return strings.Contains(line, "project ci")
	}))
}

func readScaffoldFile(t *testing.T, root string, path ...string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
	require.NoError(t, err)

	return string(content)
}

func TestSupportsDedicatedWorker(t *testing.T) {
	t.Parallel()

	assert.False(t, supportsDedicatedWorker("6.6.3.1"))
	assert.True(t, supportsDedicatedWorker("6.6.4.0"))
	assert.True(t, supportsDedicatedWorker("6.7.12.1"))
	assert.True(t, supportsDedicatedWorker("dev-trunk"))
	assert.False(t, supportsDedicatedWorker("dev-6.5.x"))
}
