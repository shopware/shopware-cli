package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectSbomCmd = &cobra.Command{
	Use:   "sbom [path]",
	Short: "Generate a CycloneDX SBOM from composer.lock",
	Long: `Generate a Software Bill of Materials (SBOM) for a Shopware project.

Reads composer.lock (and optionally composer.json for the root component name
and version) and writes a CycloneDX 1.7 JSON document — the same artifact that
project ci produces, without running the rest of the CI build.

Examples:
  # Write sbom.cdx.json into the current Shopware project
  shopware-cli project sbom

  # Explicit project path and output file
  shopware-cli project sbom ./my-shop \
    --format cyclonedx-json \
    --output sbom.json

The command is non-interactive and exits non-zero when generation fails
(missing or unreadable composer.lock, unsupported format, write errors).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveProjectSbomRoot(args)
		if err != nil {
			return err
		}

		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		if err := shop.ValidateProjectSBOMFormat(format); err != nil {
			return err
		}

		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		includeDev, err := cmd.Flags().GetBool("include-dev-dependencies")
		if err != nil {
			return err
		}

		return shop.WriteProjectSBOM(cmd.Context(), root, shop.ProjectSBOMOptions{
			OutputPath:             output,
			SkipMissingLock:        false,
			IncludeDevDependencies: includeDev,
			ToolVersion:            tui.AppVersion,
		})
	},
}

func init() {
	projectRootCmd.AddCommand(projectSbomCmd)
	projectSbomCmd.Flags().String("format", shop.ProjectSBOMFormatCycloneDXJSON, "SBOM format (only cyclonedx-json is supported)")
	projectSbomCmd.Flags().StringP("output", "o", "", fmt.Sprintf("Output file path (default: %s in the project root)", shop.DefaultProjectSBOMOutput))
	projectSbomCmd.Flags().Bool("include-dev-dependencies", false, "Include packages-dev from composer.lock (excluded by default, matching project ci)")
}

// resolveProjectSbomRoot picks the project directory: an explicit path argument,
// otherwise the closest Shopware project (composer.json/lock walk), falling back
// to the working directory when no Shopware markers are found further up.
func resolveProjectSbomRoot(args []string) (string, error) {
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}

	root, err := findClosestShopwareProject()
	if err == nil {
		return root, nil
	}

	// findClosestShopwareProject fails when no Shopware markers exist. Still
	// allow generating an SBOM from a plain composer project in cwd so the
	// command is useful outside full Shopware trees.
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return "", err
	}
	return cwd, nil
}
