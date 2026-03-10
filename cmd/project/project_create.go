package project

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/color"
	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
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

		type minorGroup struct {
			label    string
			versions []string
		}
		var minorGroups []minorGroup
		minorIndex := map[string]int{}
		for _, v := range filteredVersions {
			segments := v.Segments()
			key := fmt.Sprintf("%d.%d", segments[0], segments[1])
			if idx, ok := minorIndex[key]; ok {
				minorGroups[idx].versions = append(minorGroups[idx].versions, v.String())
			} else {
				minorIndex[key] = len(minorGroups)
				minorGroups = append(minorGroups, minorGroup{label: key, versions: []string{v.String()}})
			}
		}

		minorOptions := make([]huh.Option[string], 0, len(minorGroups)+1)
		minorOptions = append(minorOptions, huh.NewOption(versionLatest, versionLatest))
		for _, g := range minorGroups {
			minorOptions = append(minorOptions, huh.NewOption(g.label, g.label))
		}

		deploymentOptions := []huh.Option[string]{
			huh.NewOption("None", packagist.DeploymentNone),
			huh.NewOption("PaaS powered by Shopware", packagist.DeploymentShopwarePaaS),
			huh.NewOption("PaaS powered by Platform.sh", packagist.DeploymentPlatformSH),
			huh.NewOption("Deployer (SSH-based)", packagist.DeploymentDeployer),
		}

		ciOptions := []huh.Option[string]{
			huh.NewOption("None", ciNone),
			huh.NewOption("GitHub Actions", ciGitHub),
			huh.NewOption("GitLab CI", ciGitLab),
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
			var formGroups []*huh.Group

			if projectFolder == "" {
				formGroups = append(formGroups, huh.NewGroup(
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
				))
			}

			if selectedVersion == "" {
				selectedMinor := versionLatest
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewSelect[string]().
						Title("Shopware Version").
						Description("Select the major version to install").
						Options(minorOptions...).
						Value(&selectedMinor),
				))

				formGroups = append(formGroups, huh.NewGroup(
					huh.NewSelect[string]().
						Title("Patch Version").
						Description("Select the specific patch version").
						Height(10).
						OptionsFunc(func() []huh.Option[string] {
							if idx, ok := minorIndex[selectedMinor]; ok {
								opts := make([]huh.Option[string], 0, len(minorGroups[idx].versions))
								for _, v := range minorGroups[idx].versions {
									opts = append(opts, huh.NewOption(v, v))
								}
								return opts
							}
							return []huh.Option[string]{huh.NewOption(versionLatest, versionLatest)}
						}, &selectedMinor).
						Value(&selectedVersion),
				).WithHideFunc(func() bool {
					return selectedMinor == versionLatest
				}))
			}

			needsAdvanced := selectedDeployment == "" || selectedCI == "" ||
				!cmd.PersistentFlags().Changed("git") ||
				!cmd.PersistentFlags().Changed("docker") ||
				!cmd.PersistentFlags().Changed("with-amqp") ||
				!cmd.PersistentFlags().Changed("with-elasticsearch")

			var showAdvanced bool
			if needsAdvanced {
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewConfirm().
						Title("Advanced Settings").
						Description("Configure deployment, CI/CD, and optional features").
						Value(&showAdvanced),
				))
			}

			if selectedDeployment == "" {
				selectedDeployment = packagist.DeploymentNone
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewSelect[string]().
						Title("Deployment Method").
						Description("Select how you want to deploy your project").
						Options(deploymentOptions...).
						Value(&selectedDeployment),
				).WithHideFunc(func() bool { return !showAdvanced }))
			}

			if selectedCI == "" {
				selectedCI = ciNone
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewSelect[string]().
						Title("CI/CD System").
						Description("Select your CI/CD platform for automated testing and deployment").
						Options(ciOptions...).
						Value(&selectedCI),
				).WithHideFunc(func() bool { return !showAdvanced }))
			}

			var optionalOptions []huh.Option[string]
			if !cmd.PersistentFlags().Changed("docker") {
				optionalOptions = append(optionalOptions, huh.NewOption("Install Shopware with local Docker setup", optionDocker).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("git") {
				optionalOptions = append(optionalOptions, huh.NewOption("Initialize Git repository", optionGit).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("with-amqp") {
				optionalOptions = append(optionalOptions, huh.NewOption("AMQP queue support (for background jobs and messaging)", optionAMQP).Selected(true))
			}
			if !cmd.PersistentFlags().Changed("with-elasticsearch") {
				optionalOptions = append(optionalOptions, huh.NewOption("Set up OpenSearch (for large catalogs and advanced search)", optionElasticsearch))
			}

			if len(optionalOptions) > 0 {
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewMultiSelect[string]().
						Title("Optional").
						Description("Select additional features to enable").
						Options(optionalOptions...).
						Height(10).
						Value(&selectedOptions),
				).WithHideFunc(func() bool { return !showAdvanced }))
			}

			if len(formGroups) > 0 {
				form := huh.NewForm(formGroups...)
				if err := form.Run(); err != nil {
					return err
				}
			}

			if selectedVersion == "" {
				selectedVersion = versionLatest
			}

			if !showAdvanced {
				if !cmd.PersistentFlags().Changed("git") {
					initGit = true
				}
				if !cmd.PersistentFlags().Changed("docker") {
					useDocker = true
				}
				if !cmd.PersistentFlags().Changed("with-amqp") {
					withAMQP = true
				}
			} else {
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

		if !useDocker {
			if _, err := exec.LookPath("composer"); err != nil {
				return fmt.Errorf("composer is not installed. Please install Composer (https://getcomposer.org/) or use the --docker flag")
			}
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

		go tracking.Track(cmd.Context(), "project.create", map[string]string{
			"version":    selectedVersion,
			"deployment": selectedDeployment,
			"ci":         selectedCI,
			"docker":     fmt.Sprintf("%v", useDocker),
		})

		chooseVersion := resolveVersion(selectedVersion, filteredVersions)
		if chooseVersion == "" {
			return fmt.Errorf("cannot find version %s", selectedVersion)
		}

		if err := os.MkdirAll(projectFolder, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Setting up Shopware %s", chooseVersion)

		// @todo: it's broken in paas deployments, the paas recipe configures Elasticsearch and it's difficult to do it only when elasticsearch is available.
		if selectedDeployment == packagist.DeploymentShopwarePaaS {
			withElasticsearch = true
		}

		composerJson, err := packagist.GenerateComposerJson(cmd.Context(), packagist.ComposerJsonOptions{
			Version:          chooseVersion,
			RC:               strings.Contains(chooseVersion, "rc"),
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

		envLocalContent := ""

		if useDocker {
			envLocalContent += "APP_ENV=dev\n"
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".env.local"), []byte(envLocalContent), os.ModePerm); err != nil {
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

		isVerbose, _ := cmd.Flags().GetBool("verbose")
		showSpinner := system.IsInteractionEnabled(cmd.Context()) && !isVerbose

		if err := runComposerInstall(cmd.Context(), projectFolder, useDocker, showSpinner); err != nil {
			return err
		}

		if useDocker {
			if err := dockerpkg.WriteComposeFile(projectFolder); err != nil {
				return err
			}
		}

		if initGit {
			logging.FromContext(cmd.Context()).Infof("Initializing Git repository")
			if err := git.Init(cmd.Context(), projectFolder); err != nil {
				return fmt.Errorf("failed to initialize git repository: %w", err)
			}
		}

		logging.FromContext(cmd.Context()).Infof("Project created successfully in %s", projectFolder)

		shopCfg := shop.NewConfig()
		if useDocker {
			shopCfg.Environments["local"].Type = "docker"
		}

		if err := shop.WriteConfig(shopCfg, projectFolder); err != nil {
			return err
		}

		if useDocker {
			cmdStyle := lipgloss.NewStyle().Bold(true)
			sectionStyle := lipgloss.NewStyle().Bold(true).Underline(true)

			fmt.Println()
			fmt.Println(sectionStyle.Render("Next steps"))
			fmt.Println()
			fmt.Printf("  %s  %s\n", color.GreenText.Render("Start developing:"), cmdStyle.Render(fmt.Sprintf("cd %s && shopware-cli project dev", projectFolder)))
			fmt.Println()
			fmt.Println(sectionStyle.Render("Access your shop (after make setup)"))
			fmt.Println()
			fmt.Printf("  %s  %s\n", color.GreenText.Render("Storefront:"), cmdStyle.Render("http://127.0.0.1:8000"))
			fmt.Printf("  %s  %s\n", color.GreenText.Render("Admin:"), cmdStyle.Render("http://127.0.0.1:8000/admin"))
			fmt.Printf("  %s  %s / %s\n", color.GreenText.Render("Credentials:"), cmdStyle.Render("admin"), cmdStyle.Render("shopware"))
			fmt.Println()
		}

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

func runComposerInstall(ctx context.Context, projectFolder string, useDocker bool, showSpinner bool) error {
	var cmdInstall *exec.Cmd

	if useDocker && !system.IsInsideContainer() {
		absProjectFolder, err := filepath.Abs(projectFolder)
		if err != nil {
			return err
		}

		dockerArgs := []string{"run",
			"--rm",
			"--pull=always",
			"-v", fmt.Sprintf("%s:/app", absProjectFolder),
			"-w", "/app"}

		if system.IsDockerMountable() {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				composerDir := filepath.Join(homeDir, ".composer")
				_ = os.MkdirAll(composerDir, 0o755)
				dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/tmp/composer/", composerDir))
			}
		}

		dockerArgs = append(dockerArgs,
			"ghcr.io/shopware/docker-dev:php8.3-node22-caddy",
			"composer", "install", "--no-interaction")

		cmdInstall = exec.CommandContext(ctx, "docker", dockerArgs...)
	} else {
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
	}

	if !showSpinner {
		cmdInstall.Stdin = os.Stdin
		cmdInstall.Stdout = os.Stdout
		cmdInstall.Stderr = os.Stderr

		return cmdInstall.Run()
	}

	var stdErr bytes.Buffer
	cmdInstall.Stderr = &stdErr

	var runErr error

	if err := spinner.New().Context(ctx).Title("Installing dependencies").Action(func() {
		runErr = cmdInstall.Run()
	}).Run(); err != nil {
		return err
	}

	if runErr != nil {
		if stdErr.Len() > 0 {
			fmt.Fprint(os.Stderr, stdErr.String())
		}

		return runErr
	}

	return nil
}

func getFilteredInstallVersions(ctx context.Context) ([]*version.Version, error) {
	releases, err := packagist.GetShopwarePackageVersions(ctx)
	if err != nil {
		return nil, err
	}

	filteredVersions := make([]*version.Version, 0)
	constraint, _ := version.NewConstraint(">=6.4.18.0")

	for _, release := range releases {
		if strings.HasPrefix(release.Version, "dev-") {
			continue
		}

		parsed, err := version.NewVersion(release.Version)
		if err != nil {
			continue
		}

		if constraint.Check(parsed) {
			filteredVersions = append(filteredVersions, parsed)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(filteredVersions)))

	for i, v := range filteredVersions {
		filteredVersions[i], _ = version.NewVersion(strings.TrimPrefix(v.String(), "v"))
	}

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

