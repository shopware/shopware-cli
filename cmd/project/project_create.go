package project

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

//go:embed static/deploy.php
var deployerTemplate string

var projectCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new Shopware 6 project",
	Args:  cobra.MaximumNArgs(1),
	ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{}, cobra.ShellCompDirectiveFilterDirs
		}

		return []string{}, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useDocker, _ := cmd.PersistentFlags().GetBool("docker")
		withElasticsearch, _ := cmd.PersistentFlags().GetBool("with-elasticsearch")
		withoutElasticsearch, _ := cmd.PersistentFlags().GetBool("without-elasticsearch")
		noAudit, _ := cmd.PersistentFlags().GetBool("no-audit")
		initGit, _ := cmd.PersistentFlags().GetBool("git")
		useMinio, _ := cmd.PersistentFlags().GetBool("minio")
		versionFlag, _ := cmd.PersistentFlags().GetString("version")
		deploymentMethod, _ := cmd.PersistentFlags().GetString("deployment")
		ciSystem, _ := cmd.PersistentFlags().GetString("ci")

		// Handle deprecated --without-elasticsearch flag
		if cmd.PersistentFlags().Changed("without-elasticsearch") {
			logging.FromContext(cmd.Context()).Warnf("Flag --without-elasticsearch is deprecated, use --with-elasticsearch instead")
			withElasticsearch = !withoutElasticsearch
		}

		interactive := system.IsInteractionEnabled(cmd.Context())

		// Fetch versions early as we need them for validation and selection
		filteredVersions, err := getFilteredInstallVersions(cmd.Context())
		if err != nil {
			return err
		}

		versionOptions := make([]huh.Option[string], 0, len(filteredVersions)+1)
		versionOptions = append(versionOptions, huh.NewOption("latest", "latest"))
		for _, v := range filteredVersions {
			versionStr := v.String()
			versionOptions = append(versionOptions, huh.NewOption(versionStr, versionStr))
		}

		deploymentOptions := []huh.Option[string]{
			huh.NewOption("None", packagist.DeploymentNone),
			huh.NewOption("DeployerPHP", packagist.DeploymentDeployer),
			huh.NewOption("PaaS powered by Platform.sh", packagist.DeploymentPlatformSH),
			huh.NewOption("PaaS powered by Shopware", packagist.DeploymentShopwarePaaS),
		}

		const (
			optionDocker        = "docker"
			optionGit           = "git"
			optionElasticsearch = "elasticsearch"
			optionMinio         = "minio"

			ciNone   = "none"
			ciGitHub = "github"
			ciGitLab = "gitlab"
		)

		ciOptions := []huh.Option[string]{
			huh.NewOption("None", ciNone),
			huh.NewOption("GitHub Actions", ciGitHub),
			huh.NewOption("GitLab CI", ciGitLab),
		}

		var projectFolder string
		selectedVersion := versionFlag
		selectedDeployment := deploymentMethod
		selectedCI := ciSystem
		selectedOptions := []string{optionDocker, optionGit}

		// Get project name from args or prompt
		if len(args) > 0 {
			projectFolder = args[0]
		}

		// Set defaults for non-interactive mode
		if !interactive {
			if projectFolder == "" {
				return fmt.Errorf("project name is required in non-interactive mode")
			}
			if selectedVersion == "" {
				selectedVersion = "latest"
			}
			if selectedDeployment == "" {
				selectedDeployment = packagist.DeploymentNone
			}
			if selectedCI == "" {
				selectedCI = ciNone
			}
		} else {
			var formFields []huh.Field

			// Project name input (if not provided)
			if projectFolder == "" {
				formFields = append(formFields,
					huh.NewInput().
						Title("Project Name").
						Description("The name of the project directory to create").
						Placeholder("my-shopware-project").
						Value(&projectFolder).
						Validate(func(s string) error {
							if s == "" {
								return fmt.Errorf("project name is required")
							}
							return nil
						}),
				)
			}

			// Version selection (if not provided via flag)
			if selectedVersion == "" {
				selectedVersion = "latest" // default selection
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("Shopware Version").
						Description("Select the Shopware version to install").
						Options(versionOptions...).
						Height(10).
						Value(&selectedVersion),
				)
			}

			// Deployment method (if not provided via flag)
			if selectedDeployment == "" {
				selectedDeployment = packagist.DeploymentNone // default selection
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("Deployment Method").
						Description("Select how you want to deploy your project").
						Options(deploymentOptions...).
						Value(&selectedDeployment),
				)
			}

			// CI/CD system (if not provided via flag)
			if selectedCI == "" {
				selectedCI = ciNone // default selection
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("CI/CD System").
						Description("Select your CI/CD platform for automated testing and deployment").
						Options(ciOptions...).
						Value(&selectedCI),
				)
			}

			// Build optional features multi-select
			var optionalOptions []huh.Option[string]
			if !cmd.PersistentFlags().Changed("git") {
				optionalOptions = append(optionalOptions, huh.NewOption("Initialize Git Repository", optionGit))
			}
			if !cmd.PersistentFlags().Changed("docker") {
				optionalOptions = append(optionalOptions, huh.NewOption("Local Docker Setup", optionDocker))
			}
			if !cmd.PersistentFlags().Changed("with-elasticsearch") {
				optionalOptions = append(optionalOptions, huh.NewOption("Install Elasticsearch/OpenSearch support", optionElasticsearch))
			}

			if len(optionalOptions) > 0 {
				formFields = append(formFields,
					huh.NewMultiSelect[string]().
						Title("Optional").
						Description("Select additional features to enable").
						Options(optionalOptions...).
						Value(&selectedOptions),
				)
			}

			if len(formFields) > 0 {
				form := huh.NewForm(huh.NewGroup(formFields...))
				if err := form.Run(); err != nil {
					return err
				}
			}

			// Apply selected options
			for _, opt := range selectedOptions {
				switch opt {
				case optionDocker:
					useDocker = true
				case optionGit:
					initGit = true
				case optionElasticsearch:
					withElasticsearch = true
				}
			}

			// Ask for Minio only when Docker is selected (needs second form since it depends on Docker choice)
			if useDocker && !cmd.PersistentFlags().Changed("minio") {
				minioForm := huh.NewForm(huh.NewGroup(
					huh.NewConfirm().
						Title("Enable Minio (S3 Storage)?").
						Description("Add Minio as local S3-compatible storage for development").
						Value(&useMinio),
				))
				if err := minioForm.Run(); err != nil {
					return err
				}
			}
		}

		// Check PHP version if not using Docker
		if !useDocker {
			phpOk, err := system.IsPHPVersionAtLeast(cmd.Context(), "8.2")
			if err != nil {
				return fmt.Errorf("PHP 8.2 or higher is required: %w", err)
			}
			if !phpOk {
				return fmt.Errorf("PHP 8.2 or higher is required for Shopware 6")
			}
		}

		// Validate deployment method
		validDeployments := map[string]bool{
			packagist.DeploymentNone:         true,
			packagist.DeploymentDeployer:     true,
			packagist.DeploymentPlatformSH:   true,
			packagist.DeploymentShopwarePaaS: true,
		}
		if !validDeployments[selectedDeployment] {
			return fmt.Errorf("invalid deployment method: %s. Valid options: none, deployer, platformsh, shopware-paas", selectedDeployment)
		}

		// Check if folder exists and is not empty
		if _, err := os.Stat(projectFolder); err == nil {
			empty, err := system.IsDirEmpty(projectFolder)
			if err != nil {
				return err
			}

			if !empty {
				return fmt.Errorf("the folder %s exists already and is not empty", projectFolder)
			}
		}

		logging.FromContext(cmd.Context()).Infof("Using Symfony Flex to create a new Shopware 6 project")

		// Resolve version
		chooseVersion := resolveVersion(selectedVersion, filteredVersions)
		if chooseVersion == "" {
			return fmt.Errorf("cannot find version %s", selectedVersion)
		}

		if err := os.MkdirAll(projectFolder, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Setting up Shopware %s", chooseVersion)

		composerJson, err := packagist.GenerateComposerJson(cmd.Context(), packagist.ComposerJsonOptions{
			Version:          chooseVersion,
			RC:               strings.Contains(chooseVersion, "rc"),
			UseDocker:        useDocker,
			UseMinio:         useMinio,
			UseElasticsearch: withElasticsearch,
			NoAudit:          noAudit,
			DeploymentMethod: selectedDeployment,
		})
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, "composer.json"), []byte(composerJson), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".env"), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".env.local"), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".gitignore"), []byte("/.idea\n/vendor"), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "plugins"), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "static-plugins"), os.ModePerm); err != nil {
			return err
		}

		if !useDocker && system.IsSymfonyCliInstalled() {
			if err := os.WriteFile(filepath.Join(projectFolder, "php.ini"), []byte("memory_limit=512M"), os.ModePerm); err != nil {
				return err
			}
		}

		if err := setupDeployment(projectFolder, selectedDeployment); err != nil {
			return err
		}

		if err := setupCI(projectFolder, selectedCI); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Installing dependencies")

		if err := runComposerInstall(cmd.Context(), projectFolder, useDocker); err != nil {
			return err
		}

		// Initialize git repository if requested
		if initGit {
			logging.FromContext(cmd.Context()).Infof("Initializing Git repository")
			if err := git.Init(cmd.Context(), projectFolder); err != nil {
				return fmt.Errorf("failed to initialize git repository: %w", err)
			}
		}

		logging.FromContext(cmd.Context()).Infof("Project created successfully in %s", projectFolder)

		return nil
	},
}

