package project

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/huh"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/color"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

//go:embed static/deploy.php
var deployerTemplate string

//go:embed static/github-ci.yml
var githubCITemplate string

//go:embed static/github-deploy.yml
var githubDeployTemplate string

//go:embed static/gitlab-ci.yml.tmpl
var gitlabCITemplate string

const versionLatest = "latest"

var projectCreateCmd = &cobra.Command{
	Use:   "create [name] [version]",
	Short: "Create a new Shopware 6 project",
	Args:  cobra.MaximumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{}, cobra.ShellCompDirectiveFilterDirs
		}

		if len(args) == 1 {
			filteredVersions, err := getFilteredInstallVersions(cmd.Context())
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveNoFileComp
			}
			versions := make([]string, 0, len(filteredVersions)+1)
			versions = append(versions, versionLatest)
			for _, v := range filteredVersions {
				versions = append(versions, v.String())
			}
			return versions, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{}, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useDocker, _ := cmd.PersistentFlags().GetBool("docker")
		withElasticsearch, _ := cmd.PersistentFlags().GetBool("with-elasticsearch")
		withoutElasticsearch, _ := cmd.PersistentFlags().GetBool("without-elasticsearch")
		withAMQP, _ := cmd.PersistentFlags().GetBool("with-amqp")
		noAudit, _ := cmd.PersistentFlags().GetBool("no-audit")
		initGit, _ := cmd.PersistentFlags().GetBool("git")
		versionFlag, _ := cmd.PersistentFlags().GetString("version")
		deploymentMethod, _ := cmd.PersistentFlags().GetString("deployment")
		ciSystem, _ := cmd.PersistentFlags().GetString("ci")

		if cmd.PersistentFlags().Changed("without-elasticsearch") {
			logging.FromContext(cmd.Context()).Warnf("Flag --without-elasticsearch is deprecated, use --with-elasticsearch instead")
			withElasticsearch = !withoutElasticsearch
		}

		interactive := system.IsInteractionEnabled(cmd.Context())

		const (
			optionDocker        = "docker"
			optionGit           = "git"
			optionElasticsearch = "elasticsearch"
			optionAMQP          = "amqp"

			ciNone   = "none"
			ciGitHub = "github"
			ciGitLab = "gitlab"
		)

		filteredVersions, err := getFilteredInstallVersions(cmd.Context())
		if err != nil {
			return err
		}

		versionOptions := make([]huh.Option[string], 0, len(filteredVersions)+1)
		versionOptions = append(versionOptions, huh.NewOption(color.NeutralText.Render(versionLatest), versionLatest))
		for _, v := range filteredVersions {
			versionStr := v.String()
			versionOptions = append(versionOptions, huh.NewOption(color.NeutralText.Render(versionStr), versionStr))
		}

		deploymentOptions := []huh.Option[string]{
			huh.NewOption(color.RecommendedText.Render("PaaS powered by Shopware (Recommended)"), packagist.DeploymentShopwarePaaS),
			huh.NewOption(color.NeutralText.Render("None"), packagist.DeploymentNone),
			huh.NewOption(color.SecondaryText.Render("DeployerPHP"), packagist.DeploymentDeployer),
			huh.NewOption(color.SecondaryText.Render("PaaS powered by Platform.sh"), packagist.DeploymentPlatformSH),
		}

		ciOptions := []huh.Option[string]{
			huh.NewOption(color.RecommendedText.Render("GitHub Actions (Recommended)"), ciGitHub),
			huh.NewOption(color.NeutralText.Render("None"), ciNone),
			huh.NewOption(color.NeutralText.Render("GitLab CI"), ciGitLab),
		}

		var projectFolder string
		selectedVersion := versionFlag
		selectedDeployment := deploymentMethod
		selectedCI := ciSystem
		var selectedOptions []string

		if len(args) > 0 {
			projectFolder = args[0]
		}

		if len(args) > 1 && selectedVersion == "" {
			selectedVersion = args[1]
		}

		if !interactive {
			if projectFolder == "" {
				return fmt.Errorf("project name is required in non-interactive mode")
			}
			if selectedVersion == "" {
				selectedVersion = versionLatest
			}
			if selectedDeployment == "" {
				selectedDeployment = packagist.DeploymentNone
			}
			if selectedCI == "" {
				selectedCI = ciNone
			}
			if !cmd.PersistentFlags().Changed("with-elasticsearch") {
				withElasticsearch = true
			}
		} else {
			var formFields []huh.Field

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
							if info, err := os.Stat(s); err == nil && info.IsDir() {
								empty, err := system.IsDirEmpty(s)
								if err != nil {
									return err
								}
								if !empty {
									return fmt.Errorf("folder already exists and is not empty")
								}
							}
							return nil
						}),
				)
			}

			if selectedVersion == "" {
				selectedVersion = versionLatest
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("Shopware Version").
						Description("Select the Shopware version to install").
						Options(versionOptions...).
						Height(10).
						Value(&selectedVersion),
				)
			}

			if selectedDeployment == "" {
				selectedDeployment = packagist.DeploymentNone
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("Deployment Method").
						Description("Select how you want to deploy your project").
						Options(deploymentOptions...).
						Value(&selectedDeployment),
				)
			}

			if selectedCI == "" {
				selectedCI = ciNone
				formFields = append(formFields,
					huh.NewSelect[string]().
						Title("CI/CD System").
						Description("Select your CI/CD platform for automated testing and deployment").
						Options(ciOptions...).
						Value(&selectedCI),
				)
			}

			var optionalOptions []huh.Option[string]
			if !cmd.PersistentFlags().Changed("git") {
				optionalOptions = append(optionalOptions, huh.NewOption(color.RecommendedText.Render("Initialize Git Repository (Recommended)"), optionGit).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("docker") {
				optionalOptions = append(optionalOptions, huh.NewOption(color.RecommendedText.Render("Local Docker Setup (Recommended)"), optionDocker).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("with-amqp") {
				optionalOptions = append(optionalOptions, huh.NewOption(color.RecommendedText.Render("AMQP Queue Support (Recommended)"), optionAMQP).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("with-elasticsearch") {
				optionalOptions = append(optionalOptions, huh.NewOption(color.NeutralText.Render("Setup Elasticsearch/OpenSearch support"), optionElasticsearch))
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

			for _, opt := range selectedOptions {
				switch opt {
				case optionDocker:
					useDocker = true
				case optionGit:
					initGit = true
				case optionElasticsearch:
					withElasticsearch = true
				case optionAMQP:
					withAMQP = true
				}
			}
		}

		if !useDocker {
			phpOk, err := system.IsPHPVersionAtLeast(cmd.Context(), "8.2")
			if err != nil {
				return fmt.Errorf("PHP 8.2 or higher is required: %w", err)
			}
			if !phpOk {
				return fmt.Errorf("PHP 8.2 or higher is required for Shopware 6")
			}
		}

		validDeployments := map[string]bool{
			packagist.DeploymentNone:         true,
			packagist.DeploymentDeployer:     true,
			packagist.DeploymentPlatformSH:   true,
			packagist.DeploymentShopwarePaaS: true,
		}
		if !validDeployments[selectedDeployment] {
			return fmt.Errorf("invalid deployment method: %s. Valid options: none, deployer, platformsh, shopware-paas", selectedDeployment)
		}

		validCISystems := map[string]bool{
			ciNone:   true,
			ciGitHub: true,
			ciGitLab: true,
		}
		if !validCISystems[selectedCI] {
			return fmt.Errorf("invalid CI system: %s. Valid options: none, github, gitlab", selectedCI)
		}

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
			UseElasticsearch: withElasticsearch,
			UseAMQP:          withAMQP,
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

		if err := setupCI(projectFolder, selectedCI, selectedDeployment); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Installing dependencies")

		if err := runComposerInstall(cmd.Context(), projectFolder, useDocker); err != nil {
			return err
		}

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
	if selectedVersion == versionLatest {
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

		if err := os.WriteFile(filepath.Join(projectFolder, "application.yaml"), []byte(shopwarePaasApp), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func setupCI(projectFolder, ciSystem, deploymentMethod string) error {
	switch ciSystem {
	case "github":
		if err := os.MkdirAll(filepath.Join(projectFolder, ".github", "workflows"), os.ModePerm); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(projectFolder, ".github", "workflows", "ci.yml"), []byte(githubCITemplate), os.ModePerm); err != nil {
			return err
		}
		if deploymentMethod == packagist.DeploymentDeployer {
			if err := os.WriteFile(filepath.Join(projectFolder, ".github", "workflows", "deploy.yml"), []byte(githubDeployTemplate), os.ModePerm); err != nil {
				return err
			}
		}

	case "gitlab":
		tmpl, err := template.New("gitlab-ci").Parse(gitlabCITemplate)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, struct{ Deployer bool }{Deployer: deploymentMethod == packagist.DeploymentDeployer}); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".gitlab-ci.yml"), buf.Bytes(), os.ModePerm); err != nil {
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

		dockerArgs := []string{"run", "--rm", "--pull=always",
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
	projectCreateCmd.PersistentFlags().Bool("with-amqp", false, "Include AMQP queue support (symfony/amqp-messenger)")
	projectCreateCmd.PersistentFlags().Bool("no-audit", false, "Disable composer audit blocking insecure packages")
	projectCreateCmd.PersistentFlags().Bool("git", false, "Initialize a Git repository")
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
