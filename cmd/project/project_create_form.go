package project

import (
	"fmt"
	"os"
	"slices"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

func runCreateForm(cmd *cobra.Command, opts *createOptions, filteredVersions []*version.Version) error { //nolint:gocyclo
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
	minorOptions = append(minorOptions, huh.NewOption(shop.VersionLatest, shop.VersionLatest))
	for _, g := range minorGroups {
		minorOptions = append(minorOptions, huh.NewOption(g.label, g.label))
	}

	deploymentOptions := []huh.Option[string]{
		huh.NewOption("None", shop.DeploymentNone),
		huh.NewOption("PaaS powered by Shopware", shop.DeploymentShopwarePaaS),
		huh.NewOption("PaaS powered by Platform.sh", shop.DeploymentPlatformSH),
		huh.NewOption("Deployer (SSH-based)", shop.DeploymentDeployer),
	}

	ciOptions := []huh.Option[string]{
		huh.NewOption("None", shop.CINone),
		huh.NewOption("GitHub Actions", shop.CIGitHub),
		huh.NewOption("GitLab CI", shop.CIGitLab),
	}

	needsAdvanced := opts.selectedDeployment == "" || opts.selectedCI == "" ||
		!cmd.PersistentFlags().Changed("git") ||
		!cmd.PersistentFlags().Changed("with-amqp") ||
		!opts.elasticsearchExplicit

	needsProjectFolder := opts.projectFolder == ""
	needsVersion := opts.selectedVersion == ""
	needsDeployment := opts.selectedDeployment == ""
	needsCI := opts.selectedCI == ""

	selectDocker := tui.Yes
	selectGit := tui.Yes
	selectElasticsearch := tui.No
	selectAMQP := tui.Yes

	if !system.IsGitInstalled() {
		selectGit = tui.No
	}

	if !opts.useDocker {
		extensions, err := system.GetAvailablePHPExtensions(cmd.Context())
		if err == nil && !slices.Contains(extensions, "amqp") {
			selectAMQP = tui.No
		}
	}
	selectedMinor := shop.VersionLatest

	theme := huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.Focused.Title = s.Focused.Title.Foreground(tui.BlueColor)
		s.Blurred.Title = s.Blurred.Title.Foreground(tui.BlueColor)
		return s
	})

	onOff := func(v bool) string {
		if v {
			return tui.GreenText.Render("Yes")
		}
		return tui.RedText.Render("No")
	}

	sectionStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	labelStyle := lipgloss.NewStyle().Width(20)

	for {
		var formGroups []*huh.Group

		if needsProjectFolder {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewInput().
					Title("Project Name").
					DescriptionFunc(func() string {
						return projectNameFieldDescription(opts.projectFolder)
					}, &opts.projectFolder).
					Placeholder("my-shopware-project (leave empty for current directory)").
					Value(&opts.projectFolder).
					Validate(func(s string) error {
						if s == "" {
							return nil
						}
						return shop.ValidateProjectFolder(s)
					}),
			))
		}

		if needsVersion {
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
							out := make([]huh.Option[string], 0, len(minorGroups[idx].versions))
							for _, v := range minorGroups[idx].versions {
								out = append(out, huh.NewOption(v, v))
							}
							return out
						}
						return []huh.Option[string]{huh.NewOption(shop.VersionLatest, shop.VersionLatest)}
					}, &selectedMinor).
					Value(&opts.selectedVersion),
			).WithHideFunc(func() bool {
				return selectedMinor == shop.VersionLatest
			}))
		}

		if !cmd.PersistentFlags().Changed("docker") {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Docker").
					Description("Use Docker to run Shopware locally").
					Value(&selectDocker),
			))
		}

		selectAdvanced := tui.No
		if needsAdvanced {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Do you want to further customize the project creation?").
					Description("Configure deployment, CI/CD, and optional features").
					Value(&selectAdvanced),
			))
		}

		if needsDeployment {
			opts.selectedDeployment = shop.DeploymentNone
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[string]().
					Title("Deployment Method").
					Description("Select how you want to deploy your project").
					Options(deploymentOptions...).
					Value(&opts.selectedDeployment),
			).WithHideFunc(func() bool { return selectAdvanced != tui.Yes }))
		}

		if needsCI {
			opts.selectedCI = shop.CINone
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[string]().
					Title("CI/CD System").
					Description("Select your CI/CD platform for automated testing and deployment").
					Options(ciOptions...).
					Value(&opts.selectedCI),
			).WithHideFunc(func() bool { return selectAdvanced != tui.Yes }))
		}

		if !cmd.PersistentFlags().Changed("git") {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Git Repository").
					Description("Initialize a Git repository for version control").
					Value(&selectGit),
			).WithHideFunc(func() bool { return selectAdvanced != tui.Yes }))
		}

		if !opts.elasticsearchExplicit {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("OpenSearch").
					Description("Set up OpenSearch for large catalogs and advanced search").
					Value(&selectElasticsearch),
			).WithHideFunc(func() bool { return selectAdvanced != tui.Yes }))
		}

		if !cmd.PersistentFlags().Changed("with-amqp") {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("AMQP").
					Description("Enable AMQP queue support for background jobs and messaging").
					Value(&selectAMQP),
			).WithHideFunc(func() bool { return selectAdvanced != tui.Yes }))
		}

		if len(formGroups) > 0 {
			form := huh.NewForm(formGroups...).WithTheme(theme)
			if err := form.Run(); err != nil {
				return err
			}
		}

		if opts.selectedVersion == "" {
			opts.selectedVersion = shop.VersionLatest
		}

		if opts.projectFolder == "" {
			opts.projectFolder = "."
		}

		if !cmd.PersistentFlags().Changed("docker") {
			opts.useDocker = selectDocker == tui.Yes
		}
		if !cmd.PersistentFlags().Changed("git") {
			opts.initGit = selectGit == tui.Yes
		}
		if !opts.elasticsearchExplicit {
			opts.withElasticsearch = selectElasticsearch == tui.Yes
		}
		if !cmd.PersistentFlags().Changed("with-amqp") {
			opts.withAMQP = selectAMQP == tui.Yes
		}

		fmt.Println()
		fmt.Println(sectionStyle.Render("Summary"))
		fmt.Println()
		projectDisplay := opts.projectFolder
		if projectDisplay == "." {
			if wd, err := os.Getwd(); err == nil {
				projectDisplay = wd
			}
		}
		fmt.Printf("  %s %s\n", labelStyle.Render("Project name:"), projectDisplay)
		fmt.Printf("  %s %s\n", labelStyle.Render("Version:"), opts.selectedVersion)
		fmt.Printf("  %s %s\n", labelStyle.Render("Deployment:"), opts.selectedDeployment)
		fmt.Printf("  %s %s\n", labelStyle.Render("CI/CD:"), opts.selectedCI)
		fmt.Printf("  %s %s\n", labelStyle.Render("Docker:"), onOff(opts.useDocker))
		fmt.Printf("  %s %s\n", labelStyle.Render("Git Repository:"), onOff(opts.initGit))
		fmt.Printf("  %s %s\n", labelStyle.Render("OpenSearch:"), onOff(opts.withElasticsearch))
		fmt.Printf("  %s %s\n", labelStyle.Render("AMQP:"), onOff(opts.withAMQP))
		fmt.Println()

		selectConfirm := "proceed"
		confirmForm := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.NewOption("Proceed", "proceed"),
					huh.NewOption("Restart form", "restart"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&selectConfirm),
		)).WithTheme(theme)

		if err := confirmForm.Run(); err != nil {
			return err
		}

		if selectConfirm == "proceed" {
			return nil
		}

		if selectConfirm == "cancel" {
			return fmt.Errorf("project creation cancelled")
		}
	}
}
