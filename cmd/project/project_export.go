package project

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/projectarchive"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// exportTimestampLayout formats the start of an export in UTC in the default
// output name, marked with the ISO 8601 designator Z. It sorts chronologically
// and avoids characters invalid on Windows.
const exportTimestampLayout = "2006-01-02_15-04-05Z"

const (
	skipDatabaseFlag     = "skip-database"
	skipPublicFilesFlag  = "skip-public-files"
	skipPrivateFilesFlag = "skip-private-files"
)

var projectExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the project database and Shopware managed files into an archive",
	Long: `Export the database and the files of the public and private Shopware
filesystems of the current environment into one archive, which "project import"
restores. Code and extensions are not part of the archive.

From the public filesystem only media and thumbnails are exported, from the
private filesystem everything, e.g. invoices and documents. For now only the local
filesystem adapter is supported, not S3 or other adapters.

Skipping files also excludes their database rows: --skip-public-files removes
public media like product images, --skip-private-files private media like order documents.
DB rows referencing them are unlinked if their foreign key allows it, otherwise
removed.

The dump flags only shape the database. Files are exported as stored and are
never anonymized. Do not remove or replace media while exporting, the archive
may miss files otherwise. The archive is staged next to the output file, which
needs free space for about twice its size.`,
	Example: `  shopware-cli project export
  shopware-cli project export --anonymize --output anonymized.tar.gz
  shopware-cli project export -e production --clean --skip-private-files`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		components := selectedComponents(cmd)
		if !components.Any() {
			return fmt.Errorf("cannot export: every component is skipped, remove one of --%s, --%s, --%s", skipDatabaseFlag, skipPublicFilesFlag, skipPrivateFilesFlag)
		}

		createdAt := time.Now().UTC()
		output, _ := cmd.Flags().GetString("output")
		if output == "" {
			output = "shopware-export-" + createdAt.Format(exportTimestampLayout) + ".tar.gz"
		}

		projectRoot, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		opts := projectarchive.ExportOptions{
			Output:      output,
			Components:  components,
			Environment: cmdExecutor,
			CreatedAt:   createdAt,
		}

		if opts.FileRoots, err = projectarchive.ResolveFileRoots(cmd.Context(), cmdExecutor, components); err != nil {
			return err
		}

		if opts.ShopwareVersion, err = projectarchive.ShopwareVersion(cmd.Context(), cmdExecutor); err != nil {
			return err
		}

		if components.Database {
			if opts.Database.Connection, err = cmdExecutor.DatabaseConnection(cmd.Context()); err != nil {
				return err
			}
			if opts.Database.Options, err = dumpDataOptions(cmd); err != nil {
				return err
			}
			opts.Database.Config = cmdExecutor.ShopConfig().ConfigDump
		}

		if err := projectarchive.Export(cmd.Context(), opts); err != nil {
			return err
		}

		printExportSummary(cmd.OutOrStdout(), components, opts.Database.Options.Anonymize, output)

		return nil
	},
}

// selectedComponents reads the skip flags of the export.
func selectedComponents(cmd *cobra.Command) projectarchive.Components {
	skipDatabase, _ := cmd.Flags().GetBool(skipDatabaseFlag)
	skipPublicFiles, _ := cmd.Flags().GetBool(skipPublicFilesFlag)
	skipPrivateFiles, _ := cmd.Flags().GetBool(skipPrivateFilesFlag)

	return projectarchive.Components{
		Database:     !skipDatabase,
		PublicFiles:  !skipPublicFiles,
		PrivateFiles: !skipPrivateFiles,
	}
}

func printExportSummary(w io.Writer, components projectarchive.Components, anonymized bool, output string) {
	const included, skipped = "included", "skipped"

	database := included
	if anonymized {
		database = included + ", anonymized"
	}

	filesSkipped := skipped
	if components.Database {
		filesSkipped = skipped + ", their database rows are excluded"
	}

	lines := []struct {
		label    string
		included bool
		state    string
		skipped  string
	}{
		{"Database", components.Database, database, skipped},
		{"Public files", components.PublicFiles, included, filesSkipped},
		{"Private files", components.PrivateFiles, included, filesSkipped},
	}

	_, _ = fmt.Fprintln(w)
	for _, line := range lines {
		if !line.included {
			_, _ = fmt.Fprintln(w, tui.DimText.Render(fmt.Sprintf("- %s: %s", line.label, line.skipped)))
			continue
		}
		_, _ = fmt.Fprintf(w, "%s %s: %s\n", tui.CheckOK(), line.label, line.state)
	}
	_, _ = fmt.Fprintln(w, tui.CheckOK()+" Archive created: "+output)

	if components.PublicFiles || components.PrivateFiles {
		printWarning(w, "Files are not anonymized and may contain sensitive data", "")
	}
}

func init() {
	projectRootCmd.AddCommand(projectExportCmd)

	projectExportCmd.Flags().String("output", "", "Path of the archive (default: shopware-export-<YYYY-MM-DD_HH-MM-SS>Z.tar.gz in UTC)")
	projectExportCmd.Flags().Bool(skipDatabaseFlag, false, "Skip the database")
	projectExportCmd.Flags().Bool(skipPublicFilesFlag, false, "Skip the media and thumbnails of the public filesystem")
	projectExportCmd.Flags().Bool(skipPrivateFilesFlag, false, "Skip the files of the private filesystem")
	registerDumpDataFlags(projectExportCmd)
}