func resolveVersion(selectedVersion string, filteredVersions []*version.Version) string {
	if selectedVersion == "latest" {
		// pick the most recent stable (non-RC) version
		for _, v := range filteredVersions {
			vs := v.String()
			if !strings.Contains(strings.ToLower(vs), "rc") {
				return vs
			}
		}
		// if no stable found, fall back to top entry
		if len(filteredVersions) > 0 {
			return filteredVersions[0].String()
		}
		return ""
	}

	if strings.HasPrefix(selectedVersion, "dev-") {
		return selectedVersion
	}

	for _, release := range filteredVersions {
		if release.String() == selectedVersion {
			return release.String()
		}
	}

	return ""
}

func setupDeployment(projectFolder, deploymentMethod string) error {
	switch deploymentMethod {
	case packagist.DeploymentDeployer:
		if err := os.WriteFile(filepath.Join(projectFolder, "deploy.php"), []byte(deployerTemplate), os.ModePerm); err != nil {
			return err
		}

	case packagist.DeploymentShopwarePaaS:
		shopwarePaasApp := `app:
  php:
    version: "8.4"
services:
  mysql:
    version: "8.0"
`

		if err := os.WriteFile(path.Join(projectFolder, "application.yaml"), []byte(shopwarePaasApp), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func setupCI(projectFolder, ciSystem string) error {
	switch ciSystem {
	case "github":
		if err := os.MkdirAll(filepath.Join(projectFolder, ".github", "workflows"), os.ModePerm); err != nil {
			return err
		}

		ciWorkflow := `name: CI

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Setup PHP
        uses: shivammathur/setup-php@v2
        with:
          php-version: '8.3'
          extensions: mbstring, xml, ctype, iconv, intl, pdo_mysql, dom, gd, zip, curl
          coverage: none

      - name: Install dependencies
        run: composer install --prefer-dist --no-progress

      - name: Run PHPStan
        run: composer run phpstan

      - name: Run PHPUnit
        run: composer run phpunit
`
		if err := os.WriteFile(filepath.Join(projectFolder, ".github", "workflows", "ci.yml"), []byte(ciWorkflow), os.ModePerm); err != nil {
			return err
		}

	case "gitlab":
		ciConfig := `stages:
  - build
  - test

variables:
  COMPOSER_HOME: ${CI_PROJECT_DIR}/.composer

cache:
  paths:
    - .composer/
    - vendor/

build:
  stage: build
  image: php:8.3-cli
  before_script:
    - apt-get update && apt-get install -y git unzip libzip-dev libpng-dev libicu-dev
    - docker-php-ext-install zip gd intl pdo_mysql
    - curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer
  script:
    - composer install --prefer-dist --no-progress

phpstan:
  stage: test
  image: php:8.3-cli
  before_script:
    - apt-get update && apt-get install -y git unzip libzip-dev libpng-dev libicu-dev
    - docker-php-ext-install zip gd intl pdo_mysql
    - curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer
  script:
    - composer install --prefer-dist --no-progress
    - composer run phpstan
  needs:
    - build

phpunit:
  stage: test
  image: php:8.3-cli
  before_script:
    - apt-get update && apt-get install -y git unzip libzip-dev libpng-dev libicu-dev
    - docker-php-ext-install zip gd intl pdo_mysql
    - curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer
  script:
    - composer install --prefer-dist --no-progress
    - composer run phpunit
  needs:
    - build
`
		if err := os.WriteFile(filepath.Join(projectFolder, ".gitlab-ci.yml"), []byte(ciConfig), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func runComposerInstall(ctx context.Context, projectFolder string, useDocker bool) error {
	var cmdInstall *exec.Cmd

	if useDocker {
		absProjectFolder, err := filepath.Abs(projectFolder)
		if err != nil {
			return err
		}

		dockerArgs := []string{"run", "--rm",
			"-v", fmt.Sprintf("%s:/app", absProjectFolder),
			"-w", "/app",
			"ghcr.io/shopware/docker-dev:php8.3-node22-caddy",
			"composer", "install", "--no-interaction"}

		cmdInstall = exec.CommandContext(ctx, "docker", dockerArgs...)
		cmdInstall.Stdout = os.Stdout
		cmdInstall.Stderr = os.Stderr

		return cmdInstall.Run()
	}

	composerBinary, err := exec.LookPath("composer")
	if err != nil {
		return err
	}

	phpBinary := os.Getenv("PHP_BINARY")

	if phpBinary != "" {
		cmdInstall = exec.CommandContext(ctx, phpBinary, composerBinary, "install", "--no-interaction")
	} else {
		cmdInstall = exec.CommandContext(ctx, "composer", "install", "--no-interaction")
	}

	cmdInstall.Dir = projectFolder
	cmdInstall.Stdin = os.Stdin
	cmdInstall.Stdout = os.Stdout
	cmdInstall.Stderr = os.Stderr

	return cmdInstall.Run()
}

func getFilteredInstallVersions(ctx context.Context) ([]*version.Version, error) {
	releases, err := fetchAvailableShopwareVersions(ctx)
	if err != nil {
		return nil, err
	}

	filteredVersions := make([]*version.Version, 0)
	constraint, _ := version.NewConstraint(">=6.4.18.0")

	for _, release := range releases {
		parsed := version.Must(version.NewVersion(release))

		if constraint.Check(parsed) {
			filteredVersions = append(filteredVersions, parsed)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(filteredVersions)))

	return filteredVersions, nil
}

func init() {
	projectRootCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.PersistentFlags().Bool("docker", false, "Use Docker to run Composer instead of local installation")
	projectCreateCmd.PersistentFlags().Bool("with-elasticsearch", false, "Include Elasticsearch/OpenSearch support")
	projectCreateCmd.PersistentFlags().Bool("without-elasticsearch", false, "Remove Elasticsearch from the installation (deprecated: use --with-elasticsearch)")
	projectCreateCmd.PersistentFlags().Bool("no-audit", false, "Disable composer audit blocking insecure packages")
	projectCreateCmd.PersistentFlags().Bool("git", false, "Initialize a Git repository")
	projectCreateCmd.PersistentFlags().Bool("minio", false, "Add Minio as local S3-compatible storage (requires Docker)")
	projectCreateCmd.PersistentFlags().String("version", "", "Shopware version to install (e.g., 6.6.0.0, latest)")
	projectCreateCmd.PersistentFlags().String("deployment", "", "Deployment method: none, deployer, platformsh, shopware-paas")
	projectCreateCmd.PersistentFlags().String("ci", "", "CI/CD system: none, github, gitlab")
}

func fetchAvailableShopwareVersions(ctx context.Context) ([]string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://releases.shopware.com/changelog/index.json", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("fetchAvailableShopwareVersions: %v", err)
		}
	}()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []string

	if err := json.Unmarshal(content, &releases); err != nil {
		return nil, err
	}

	return releases, nil
}
