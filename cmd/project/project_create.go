package project

import (
	"fmt"

	"github.com/shyim/go-composer/repository"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	// projectNameHelp is the help text shown under the project name input.
	projectNameHelp = "The name of the project directory to create (leave empty to use the current directory)"
)

type createOptions struct {
	projectFolder      string
	selectedVersion    string
	selectedDeployment string
	selectedCI         string
	// phpVersion is the major.minor PHP series the project uses: the Docker image
	// tag for Docker projects, the local PHP lookup otherwise. Persisted as-is.
	phpVersion string
	// phpVersionExplicit records that --php-version was passed, so the creation
	// form does not ask again.
	phpVersionExplicit bool
	// phpBinary is the local executable phpVersion resolved to; never persisted.
	phpBinary         string
	useDocker         bool
	initGit           bool
	withElasticsearch bool
	withAMQP          bool
	noAudit           bool
	// useLocalDomain serves the shop at a stable hostname
	// (<name>.<baseDomain>) through the shared proxy instead of a fixed port.
	// Only meaningful with Docker.
	useLocalDomain bool
	// setupProxyNow runs the one-time machine setup (DNS + HTTPS trust, needs
	// sudo) inline during create, so the local domain works immediately. Set
	// only when the user opts in and the machine is not configured yet.
	setupProxyNow bool

	interactive           bool
	elasticsearchExplicit bool
	isVerbose             bool
}

func (o *createOptions) setPHP(installation system.PHPInstallation) {
	o.phpBinary = installation.Binary
	o.phpVersion = system.PHPVersionPin(installation.Version)
}

// clearPHP drops a resolved local PHP, e.g. when the form switches to Docker. An
// explicit --php-version is kept, since it applies to Docker projects too.
func (o *createOptions) clearPHP() {
	o.phpBinary = ""
	if !o.phpVersionExplicit {
		o.phpVersion = ""
	}
}

// resolveLocalDomainChoice derives the final local-domain settings from the
// individual inputs. Local domains require Docker, so useLocalDomain is always
// gated on useDocker regardless of how the choice was made (interactive or the
// --local-domain flag). setupProxyNow — which triggers the one-time sudo setup
// inline — is only ever true when the choice came from the interactive prompt
// (promptShown), so passing --local-domain never runs sudo without asking.
func resolveLocalDomainChoice(useDocker, wantLocalDomain, promptShown, machineSetupDone, setupNowAnswer bool) (useLocalDomain, setupProxyNow bool) {
	useLocalDomain = useDocker && wantLocalDomain
	setupProxyNow = useLocalDomain && promptShown && !machineSetupDone && setupNowAnswer
	return useLocalDomain, setupProxyNow
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [name] [version]",
	Short: "Create a new Shopware 6 project",
	Args:  cobra.MaximumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{}, cobra.ShellCompDirectiveFilterDirs
		}

		if len(args) == 1 {
			pkg, err := repository.New(packagistURL, nil).GetPackage(cmd.Context(), "shopware/core")
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveNoFileComp
			}
			filteredVersions := shop.FilterInstallVersions(pkg.Versions)
			versions := make([]string, 0, len(filteredVersions)+1)
			versions = append(versions, shop.VersionLatest)
			for _, v := range filteredVersions {
				versions = append(versions, v.String())
			}
			return versions, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{}, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := parseCreateFlags(cmd, args)

		if opts.phpVersionExplicit {
			if err := shop.ValidatePHPVersion(opts.phpVersion); err != nil {
				return err
			}
		}

		if opts.interactive {
			tui.PrintBanner()
		}

		pkg, err := repository.New(packagistURL, nil).GetPackage(cmd.Context(), "shopware/core")
		if err != nil {
			return err
		}
		releases := pkg.Versions
		filteredVersions := shop.FilterInstallVersions(releases)

		if opts.interactive {
			if err := runCreateForm(cmd, &opts, releases, filteredVersions); err != nil {
				return err
			}
		} else {
			if err := applyNonInteractiveDefaults(&opts); err != nil {
				return err
			}
		}

		chosenVersion, phpConstraint, err := validateAndPreflight(cmd.Context(), &opts, releases, filteredVersions)
		if err != nil {
			return err
		}

		// Do the one-time machine setup up front (while the user is still at the
		// keyboard for the sudo prompt), before the long composer install. It is
		// best-effort: a blocked/declined sudo just means the domain resolves
		// once the user runs `project proxy setup` later.
		if opts.setupProxyNow {
			fmt.Println()
			fmt.Println(tui.BoldText.Render("Setting up local domains (one-time, needs sudo)"))
			_ = runInlineProxySetup(cmd.Context(), proxy.BaseDomain())
		}

		if err := scaffoldProject(cmd.Context(), &opts, chosenVersion); err != nil {
			return err
		}

		return installAndFinalize(cmd, &opts, phpConstraint, chosenVersion)
	},
}

