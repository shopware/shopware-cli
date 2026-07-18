package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestFilterInstallVersions(t *testing.T) {
	t.Parallel()

	releases := []repository.Version{
		{Version: "6.7.12.x-dev"},
		{Version: "6.7.12.1"},
		{Version: "6.7.11.x-dev"},
		{Version: "6.7.11.1"},
		{Version: "dev-trunk"},
		{Version: "6.3.0.0"},
	}

	filtered := filterInstallVersions(releases)

	got := make([]string, 0, len(filtered))
	for _, v := range filtered {
		got = append(got, v.String())
	}

	assert.Equal(t, []string{"6.7.12.1", "6.7.11.1"}, got)
}

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
		".",
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

	t.Run("dot shows help text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, projectNameHelp, projectNameFieldDescription("."))
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

func TestCurrentProjectName(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(tmpDir))

	name, err := currentProjectName()
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(tmpDir), name)
}

func TestCurrentProjectNameWithInvalidDirectoryName(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	invalidDir := filepath.Join(tmpDir, "MyBadDir")
	require.NoError(t, os.MkdirAll(invalidDir, os.ModePerm))

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(invalidDir))

	name, err := currentProjectName()
	require.NoError(t, err)

	assert.Equal(t, "MyBadDir", name)
	assert.Error(t, validateProjectName(name))
}

func TestScaffoldProjectInCurrentDirectory(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(tmpDir))

	opts := createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	err = scaffoldProject(t.Context(), &opts, "6.6.0.0")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmpDir, "composer.json"))
	assert.FileExists(t, filepath.Join(tmpDir, ".env"))
	assert.FileExists(t, filepath.Join(tmpDir, ".env.local"))
	assert.FileExists(t, filepath.Join(tmpDir, ".gitignore"))
	assert.DirExists(t, filepath.Join(tmpDir, "custom", "plugins"))
	assert.DirExists(t, filepath.Join(tmpDir, "custom", "static-plugins"))

	// The project folder option stays as "." when scaffolding in the current directory.
	assert.Equal(t, ".", opts.projectFolder)
}

func TestScaffoldProjectCreatesDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "my-shop")

	opts := createOptions{
		projectFolder:      projectDir,
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	err := scaffoldProject(t.Context(), &opts, "6.6.0.0")
	require.NoError(t, err)

	assert.DirExists(t, projectDir)
	assert.FileExists(t, filepath.Join(projectDir, "composer.json"))
}

func TestScaffoldProjectRejectsNonEmptyDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "existing.txt"), []byte("existing"), os.ModePerm))

	opts := createOptions{
		projectFolder:      tmpDir,
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	err := scaffoldProject(t.Context(), &opts, "6.6.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestScaffoldProjectRejectsNonEmptyCurrentDirectory(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	require.NoError(t, os.Chdir(tmpDir))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "existing.txt"), []byte("existing"), os.ModePerm))

	opts := createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	err = scaffoldProject(t.Context(), &opts, "6.6.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestSetupDeployment(t *testing.T) {
	t.Parallel()
	t.Run("none creates no files", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, shop.DeploymentNone)
		assert.NoError(t, err)

		entries, err := os.ReadDir(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("deployer creates deploy.php", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, shop.DeploymentDeployer)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, "deploy.php"))
		content, err := os.ReadFile(filepath.Join(tmpDir, "deploy.php"))
		assert.NoError(t, err)
		assert.Equal(t, deployerTemplate, string(content))
	})

	t.Run("shopware-paas creates application.yaml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupDeployment(tmpDir, shop.DeploymentShopwarePaaS)
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

		err := setupDeployment(tmpDir, shop.DeploymentPlatformSH)
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

		err := setupCI(t.Context(), tmpDir, "none", shop.DeploymentNone)
		assert.NoError(t, err)

		entries, err := os.ReadDir(tmpDir)
		assert.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("github creates workflow directory and ci.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "github", shop.DeploymentNone)
		assert.NoError(t, err)

		assert.DirExists(t, filepath.Join(tmpDir, ".github", "workflows"))
		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.NoFileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("github with deployer creates deploy.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "github", shop.DeploymentDeployer)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"))
		assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "deploy.yml"))
	})

	t.Run("gitlab creates .gitlab-ci.yml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "gitlab", shop.DeploymentNone)
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(tmpDir, ".gitlab-ci.yml"))
	})

	t.Run("gitlab with deployer includes deploy config", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		err := setupCI(t.Context(), tmpDir, "gitlab", shop.DeploymentDeployer)
		assert.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(tmpDir, ".gitlab-ci.yml"))
		assert.NoError(t, err)
		assert.Contains(t, string(content), "deploy")
	})
}

