package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/environment"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectConfigEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage the environments of the project config",
	Long: `Open a guided menu to add, edit, test and remove the environments of the
project config.

Each action is also available as a subcommand for scripts. Test results are
remembered per machine and shown as READY or NOT READY.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("cannot open the environments menu without interaction: use the list, add, edit, test or remove subcommand")
		}
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}
		return newEnvWizard(cmd, project.configPath).menu()
	},
}

var projectConfigEnvListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List the environments of the project config",
	Long: `List every environment with its type, target and the result of its last test.

Run "shopware-cli project config env test" to refresh the status.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		formatName, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		format, err := tui.ParseTableFormat(formatName)
		if err != nil {
			return err
		}
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}

		table := tui.NewTable(
			tui.TableColumn{Title: "Environment", JSONKey: "name"},
			tui.TableColumn{Title: "Type", JSONKey: "type"},
			tui.TableColumn{Title: "Status", JSONKey: "status"},
			tui.TableColumn{Title: "Last test", JSONKey: "last_test"},
			tui.TableColumn{Title: "Target", JSONKey: "target"},
		)
		for _, overview := range environment.Overviews(project.cfg, project.store, time.Now()) {
			status := "not_ready"
			if overview.Ready {
				status = "ready"
			}
			target := strings.TrimPrefix(environment.Target(overview.Config), overview.Type+" · ")
			row := map[string]any{
				"name":      overview.Name,
				"type":      overview.Type,
				"status":    status,
				"limited":   overview.Limited,
				"last_test": overview.LastTest,
				"tested_at": nil,
				"target":    target,
			}
			if !overview.TestedAt.IsZero() {
				row["tested_at"] = overview.TestedAt
			}
			table.AddRowWithJSON(row, overview.Name, overview.Type, envStatusLabel(overview.Ready, overview.Limited), overview.LastTest, target)
		}
		return table.Write(cmd.OutOrStdout(), format)
	},
}

var projectConfigEnvAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add an SSH environment to the project config",
	Long: `Add an environment that points at a Shopware project on a remote host.

Asks for every value that is not passed. Pass the name, --host and --directory
to add the environment without prompts.`,
	Example: `  shopware-cli project config env add preprod --url https://preprod.example.com --host preprod.example.com --directory /var/www/shop/current`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}

		var input environment.Settings
		if len(args) == 1 {
			input.Name = args[0]
		}
		input.URL, _ = cmd.Flags().GetString("url")
		input.Host, _ = cmd.Flags().GetString("host")
		input.Directory, _ = cmd.Flags().GetString("directory")
		input.User, _ = cmd.Flags().GetString("user")
		input.Port, _ = cmd.Flags().GetInt("port")
		input.IdentityFile, _ = cmd.Flags().GetString("identity-file")
		input.PHPBinary, _ = cmd.Flags().GetString("php-binary")
		test, _ := cmd.Flags().GetBool("test")

		if input.Name == "" || input.Host == "" || input.Directory == "" {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return errors.New("cannot add environment without interaction: pass the name, --host and --directory")
			}
			wizard := newEnvWizard(cmd, project.configPath)
			return wizard.direct(wizard.add(input, cmd.Flags().Changed("test"), test))
		}

		if err := environment.ValidateHost(input.Host); err != nil {
			return fmt.Errorf("cannot add environment %s: --host: %w", input.Name, err)
		}
		if err := shop.AddEnvironment(project.configPath, input.Name, input.Config()); err != nil {
			return err
		}
		logging.FromContext(cmd.Context()).Infof("Added environment %s to %s", input.Name, project.configPath)

		if !test {
			return nil
		}
		return testEnvironments(cmd, project.configPath, []string{input.Name})
	},
}

var projectConfigEnvEditCmd = &cobra.Command{
	Use:   "edit [name]",
	Short: "Change an environment of the project config",
	Long: `Change the settings of an environment and keep the rest of the project config
as it is.

Asks what to change when no setting is passed. An empty value removes the
setting, so the default applies again. The environment counts as untested
afterwards.`,
	Example: `  shopware-cli project config env edit production --host new.example.com --port 2222`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}

		name := environmentName
		if len(args) == 1 {
			name = args[0]
		}
		change := envChangeFromFlags(cmd)

		if name == "" || change.Empty() {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return errors.New("cannot edit environment without interaction: pass the name and the settings to change, e.g. --host")
			}
			wizard := newEnvWizard(cmd, project.configPath)
			if name == "" {
				if name, err = wizard.pick("edit", ""); err != nil {
					return wizard.direct(err)
				}
			}
			if change.Empty() {
				return wizard.direct(wizard.edit(name, ""))
			}
		}

		if _, err := project.cfg.ResolveEnvironment(name); err != nil {
			return err
		}
		if change.Host != nil {
			if err := environment.ValidateHost(*change.Host); err != nil {
				return fmt.Errorf("cannot edit environment %s: --host: %w", name, err)
			}
		}
		if err := shop.UpdateEnvironment(project.configPath, name, change); err != nil {
			return err
		}
		if change.Name != nil && *change.Name != name {
			if err := project.store.Rename(name, *change.Name); err != nil {
				return err
			}
			name = *change.Name
		}
		logging.FromContext(cmd.Context()).Infof("Saved environment %s to %s", name, project.configPath)
		return nil
	},
}

