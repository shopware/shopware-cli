package project

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	versionLatest = "latest"

	ciNone   = "none"
	ciGitHub = "github"
	ciGitLab = "gitlab"

	// projectNameHelp is the help text shown under the project name input.
	projectNameHelp = "The name of the project directory to create"
	// projectNameRule describes which characters are allowed in a project name.
	// It is shared between the up-front validation error and the live form hint
	// so both stay in sync.
	projectNameRule = "only lowercase letters, digits, dashes (-) and underscores (_) are allowed, and it must start with a lowercase letter or digit"
)

// composeProjectNameRegexp matches names that are valid as a Docker Compose
// project name. Docker Compose only allows lowercase letters, digits, dashes
// and underscores, and the name must start with a lowercase letter or digit.
// Anything else (uppercase letters, umlauts, spaces, dots, …) is rejected by
// Docker Compose once the generated Docker setup runs from the project
// directory, so we reject such project names up front.
var composeProjectNameRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validateProjectName ensures the project folder name can be used as a Docker
// Compose project name. Only the final path element is relevant, as that is
// what Docker Compose uses to derive the project name.
func validateProjectName(name string) error {
	base := filepath.Base(name)

	if !composeProjectNameRegexp.MatchString(base) {
		return fmt.Errorf("invalid project name %q: %s, so it can be used as a Docker Compose project name", base, projectNameRule)
	}

	return nil
}

// projectNameFieldDescription returns the description shown under the project
// name input in the interactive form. While the typed name is invalid it
// returns the rule highlighted in red, validating the input live; otherwise it
// returns the regular help text.
func projectNameFieldDescription(name string) string {
	if name != "" {
		if err := validateProjectName(name); err != nil {
			return tui.RedText.Render(projectNameRule)
		}
	}

	return projectNameHelp
}

type createOptions struct {
	projectFolder      string
	selectedVersion    string
	selectedDeployment string
	selectedCI         string
	useDocker          bool
	initGit            bool
	withElasticsearch  bool
	withAMQP           bool
	noAudit            bool

	interactive           bool
	elasticsearchExplicit bool
	isVerbose             bool
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
			releases, err := packagist.GetShopwarePackageVersions(cmd.Context())
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveNoFileComp
			}
			filteredVersions := filterInstallVersions(releases)
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
		opts := parseCreateFlags(cmd, args)

		if opts.interactive {
			tui.PrintBanner()
		}

		releases, err := packagist.GetShopwarePackageVersions(cmd.Context())
		if err != nil {
			return err
		}
		filteredVersions := filterInstallVersions(releases)

		if opts.interactive {
			if err := runCreateForm(cmd, &opts, filteredVersions); err != nil {
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

		if err := scaffoldProject(cmd.Context(), &opts, chosenVersion); err != nil {
			return err
		}

		return installAndFinalize(cmd, &opts, phpConstraint)
	},
}

func parseCreateFlags(cmd *cobra.Command, args []string) createOptions {
	useDocker, _ := cmd.PersistentFlags().GetBool("docker")
	withElasticsearch, _ := cmd.PersistentFlags().GetBool("with-elasticsearch")
	withAMQP, _ := cmd.PersistentFlags().GetBool("with-amqp")
	noAudit, _ := cmd.PersistentFlags().GetBool("no-audit")
	initGit, _ := cmd.PersistentFlags().GetBool("git")
	versionFlag, _ := cmd.PersistentFlags().GetString("version")
	deploymentMethod, _ := cmd.PersistentFlags().GetString("deployment")
	ciSystem, _ := cmd.PersistentFlags().GetString("ci")

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
		selectedVersion:       versionFlag,
		selectedDeployment:    deploymentMethod,
		selectedCI:            ciSystem,
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
		return fmt.Errorf("project name is required in non-interactive mode")
	}
	if opts.selectedVersion == "" {
		opts.selectedVersion = versionLatest
	}
	if opts.selectedDeployment == "" {
		opts.selectedDeployment = packagist.DeploymentNone
	}
	if opts.selectedCI == "" {
		opts.selectedCI = ciNone
	}
	if !opts.elasticsearchExplicit {
		opts.withElasticsearch = true
	}
	return nil
}

func init() {
	projectRootCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.PersistentFlags().Bool("docker", false, "Use Docker to run Composer instead of local installation")
	projectCreateCmd.PersistentFlags().Bool("with-elasticsearch", false, "Include Elasticsearch/OpenSearch support")
	projectCreateCmd.PersistentFlags().Bool("without-elasticsearch", false, "Remove Elasticsearch from the installation")
	_ = projectCreateCmd.PersistentFlags().MarkDeprecated("without-elasticsearch", "use --with-elasticsearch instead")
	projectCreateCmd.PersistentFlags().Bool("with-amqp", false, "Include AMQP queue support (symfony/amqp-messenger)")
	projectCreateCmd.PersistentFlags().Bool("no-audit", false, "Disable composer audit blocking insecure packages")
	projectCreateCmd.PersistentFlags().Bool("git", false, "Initialize a Git repository")
	projectCreateCmd.PersistentFlags().String("version", "", "Shopware version to install (e.g., 6.6.0.0, latest)")
	projectCreateCmd.PersistentFlags().String("deployment", "", "Deployment method: none, deployer, platformsh, shopware-paas")
	projectCreateCmd.PersistentFlags().String("ci", "", "CI/CD system: none, github, gitlab")
}
