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
	// Now I will group full versions (e.g. 6.6.10.0) under their minor line (e.g. "6.6")
	// so later I can ask "which line?" before "which exact release?".
	type minorGroup struct {
		label    string   // "6.6"
		versions []string // "6.6.10.0", "6.6.9.0", ...
	}
	var minorGroups []minorGroup
	// Now I will keep a map from "6.6" to its index in minorGroups so I can
	// append without scanning the slice each time.
	minorIndex := map[string]int{}
	for _, v := range filteredVersions {
		// Now I will split 6.6.10.0 into [6, 6, 10, 0] and keep only major.minor.
		segments := v.Segments()
		key := fmt.Sprintf("%d.%d", segments[0], segments[1])
		if idx, ok := minorIndex[key]; ok {
			// Now I will add this patch to the group I already started.
			minorGroups[idx].versions = append(minorGroups[idx].versions, v.String())
		} else {
			// Now I will remember this new minor line and start a group with this version.
			minorIndex[key] = len(minorGroups)
			minorGroups = append(minorGroups, minorGroup{label: key, versions: []string{v.String()}})
		}
	}

	// Now I will turn those groups into dropdown options, putting "latest" first.
	minorOptions := make([]huh.Option[string], 0, len(minorGroups)+1)
	minorOptions = append(minorOptions, huh.NewOption(shop.VersionLatest, shop.VersionLatest))
	for _, g := range minorGroups {
		minorOptions = append(minorOptions, huh.NewOption(g.label, g.label))
	}

	// Now I will prepare the deployment and CI dropdowns I may show later.
	deploymentOptions := []huh.Option[string]{
		huh.NewOption("None", shop.DeploymentNone),
		huh.NewOption("Docker (Container)", shop.DeploymentContainer),
		huh.NewOption("PaaS powered by Shopware", shop.DeploymentShopwarePaaS),
		huh.NewOption("PaaS powered by Platform.sh", shop.DeploymentPlatformSH),
		huh.NewOption("Deployer (SSH-based)", shop.DeploymentDeployer),
	}

	ciOptions := []huh.Option[string]{
		huh.NewOption("None", shop.CINone),
		huh.NewOption("GitHub Actions", shop.CIGitHub),
		huh.NewOption("GitLab CI", shop.CIGitLab),
	}

	// Now I will check which answers are still missing (flags already fill some of these).
	needsProjectFolder := opts.projectFolder == ""
	needsVersion := opts.selectedVersion == ""
	needsDeployment := opts.selectedDeployment == ""
	needsCI := opts.selectedCI == ""
	// Now I will skip the PHP question if the user already passed --php-version;
	// that flag is authoritative and gets validated later.
	needsPHPVersion := !opts.phpVersionExplicit

	// Now I will decide whether the "customize further?" step is worth showing at all.
	needsAdvanced := needsDeployment || needsCI || needsPHPVersion ||
		!cmd.PersistentFlags().Changed("git") ||
		!cmd.PersistentFlags().Changed("with-amqp") ||
		!opts.elasticsearchExplicit

	// Now I will set friendly defaults for the yes/no questions.
	selectDocker := tui.Yes
	selectGit := tui.Yes
	selectElasticsearch := tui.No
	selectAMQP := tui.Yes

	baseDomain := proxy.BaseDomain()
	// Now I will default to a stable hostname (recommended); this only applies with Docker.
	selectLocalDomain := true
	// Now I will check whether this machine already resolves the proxy domain.
	// If it does, I will never ask for the one-time sudo setup again.
	machineSetupDone := proxy.CheckResolverConfigured(baseDomain).Configured
	selectSetupNow := tui.Yes

	// Now I will turn Git off by default if git is not installed on this machine.
	if !system.IsGitInstalled() {
		selectGit = tui.No
	}

	// Now I will turn AMQP off by default when this machine's PHP has no amqp extension
	// (Docker projects skip this check; the image can include it).
	if !opts.useDocker {
		extensions, err := system.GetAvailablePHPExtensions(cmd.Context())
		if err == nil && !slices.Contains(extensions, "amqp") {
			selectAMQP = tui.No
		}
	}
	selectedMinor := shop.VersionLatest

	// Now I will define a helper: Docker may come from --docker or from the form,
	// and PHP options depend on that answer either way.
	dockerSelected := func() bool {
		if cmd.PersistentFlags().Changed("docker") {
			return opts.useDocker
		}
		return selectDocker == tui.Yes
	}

	// Now I will define a helper for "which Shopware version is in play right now?".
	// The patch dropdown stays hidden for "latest" and leaves opts.selectedVersion
	// empty until I default it after the form ran.
	effectiveVersion := func() string {
		if opts.selectedVersion != "" {
			return opts.selectedVersion
		}
		return shop.VersionLatest
	}

	// Now I will prepare PHP discovery, but I will not scan the machine until I need it.
	// Discovery spawns a subprocess per candidate, so I run it at most once; I only
	// re-filter when the Shopware version changes. Docker projects never reach this:
	// their PHP comes from the image, not this machine.
	var phpInstallations []system.PHPInstallation
	phpDiscovered := false
	compatiblePHPForSelection := func() []system.PHPInstallation {
		if !phpDiscovered {
			phpInstallations = discoverPHPInstallations(cmd.Context())
			phpDiscovered = true
		}
		return filterCompatiblePHPFor(phpInstallations, releases, effectiveVersion(), filteredVersions)
	}

	// Now I will list Docker image PHP tags that the selected Shopware release supports.
	dockerPHPForSelection := func() []string {
		return phpConstraintFor(releases, effectiveVersion(), filteredVersions).SupportedVersions()
	}

	// Now I will style the form (blue titles) and helpers for the summary screen.
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

	// Now I will loop: show the form, print a summary, then proceed / restart / cancel.
	for {
		var formGroups []*huh.Group

		// Now I will ask for a project folder if the user did not already pass one.
		if needsProjectFolder {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewInput().
					Title("Project Name").
					Description(projectNameHelp).
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

		// Now I will ask for Shopware line, then (unless they picked "latest") the patch.
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

		// Now I will ask about Docker unless --docker already answered it.
		if !cmd.PersistentFlags().Changed("docker") {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[string]().
					Title("Docker").
					Description("How do you want to run Shopware?").
					Options(
						huh.NewOption("Run Shopware with Docker", tui.Yes),
						huh.NewOption("Use PHP and Composer; Shopware CLI handles the installation", tui.No),
					).
					Value(&selectDocker),
			))
		}

		// Now I will ask about a stable local hostname unless --local-domain was set.
		if !cmd.PersistentFlags().Changed("local-domain") {
			formGroups = append(formGroups, huh.NewGroup(
				huh.NewSelect[bool]().
					Title("Local domains").
					Description("Reach this shop at a stable hostname instead of a changing port").
					OptionsFunc(func() []huh.Option[bool] {
						host := "<name>." + baseDomain
						if opts.projectFolder != "" {
							host = proxy.LocalDomainHostname(opts.projectFolder, baseDomain)
						}
						return []huh.Option[bool]{
							huh.NewOption("Yes (recommended) — https://"+host, true),
							huh.NewOption("No — use a port (http://localhost:8000)", false),
						}
					}, &opts.projectFolder).
					Value(&selectLocalDomain),
				// Now I will hide this when Docker is off: the shared proxy is Docker-only.
			).WithHideFunc(func() bool {
				if cmd.PersistentFlags().Changed("docker") {
					return !opts.useDocker
				}
				return selectDocker != tui.Yes
			}))

			// Now I will offer one-time machine setup, but only when it is actually
			// needed: local domains on, Docker on, and this machine not configured yet.
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

		// Now I will ask whether they want the extra options (PHP, deploy, CI, extras).
		selectAdvanced := tui.No
		if needsAdvanced {
			formGroups = append(formGroups, huh.NewGroup(
				tui.NewYesNo().
					Title("Do you want to further customize the project creation?").
					Description("Configure PHP, deployment, CI/CD, and optional features").
					Value(&selectAdvanced),
			))
		}

		// Now I will decide how PHP is chosen: a local executable vs a Docker image tag.
		// I must not hide this group based on OptionsFunc having run — huh evaluates
		// WithHideFunc during navigation, while OptionsFunc is async, so that would
		// hide the group forever.
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
						// Now I will reset the pick if a Shopware version change made it invalid.
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

		// Now I will add the advanced questions, each hidden until they said "customize".
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

		// Now I will run the form if I actually built any questions.
		if len(formGroups) > 0 {
			form := huh.NewForm(formGroups...).WithTheme(theme)
			if err := form.Run(); err != nil {
				return err
			}
		}

		// Now I will fill in defaults for anything still empty after the prompts.
		if opts.selectedVersion == "" {
			opts.selectedVersion = shop.VersionLatest
		}

		if opts.projectFolder == "" {
			opts.projectFolder = "."
		}

		// Now I will copy form answers into opts, unless a flag already owns that field.
		if !cmd.PersistentFlags().Changed("docker") {
			opts.useDocker = selectDocker == tui.Yes
		}
		// Now I will resolve local-domain: the flag wins when set, otherwise the prompt.
		// I only offer one-time setup when I actually prompted (never unprompted sudo).
		localFlagChanged := cmd.PersistentFlags().Changed("local-domain")
		wantLocalDomain := opts.useLocalDomain
		if !localFlagChanged {
			wantLocalDomain = selectLocalDomain
		}
		opts.useLocalDomain, opts.setupProxyNow = resolveLocalDomainChoice(
			opts.useDocker, wantLocalDomain, !localFlagChanged, machineSetupDone, selectSetupNow == tui.Yes)
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
			// Now I will clear PHP so a restart or Docker switch cannot keep a stale pick.
			opts.clearPHP()
			switch {
			case opts.useDocker:
				// Now I will store only a version string (PHP lives in the image).
				// If I never asked, I leave it empty so installAndFinalize can pick the highest.
				if phpGroupShown() {
					opts.phpVersion = selectedPHP
				}
			case phpGroupShown():
				if selected := system.FindPHPByBinary(compatiblePHPForSelection(), selectedPHP); selected != nil {
					opts.setPHP(*selected)
				}
			default:
				// Now I will still resolve PHP so the summary can show what will be used,
				// even though I never asked (at most one compatible install).
				if preferred := system.PreferredPHPInstallation(compatiblePHPForSelection()); preferred != nil {
					opts.setPHP(*preferred)
				}
			}
		}

		// Now I will print a summary of every choice so the user can check it.
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
				localDomainValue = tui.GreenText.Render("https://" + proxy.LocalDomainHostname(opts.projectFolder, baseDomain))
			}
			fmt.Printf("  %s %s\n", labelStyle.Render("Local domain:"), localDomainValue)
		}
		fmt.Printf("  %s %s\n", labelStyle.Render("Git Repository:"), onOff(opts.initGit))
		fmt.Printf("  %s %s\n", labelStyle.Render("OpenSearch:"), onOff(opts.withElasticsearch))
		fmt.Printf("  %s %s\n", labelStyle.Render("AMQP:"), onOff(opts.withAMQP))
		fmt.Println()

		// Now I will ask: proceed with creation, restart this form, or cancel.
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
		// Now I will loop and show the form again ("Restart form").
	}
}