var projectConfigEnvTestCmd = &cobra.Command{
	Use:   "test [name...]",
	Short: "Test the connection, configuration and capabilities of environments",
	Long: `Connect to each environment and check that its directory is a Shopware project
and which commands work against it.

SSH connections are opened without prompts, so authentication has to work
through a key file or the ssh agent. The command fails when an environment is
NOT READY.`,
	Example: `  shopware-cli project config env test production staging
  shopware-cli project config env test --all`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}

		names := args
		if all, _ := cmd.Flags().GetBool("all"); all {
			if len(args) > 0 {
				return errors.New("cannot test environments: pass names or --all, not both")
			}
			names = project.names()
		} else if len(names) == 0 && environmentName != "" {
			names = []string{environmentName}
		}

		if len(names) == 0 {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return errors.New("cannot test environments without interaction: pass environment names or --all")
			}
			wizard := newEnvWizard(cmd, project.configPath)
			return wizard.direct(wizard.test(""))
		}

		return testEnvironments(cmd, project.configPath, names)
	},
}

var projectConfigEnvRemoveCmd = &cobra.Command{
	Use:     "remove [name...]",
	Aliases: []string{"rm"},
	Short:   "Remove environments from the project config",
	Long: `Remove environments from the project config.

The remote shop and its files are not touched.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := loadEnvProject(cmd)
		if err != nil {
			return err
		}

		names := args
		if len(names) == 0 && environmentName != "" {
			names = []string{environmentName}
		}
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return errors.New("cannot remove environments without interaction: pass environment names and --force")
			}
			wizard := newEnvWizard(cmd, project.configPath)
			if len(names) == 0 {
				return wizard.direct(wizard.remove(""))
			}
			// Each environment is confirmed on its own.
			for _, name := range names {
				if err := wizard.remove(name); err != nil {
					return wizard.direct(err)
				}
			}
			return nil
		}

		if len(names) == 0 {
			return errors.New("cannot remove environments: pass the names to remove")
		}
		for _, name := range names {
			if err := removeEnvironment(project, name); err != nil {
				return err
			}
			logging.FromContext(cmd.Context()).Infof("Removed environment %s from %s", name, project.configPath)
		}
		return nil
	},
}

func init() {
	projectConfigCmd.AddCommand(projectConfigEnvCmd)
	projectConfigEnvCmd.AddCommand(projectConfigEnvListCmd, projectConfigEnvAddCmd, projectConfigEnvEditCmd, projectConfigEnvTestCmd, projectConfigEnvRemoveCmd)

	projectConfigEnvListCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table, json)")

	for _, cmd := range []*cobra.Command{projectConfigEnvAddCmd, projectConfigEnvEditCmd} {
		cmd.Flags().String("url", "", "Shop URL of the environment")
		cmd.Flags().String("host", "", "SSH host name or IP address")
		cmd.Flags().String("directory", "", "Absolute path of the Shopware project on the remote host")
		cmd.Flags().String("user", "", "SSH user")
		cmd.Flags().Int("port", 0, "SSH port")
		cmd.Flags().String("identity-file", "", "Path to the SSH private key file")
		cmd.Flags().String("php-binary", "", "PHP binary on the remote host")
	}
	projectConfigEnvAddCmd.Flags().Bool("test", false, "Test the environment after adding it")
	projectConfigEnvEditCmd.Flags().String("name", "", "New name of the environment")
	projectConfigEnvTestCmd.Flags().Bool("all", false, "Test every environment of the project config")
	projectConfigEnvRemoveCmd.Flags().Bool("force", false, "Remove without confirmation")

	for _, cmd := range []*cobra.Command{projectConfigEnvEditCmd, projectConfigEnvTestCmd, projectConfigEnvRemoveCmd} {
		cmd.ValidArgsFunction = completeEnvironmentNames
	}
}

// envProject is the project config whose environments are managed, together
// with the test results remembered for it.
type envProject struct {
	configPath string
	cfg        *shop.Config
	store      *environment.Store
}

func loadEnvProject(cmd *cobra.Command) (*envProject, error) {
	projectRoot, err := shop.FindClosestShopwareProject(true)
	if err != nil {
		return nil, err
	}
	return readEnvProject(cmd.Context(), shop.SearchConfigPath(cmd.Context(), projectRoot, projectConfigPath))
}

func readEnvProject(ctx context.Context, configPath string) (*envProject, error) {
	cfg, err := shop.ReadConfig(ctx, configPath, false)
	if err != nil {
		return nil, err
	}
	return &envProject{configPath: configPath, cfg: cfg, store: environment.OpenStore(configPath)}, nil
}

func (p *envProject) names() []string {
	names := make([]string, 0, len(p.cfg.Environments))
	for name, env := range p.cfg.Environments {
		if env != nil {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func removeEnvironment(project *envProject, name string) error {
	if err := shop.RemoveEnvironment(project.configPath, name); err != nil {
		return err
	}
	return project.store.Forget(name)
}

// errEnvNotReady exits non-zero after the failed checks were printed.
var errEnvNotReady = logging.NewSilent("environment is not ready")

// testEnvironments tests the named environments, prints each result and
// remembers it. It fails when an environment is not ready.
func testEnvironments(cmd *cobra.Command, configPath string, names []string) error {
	results, err := runEnvironmentTests(cmd, configPath, names)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(results, func(result environment.Result) bool { return !result.Ready }) {
		return errEnvNotReady
	}
	return nil
}

func runEnvironmentTests(cmd *cobra.Command, configPath string, names []string) ([]environment.Result, error) {
	project, err := readEnvProject(cmd.Context(), configPath)
	if err != nil {
		return nil, err
	}

	// Resolve every name first, so a typo fails before the first connection.
	environments := make([]*shop.EnvironmentConfig, 0, len(names))
	for _, name := range names {
		env, err := project.cfg.ResolveEnvironment(name)
		if err != nil {
			return nil, err
		}
		environments = append(environments, env)
	}

	results := make([]environment.Result, 0, len(names))
	for i, name := range names {
		var result environment.Result
		err := runStep(cmd.Context(), fmt.Sprintf("Testing %s...", name), func(ctx context.Context) error {
			result = environment.Test(ctx, name, environments[i], executor.ProbeSSH)
			return ctx.Err()
		})
		if err != nil {
			return results, err
		}
		if err := project.store.Record(environments[i], result); err != nil {
			return results, err
		}
		writeEnvResult(cmd.OutOrStdout(), result)
		results = append(results, result)
	}
	return results, nil
}

func envStatusLabel(ready, limited bool) string {
	switch {
	case !ready:
		return tui.YellowText.Render("NOT READY")
	case limited:
		return tui.GreenText.Render("READY") + ", limited"
	default:
		return tui.GreenText.Render("READY")
	}
}

func writeEnvResult(w io.Writer, result environment.Result) {
	headline := tui.CheckOK()
	switch {
	case !result.Ready:
		headline = tui.CheckFail()
	case result.Limited:
		headline = tui.CheckWarn()
	}
	_, _ = fmt.Fprintf(w, "%s %s is %s\n", headline, tui.BoldText.Render(result.Name), envStatusLabel(result.Ready, result.Limited))
	_, _ = fmt.Fprintf(w, "  %s\n", tui.DimText.Render(result.Target))

	for _, check := range result.Checks {
		glyph, summary := tui.DimText.Render("○"), tui.DimText.Render(check.Summary)
		switch check.State {
		case environment.CheckOK:
			glyph, summary = tui.CheckOK(), check.Summary
		case environment.CheckWarn:
			glyph, summary = tui.YellowText.Render("!"), check.Summary
		case environment.CheckFail:
			glyph, summary = tui.CheckFail(), check.Summary
		case environment.CheckSkipped:
		}
		_, _ = fmt.Fprintf(w, "  %s %-14s %s\n", glyph, check.Name, summary)
		if check.Detail != "" {
			_, _ = fmt.Fprintf(w, "    %-14s %s\n", "", tui.DimText.Render(check.Detail))
		}
	}

	if result.Hint != "" {
		_, _ = fmt.Fprintf(w, "\n  %s\n", result.Hint)
	}
	_, _ = fmt.Fprintln(w)
}

// envChangeFromFlags builds a change from the flags that were passed, so an
// explicitly empty value removes the setting.
func envChangeFromFlags(cmd *cobra.Command) shop.EnvironmentChange {
	var change shop.EnvironmentChange
	for flag, target := range map[string]**string{
		"name":          &change.Name,
		"url":           &change.URL,
		"host":          &change.Host,
		"directory":     &change.Directory,
		"user":          &change.User,
		"identity-file": &change.IdentityFile,
		"php-binary":    &change.PHPBinary,
	} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			*target = &value
		}
	}
	if cmd.Flags().Changed("port") {
		port, _ := cmd.Flags().GetInt("port")
		change.Port = &port
	}
	return change
}