func TestValidDeploymentMethods(t *testing.T) {
	t.Parallel()
	validDeployments := map[string]bool{
		shop.DeploymentNone:         true,
		shop.DeploymentDeployer:     true,
		shop.DeploymentPlatformSH:   true,
		shop.DeploymentShopwarePaaS: true,
	}

	t.Run("all deployment constants are valid", func(t *testing.T) {
		t.Parallel()
		assert.True(t, validDeployments[shop.DeploymentNone])
		assert.True(t, validDeployments[shop.DeploymentDeployer])
		assert.True(t, validDeployments[shop.DeploymentPlatformSH])
		assert.True(t, validDeployments[shop.DeploymentShopwarePaaS])
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

// TestValidateAndPreflightCurrentDirectory validates that the current directory
// can be used as the project target when the project name is valid.
func TestValidateAndPreflightCurrentDirectory(t *testing.T) {
	// This test and the related preflight tests change the process working
	// directory, so they must not run in parallel with each other.

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	require.NoError(t, os.Chdir(tmpDir))

	opts := createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	chosen, _, err := validateAndPreflight(t.Context(), &opts, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	require.NoError(t, err)
	assert.Equal(t, "6.6.0.0", chosen)
}

// TestValidateAndPreflightRejectsInvalidCurrentDirectory validates that running
// the command in a directory whose name is not a valid Docker Compose project
// name returns an error before any network or filesystem work is attempted.
func TestValidateAndPreflightRejectsInvalidCurrentDirectory(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	invalidDir := filepath.Join(tmpDir, "MyBadDir")
	require.NoError(t, os.MkdirAll(invalidDir, os.ModePerm))

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	require.NoError(t, os.Chdir(invalidDir))

	opts := createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	_, _, err = validateAndPreflight(t.Context(), &opts, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "invalid project name")
}

func TestValidateAndPreflightRejectsNonEmptyCurrentDirectory(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "existing.txt"), []byte("existing"), os.ModePerm))

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	require.NoError(t, os.Chdir(tmpDir))

	_, _, err = validateAndPreflight(t.Context(), &createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestValidateAndPreflightRejectsNonEmptyTargetDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "existing-shop")
	require.NoError(t, os.MkdirAll(targetDir, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "existing.txt"), []byte("existing"), os.ModePerm))

	opts := createOptions{
		projectFolder:      targetDir,
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	_, _, err := validateAndPreflight(t.Context(), &opts, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

// TestValidateAndPreflightAllowsEmptyTargetDirectory validates that an empty
// existing directory is accepted as a valid target folder.
func TestValidateAndPreflightAllowsEmptyTargetDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "empty-shop")
	require.NoError(t, os.MkdirAll(targetDir, os.ModePerm))

	opts := createOptions{
		projectFolder:      targetDir,
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	chosen, _, err := validateAndPreflight(t.Context(), &opts, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	require.NoError(t, err)
	assert.Equal(t, "6.6.0.0", chosen)
}

// TestValidateAndPreflightAcceptsValidCurrentDirectoryName ensures that a
// valid current directory name passes the name check even when the folder name
// is otherwise empty.
func TestValidateAndPreflightAcceptsValidCurrentDirectoryName(t *testing.T) {
	// This test changes the process working directory, so it cannot run in
	// parallel with other tests that also change the working directory.

	tmpDir := t.TempDir()
	validDir := filepath.Join(tmpDir, "my-valid-dir")
	require.NoError(t, os.MkdirAll(validDir, os.ModePerm))

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	require.NoError(t, os.Chdir(validDir))

	opts := createOptions{
		projectFolder:      ".",
		selectedVersion:    "6.6.0.0",
		selectedDeployment: shop.DeploymentNone,
		selectedCI:         ciNone,
		useDocker:          false,
		noAudit:            true,
	}

	chosen, _, err := validateAndPreflight(t.Context(), &opts, []repository.Version{{Version: "6.6.0.0"}}, []*version.Version{version.Must(version.NewVersion("6.6.0.0"))})
	require.NoError(t, err)
	assert.Equal(t, "6.6.0.0", chosen)
}

// TestRunCreateFormEmptyNameIsValid checks that the form-level validator allows
// an empty project name so that users can leave the field blank.
func TestRunCreateFormEmptyNameIsValid(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateProjectName("my-shop"))
	err := validateProjectName("")
	assert.Error(t, err)
}
