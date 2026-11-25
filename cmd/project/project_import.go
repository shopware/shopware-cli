package project

import (
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectarchive"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

// unknownShopwareVersion is shown when the target version cannot be detected.
const unknownShopwareVersion = "unknown"

var projectImportCmd = &cobra.Command{
	Use:   "import archive",
	Short: "Replace the project database and files with an archive",
	Long: `Replace the database and the files of the public and private Shopware
filesystems of the current environment with an archive written by
"project export". Every component in the archive is applied unless skipped.

The database import recreates the tables contained in the archive, other tables
stay untouched. The media and thumbnail directories of the public filesystem and
the whole private filesystem are emptied before the archived files are copied.
Only the local filesystem adapter is supported, not S3 or other adapters.

The import replaces shop credentials such as the admin password, and live
integrations of the source shop become active on the target. A summary is shown
and confirmed before anything is written; pass --force to skip the
confirmation, which is required without an interactive terminal.`,
	Example: `  shopware-cli project import shop-export.tar.gz
  shopware-cli project import shop-export.tar.gz --dry-run
  shopware-cli project import shop-export.tar.gz -e staging --skip-private-files --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		archive := args[0]

		contents, err := projectarchive.Inspect(archive)
		if err != nil {
			return err
		}

		selected := selectedComponents(cmd)
		components := projectarchive.Components{
			Database:     contents.Database && selected.Database,
			PublicFiles:  contents.PublicFiles && selected.PublicFiles,
			PrivateFiles: contents.PrivateFiles && selected.PrivateFiles,
		}
		if !components.Any() {
			return fmt.Errorf("cannot import archive %s: every component it contains is skipped", archive)
		}

		projectRoot, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cmdExecutor, envCfg, err := resolveEnvironmentExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		opts := projectarchive.ImportOptions{
			Archive:     archive,
			Components:  components,
			Environment: cmdExecutor,
		}

		if components.PublicFiles || components.PrivateFiles {
			if opts.Filesystems, err = projectarchive.ResolveFilesystems(cmd.Context(), cmdExecutor); err != nil {
				return err
			}
		}

		targetVersion, err := projectarchive.ShopwareVersion(cmd.Context(), cmdExecutor)
		if err != nil {
			logging.FromContext(cmd.Context()).Warnf("Cannot compare the Shopware versions: %v", err)
			targetVersion = unknownShopwareVersion
		}

		target := describeTarget(cmdExecutor, envCfg, projectRoot)
		printImportPlan(cmd.OutOrStdout(), archive, target, targetVersion, contents, components)

		if source := contents.Metadata.Source.ShopwareVersion; targetVersion != unknownShopwareVersion && !projectarchive.SameShopwareMinor(source, targetVersion) {
			printWarning(cmd.OutOrStdout(),
				fmt.Sprintf("The archive is from Shopware %s, the target runs Shopware %s", source, targetVersion),
				"The import may fail or leave the shop inconsistent")
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return nil
		}

		if err := confirmImport(cmd, target); err != nil {
			return err
		}

		if components.Database {
			dbConn, err := cmdExecutor.DatabaseConnection(cmd.Context())
			if err != nil {
				return err
			}
			conn, cleanup, err := dbConn.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			opts.Database = conn
		}

		if err := projectarchive.Import(cmd.Context(), opts); err != nil {
			return err
		}

		printImportNextSteps(cmd.OutOrStdout(), components)

		return nil
	},
}

// describeTarget names the environment an import writes to.
func describeTarget(cmdExecutor executor.Executor, envCfg *shop.EnvironmentConfig, projectRoot string) string {
	name := environmentName
	if name == "" {
		name = "local"
	}

	location := projectRoot
	if envCfg != nil && envCfg.SSH != nil {
		location = envCfg.SSH.Host + ":" + envCfg.SSH.Directory
		if envCfg.SSH.User != "" {
			location = envCfg.SSH.User + "@" + location
		}
	}

	return fmt.Sprintf("environment %q (%s, %s)", name, cmdExecutor.Type(), location)
}

func printImportPlan(w io.Writer, archive, target, targetVersion string, contents *projectarchive.Contents, components projectarchive.Components) {
	lines := []struct {
		label     string
		inArchive bool
		applied   bool
		action    string
	}{
		{"Database", contents.Database, components.Database, "replace the tables included in the archive"},
		{"Public files", contents.PublicFiles, components.PublicFiles, "replace media and thumbnails"},
		{"Private files", contents.PrivateFiles, components.PrivateFiles, "replace all private files"},
	}

	_, _ = fmt.Fprintf(w, "Archive: %s (Shopware %s, created %s)\n", archive, contents.Metadata.Source.ShopwareVersion, contents.Metadata.CreatedAt.Format("2006-01-02 15:04 MST"))
	_, _ = fmt.Fprintf(w, "Target: %s (Shopware %s)\n\n", tui.BoldText.Render(target), targetVersion)

	for _, line := range lines {
		switch {
		case line.applied:
			_, _ = fmt.Fprintf(w, "%s: %s\n", line.label, line.action)
		case line.inArchive:
			_, _ = fmt.Fprintln(w, tui.DimText.Render(line.label+": skipped, left untouched"))
		default:
			_, _ = fmt.Fprintln(w, tui.DimText.Render(line.label+": not in the archive, left untouched"))
		}
	}

	if components.Database {
		printWarning(w, "Import replaces shop credentials such as the admin password",
			"Review credentials and live integrations (payment, shipping, apps) before using the copy")
	}
}

func printWarning(w io.Writer, message, hint string) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, tui.YellowText.Render("⚠ "+message))
	_, _ = fmt.Fprintln(w, "  "+hint)
}

func confirmImport(cmd *cobra.Command, target string) error {
	if force, _ := cmd.Flags().GetBool("force"); force {
		return nil
	}

	if !system.IsInteractionEnabled(cmd.Context()) {
		return errors.New("cannot import without confirmation: interaction is disabled (pass --force to replace the data)")
	}

	confirmed := false
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[bool]().
			Title(fmt.Sprintf("Replace data in %s?", target)).
			Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).
			Value(&confirmed),
	)).WithTheme(tui.ShopwareTheme()).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr()).RunWithContext(cmd.Context())
	if err != nil && !errors.Is(err, huh.ErrUserAborted) {
		return err
	}

	if !confirmed {
		return tracking.NewCancelled("import cancelled")
	}

	return nil
}

func printImportNextSteps(w io.Writer, components projectarchive.Components) {
	command := "shopware-cli project console"
	if environmentName != "" {
		command = fmt.Sprintf("shopware-cli project -e %s console", environmentName)
	}

	steps := []string{command + " cache:clear"}
	if components.Database {
		steps = append(steps,
			command+" dal:refresh:index",
			command+" theme:compile",
			command+" es:index (when Elasticsearch or OpenSearch is used)",
		)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, tui.GreenText.Render("✓")+" Import completed. Next steps:")
	for _, step := range steps {
		_, _ = fmt.Fprintln(w, "  "+step)
	}
	if components.Database {
		_, _ = fmt.Fprintln(w, "  Log in with the admin credentials of the source shop")
	}
}

func init() {
	projectRootCmd.AddCommand(projectImportCmd)

	registerSkipComponentFlags(projectImportCmd)
	projectImportCmd.Flags().Bool("dry-run", false, "Show what would be replaced, without changing anything")
	projectImportCmd.Flags().Bool("force", false, "Replace the data without confirmation")
}
