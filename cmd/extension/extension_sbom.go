package extension

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/cliversion"
	"github.com/shopware/shopware-cli/internal/extension"
)

const extensionSBOMFormatCycloneDXJSON = "cyclonedx-json"

var extensionSbomCmd = &cobra.Command{
	Use:   "sbom path",
	Short: "Generate a CycloneDX SBOM for an extension",
	Long: `Generate a Software Bill of Materials (SBOM) for a single extension.

The extension itself is the root component. Bundled components are what is
physically present in the extension folder: composer packages from
vendor/composer/installed.json and production npm packages from the
package-lock.json files of the Administration and Storefront sources.

Composer requirements that are not bundled, such as shopware/core, are
installed by the shop. They are listed as external components (CycloneDX
isExternal) with the required version range instead of a resolved version.

The working copy is read as it is. To describe exactly what is shipped, use
extension package --sbom, which generates the SBOM from the packaged files.`,
	Example: `  # Write sbom.cdx.json into the extension folder
  shopware-cli extension sbom ./custom/plugins/MyPlugin

  # Explicit output file
  shopware-cli extension sbom ./custom/plugins/MyPlugin --output dist/MyPlugin.cdx.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(format)) != extensionSBOMFormatCycloneDXJSON {
			return fmt.Errorf("cannot generate SBOM: unsupported format %q (cyclonedx-json)", format)
		}

		extPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), extPath)
		if err != nil {
			return fmt.Errorf("cannot detect extension type: %w", err)
		}

		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		if output == "" {
			output = filepath.Join(extPath, extension.DefaultExtensionSBOMOutput)
		} else if output, err = filepath.Abs(output); err != nil {
			return err
		}

		return extension.WriteSBOM(cmd.Context(), ext, extPath, extension.SBOMOptions{ToolVersion: cliversion.Version}, output)
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionSbomCmd)
	extensionSbomCmd.Flags().String("format", extensionSBOMFormatCycloneDXJSON, "SBOM format (only cyclonedx-json is supported)")
	_ = extensionSbomCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions([]string{extensionSBOMFormatCycloneDXJSON}, cobra.ShellCompDirectiveNoFileComp))
	extensionSbomCmd.Flags().StringP("output", "o", "", fmt.Sprintf("Output file path (default: %s in the extension folder)", extension.DefaultExtensionSBOMOutput))
}
