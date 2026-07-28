package project

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

func runCreateForm(cmd *cobra.Command, opts *createOptions, releases []repository.Version, filteredVersions []*version.Version) error { //nolint:gocyclo
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

	needsProjectFolder := opts.projectFolder == ""
	needsVersion := opts.selectedVersion == ""
	needsDeployment := opts.selectedDeployment == ""
	needsCI := opts.selectedCI == ""
	// An explicit --php-version is authoritative and validated later, so the form
	// must not offer a competing choice.
	needsPHPVersion := !opts.phpVersionExplicit

	needsAdvanced := needsDeployment || needsCI || needsPHPVersion ||
		!cmd.PersistentFlags().Changed("git") ||
		!cmd.PersistentFlags().Changed("with-amqp") ||
		!opts.elasticsearchExplicit

	selectDocker := tui.Yes
	selectGit := tui.Yes
	selectElasticsearch := tui.No
	selectAMQP := tui.Yes

	baseDomain := proxyBaseDomain()
	// Default to the stable hostname (recommended); only applies with Docker.
	selectLocalDomain := true
	// Whether this machine already resolves the proxy domain. When it does, the
	// one-time sudo setup is already done, so we never ask for it again.
	machineSetupDone := proxy.CheckResolverConfigured(baseDomain).Configured
	selectSetupNow := tui.Yes

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

	// Docker may come from the --docker flag or from the in-form question, and
	// the PHP selection depends on the answer either way.
	dockerSelected := func() bool {
		if cmd.PersistentFlags().Changed("docker") {
			return opts.useDocker
		}
		return selectDocker == tui.Yes
	}

	// The patch-version group stays hidden for "latest" and leaves
	// opts.selectedVersion empty, which is only defaulted after the form ran.
	effectiveVersion := func() string {
		if opts.selectedVersion != "" {
			return opts.selectedVersion
		}
		return shop.VersionLatest
	}

	// Discovery spawns a subprocess per candidate, so it runs at most once; only
	// the constraint filtering is redone when the Shopware version changes. Docker
	// projects never reach it: their PHP comes from the image, not this machine.
	var phpInstallations []system.PHPInstallation
	phpDiscovered := false
	compatiblePHPForSelection := func() []system.PHPInstallation {
		if !phpDiscovered {
			phpInstallations = discoverPHPInstallations(cmd.Context())
			phpDiscovered = true
		}
		return filterCompatiblePHPFor(phpInstallations, releases, effectiveVersion(), filteredVersions)
	}

	// Docker image tags the selected Shopware release supports.
	dockerPHPForSelection := func() []string {
		return phpConstraintFor(releases, effectiveVersion(), filteredVersions).SupportedVersions()
	}

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

		if !cmd.PersistentFlags().Changed("local-domain") {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[bool]().
					Title("Local domains").
					Description("Reach this shop at a stable hostname instead of a changing port").
					OptionsFunc(func() []huh.Option[bool] {
						host := localDomainHostname(opts.projectFolder, baseDomain)
						return []huh.Option[bool]{
							huh.NewOption("Yes (recommended) — https://"+host, true),
							huh.NewOption("No — use a port (http://localhost:8000)", false),
						}
					}, &opts.projectFolder).
					Value(&selectLocalDomain),
				// The shared proxy is Docker-only, so this choice is irrelevant
				// without Docker (respecting a --docker flag override).
			).WithHideFunc(func() bool {
				if cmd.PersistentFlags().Changed("docker") {
					return !opts.useDocker
				}
				return selectDocker != tui.Yes
			}))

			// Offer the one-time machine setup inline, but only when it is
			// actually needed: local domains chosen, Docker on, and the machine
			// not configured yet. Every later project skips this automatically.
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Set up local domains on this machine now?").
					Description("One-time sudo: makes *."+baseDomain+" resolve and trusts its HTTPS certificate. Skip to run `shopware-cli project proxy setup` later.").
					Value(&selectSetupNow),
			).WithHideFunc(func() bool {
				if machineSetupDone {
					return true
				}
				dockerOn := selectDocker == tui.Yes
				if cmd.PersistentFlags().Changed("docker") {
					dockerOn = opts.useDocker
				}
				localOn := selectLocalDomain
				if cmd.PersistentFlags().Changed("local-domain") {
					localOn = opts.useLocalDomain
				}
				return !dockerOn || !localOn
			}))
		}

		selectAdvanced := tui.No
		if needsAdvanced {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Do you want to further customize the project creation?").
					Description("Configure PHP, deployment, CI/CD, and optional features").
					Value(&selectAdvanced),
			))
		}

		// A local project selects an executable installed on this machine; a Docker
		// project selects an image tag. phpGroupShown must not depend on OptionsFunc
		// having run: huh evaluates WithHideFunc during navigation, while OptionsFunc
		// is dispatched asynchronously, so deciding visibility from its options hides
		// the group forever.
		var selectedPHP string
		phpCandidates := func() int {
			if dockerSelected() {
				return len(dockerPHPForSelection())
			}
			return len(compatiblePHPForSelection())
		}
		phpGroupShown := func() bool {
			return selectAdvanced == tui.Yes && shouldPromptPHPSelection(phpCandidates())
		}

		if needsPHPVersion {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[string]().
					TitleFunc(func() string {
						if dockerSelected() {
							return "PHP Version"
						}
						return "PHP Executable"
					}, &selectDocker).
					DescriptionFunc(func() string {
						if dockerSelected() {
							return "Select the PHP version of the Docker image (persisted as docker.php.version in .shopware-project.yml)"
						}
						return "Select the PHP used to create and run this project (its version is persisted as php_version in .shopware-project.yml)"
					}, &selectDocker).
					Height(10).
					OptionsFunc(func() []huh.Option[string] {
						if dockerSelected() {
							versions := dockerPHPForSelection()
							if !slices.Contains(versions, selectedPHP) {
								selectedPHP = highestOrEmpty(versions)
							}
							return phpVersionOptions(versions)
						}

						compatible := compatiblePHPForSelection()
						// Keep the selection valid when changing the Shopware
						// version narrows the compatible set.
						if system.FindPHPByBinary(compatible, selectedPHP) == nil {
							selectedPHP = ""
							if preferred := system.PreferredPHPInstallation(compatible); preferred != nil {
								selectedPHP = preferred.Binary
							}
						}
						return phpInstallationOptions(compatible)
					}, []*string{&selectDocker, &opts.selectedVersion}).
					Value(&selectedPHP),
			).WithHideFunc(func() bool { return !phpGroupShown() }))
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
		if !cmd.PersistentFlags().Changed("local-domain") {
			// Local domains need the shared proxy, which is Docker-only.
			opts.useLocalDomain = opts.useDocker && selectLocalDomain
		}
		// Offer to run the one-time machine setup inline, only when needed.
		opts.setupProxyNow = opts.useLocalDomain && !machineSetupDone && selectSetupNow == tui.Yes
		if !cmd.PersistentFlags().Changed("git") {
			opts.initGit = selectGit == tui.Yes
		}
		if !opts.elasticsearchExplicit {
			opts.withElasticsearch = selectElasticsearch == tui.Yes
		}
		if !cmd.PersistentFlags().Changed("with-amqp") {
			opts.withAMQP = selectAMQP == tui.Yes
		}
		if needsPHPVersion {
			// Reset on every round so switching to Docker (or restarting the
			// form) does not keep a stale selection from a previous pass.
			opts.clearPHP()
			switch {
			case opts.useDocker:
				// Only a version, since the PHP comes from the image. Left empty
				// when unanswered: installAndFinalize then picks the highest the
				// release supports.
				if phpGroupShown() {
					opts.phpVersion = selectedPHP
				}
			case phpGroupShown():
				if selected := system.FindPHPByBinary(compatiblePHPForSelection(), selectedPHP); selected != nil {
					opts.setPHP(*selected)
				}
			default:
				// Nothing was asked (at most one compatible install), but resolve
				// it anyway so the summary shows the PHP that will be used.
				if preferred := system.PreferredPHPInstallation(compatiblePHPForSelection()); preferred != nil {
					opts.setPHP(*preferred)
				}
			}
		}

		fmt.Println()
		fmt.Println(tui.SectionHeadingStyle.Render("Summary"))
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
		if opts.phpVersion != "" {
			phpDisplay := opts.phpVersion
			if opts.phpBinary != "" {
				phpDisplay += " (" + opts.phpBinary + ")"
			}
			fmt.Printf("  %s %s\n", labelStyle.Render("PHP:"), phpDisplay)
		}
		if opts.useDocker {
			localDomainValue := onOff(opts.useLocalDomain)
			if opts.useLocalDomain {
				localDomainValue = tui.GreenText.Render("https://" + localDomainHostname(opts.projectFolder, baseDomain))
			}
			fmt.Printf("  %s %s\n", labelStyle.Render("Local domain:"), localDomainValue)
		}
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
			return errors.New("project creation cancelled")
		}
	}
}