// packagistURL is a package variable so tests can point the create flow at a
// local repository server instead of Packagist.
var packagistURL = repository.PackagistURL

func parseCreateFlags(cmd *cobra.Command, args []string) createOptions {
	useDocker, _ := cmd.PersistentFlags().GetBool("docker")
	withElasticsearch, _ := cmd.PersistentFlags().GetBool("with-elasticsearch")
	withAMQP, _ := cmd.PersistentFlags().GetBool("with-amqp")
	noAudit, _ := cmd.PersistentFlags().GetBool("no-audit")
	initGit, _ := cmd.PersistentFlags().GetBool("git")
	localDomain, _ := cmd.PersistentFlags().GetBool("local-domain")
	versionFlag, _ := cmd.PersistentFlags().GetString("version")
	deploymentMethod, _ := cmd.PersistentFlags().GetString("deployment")
	ciSystem, _ := cmd.PersistentFlags().GetString("ci")
	phpVersion, _ := cmd.PersistentFlags().GetString("php-version")

	if cmd.PersistentFlags().Changed("without-elasticsearch") {
		withoutElasticsearch, _ := cmd.PersistentFlags().GetBool("without-elasticsearch")
		withElasticsearch = !withoutElasticsearch
	}
	elasticsearchExplicit := cmd.PersistentFlags().Changed("with-elasticsearch") || cmd.PersistentFlags().Changed("without-elasticsearch")

	isVerbose, _ := cmd.Flags().GetBool("verbose")

	opts := createOptions{
		useDocker:             useDocker,
		withElasticsearch:     withElasticsearch,
		withAMQP:              withAMQP,
		noAudit:               noAudit,
		initGit:               initGit,
		useLocalDomain:        localDomain,
		selectedVersion:       versionFlag,
		selectedDeployment:    deploymentMethod,
		selectedCI:            ciSystem,
		phpVersion:            phpVersion,
		phpVersionExplicit:    cmd.PersistentFlags().Changed("php-version"),
		interactive:           system.IsInteractionEnabled(cmd.Context()),
		elasticsearchExplicit: elasticsearchExplicit,
		isVerbose:             isVerbose,
	}

	if len(args) > 0 {
		opts.projectFolder = args[0]
	}
	if len(args) > 1 && opts.selectedVersion == "" {
		opts.selectedVersion = args[1]
	}

	return opts
}

func applyNonInteractiveDefaults(opts *createOptions) error {
	if opts.projectFolder == "" {
		opts.projectFolder = "."
	}
	if opts.selectedVersion == "" {
		opts.selectedVersion = shop.VersionLatest
	}
	if opts.selectedDeployment == "" {
		opts.selectedDeployment = shop.DeploymentNone
	}
	if opts.selectedCI == "" {
		opts.selectedCI = shop.CINone
	}
	if !opts.elasticsearchExplicit {
		opts.withElasticsearch = true
	}
	// Local domains need Docker; drop the flag if Docker is off. Never run the
	// one-time sudo setup non-interactively.
	opts.useLocalDomain = opts.useDocker && opts.useLocalDomain
	opts.setupProxyNow = false
	return nil
}

func init() {
	projectRootCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.PersistentFlags().Bool("docker", false, "Use Docker for local setup.")
	projectCreateCmd.PersistentFlags().Bool("with-elasticsearch", false, "Include Elasticsearch/OpenSearch support")
	projectCreateCmd.PersistentFlags().Bool("without-elasticsearch", false, "Remove Elasticsearch from the installation")
	_ = projectCreateCmd.PersistentFlags().MarkDeprecated("without-elasticsearch", "use --with-elasticsearch instead")
	projectCreateCmd.PersistentFlags().Bool("with-amqp", false, "Include AMQP queue support (symfony/amqp-messenger)")
	projectCreateCmd.PersistentFlags().Bool("no-audit", false, "Disable composer audit blocking insecure packages")
	projectCreateCmd.PersistentFlags().Bool("git", false, "Initialize a Git repository")
	projectCreateCmd.PersistentFlags().Bool("local-domain", false, "Serve the shop at a stable local hostname (<name>.shopware.local) via the shared proxy instead of a port (requires Docker)")
	projectCreateCmd.PersistentFlags().String("version", "", "Shopware version to install (e.g., 6.6.0.0, latest)")
	projectCreateCmd.PersistentFlags().String("deployment", "", "Deployment method: none, deployer, platformsh, shopware-paas")
	projectCreateCmd.PersistentFlags().String("ci", "", "CI/CD system: none, github, gitlab")
	projectCreateCmd.PersistentFlags().String("php-version", "", "PHP version to use (e.g. 8.3); selects the local PHP for local projects and the image tag for --docker projects")
	_ = projectCreateCmd.RegisterFlagCompletionFunc("php-version", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return shop.SupportedPHPVersions, cobra.ShellCompDirectiveNoFileComp
	})
}
