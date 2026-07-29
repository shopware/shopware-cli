package extension

import (
	"fmt"
	"path/filepath"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var extensionConfigInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Create an empty .shopware-extension.yml",
	Long: `Create an empty .shopware-extension.yml for an extension checkout.

All configuration keys are optional. The file only contains a yaml-language-server
schema comment so editors can offer completion and validation. Add keys as needed
(see: shopware-cli extension config-schema).

Examples:
  shopware-cli extension config init
  shopware-cli extension config init ./my-extension
  shopware-cli extension config init --force
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return err
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force && extension.ConfigExists(abs) && system.IsInteractionEnabled(cmd.Context()) {
			overwrite := false
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("%s already exists. Overwrite?", extension.ConfigFileName)).
						Value(&overwrite),
				),
			)
			if err := form.Run(); err != nil {
				return err
			}
			if !overwrite {
				return fmt.Errorf("aborted: config already exists (pass --force to overwrite)")
			}
			force = true
		}

		path, err := extension.InitConfig(abs, force)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Created %s", path)

		return nil
	},
}

func init() {
	extensionConfigCmd.AddCommand(extensionConfigInitCmd)

	extensionConfigInitCmd.Flags().Bool("force", false, "Overwrite existing .shopware-extension.yml")
}
