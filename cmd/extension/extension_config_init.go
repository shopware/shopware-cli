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
	Short: "Create a .shopware-extension.yml for an existing extension",
	Long: `Initialize CLI configuration for an existing Shopware extension checkout.

Creates .shopware-extension.yml used by validation, packaging, and build workflows.
The extension type (app vs plugin) is always detected from the checkout
(manifest.xml / composer.json) — it cannot be overridden.

Examples:
  # Interactive prompts for optional store metadata when TTY
  shopware-cli extension config init

  # Non-interactive
  shopware-cli extension config init -n
  shopware-cli extension config init ./my-app --name "My App" -n

  # Overwrite existing config
  shopware-cli extension config init --force -n
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

		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		maintainer, _ := cmd.Flags().GetString("maintainer")
		force, _ := cmd.Flags().GetBool("force")
		interactiveFlag, _ := cmd.Flags().GetBool("interactive")

		// Type always comes from the checkout — flag was rejected in review.
		extType, err := extension.DetectInitType(abs)
		if err != nil {
			return err
		}

		interactive := system.IsInteractionEnabled(cmd.Context())
		if interactiveFlag {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return fmt.Errorf("--interactive requires an interactive terminal (do not pass --no-interaction)")
			}
			interactive = true
		}

		if interactive {
			if err := askExtensionConfigInit(abs, &name, &description, &maintainer, &force); err != nil {
				return err
			}
		}

		path, err := extension.InitConfig(abs, extension.InitConfigOptions{
			Type:        extType,
			Name:        name,
			Description: description,
			Maintainer:  maintainer,
			Force:       force,
		})
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Created %s (type %s)", path, extType)

		return nil
	},
}

func askExtensionConfigInit(abs string, name, description, maintainer *string, force *bool) error {
	existing := extension.ConfigPath(abs)
	if existing != "" && !*force {
		overwrite := false
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("%s already exists. Overwrite?", filepath.Base(existing))).
					Value(&overwrite),
			),
		)
		if err := form.Run(); err != nil {
			return err
		}
		if !overwrite {
			return fmt.Errorf("aborted: %s already exists (pass --force to overwrite)", existing)
		}
		*force = true
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name (store meta title, optional)").
				Value(name),
			huh.NewInput().
				Title("Description (store, optional)").
				Value(description),
			huh.NewInput().
				Title("Maintainer (comment only, optional)").
				Value(maintainer),
		),
	)

	return form.Run()
}

func init() {
	extensionConfigCmd.AddCommand(extensionConfigInitCmd)

	extensionConfigInitCmd.Flags().String("name", "", "Optional store meta title (en)")
	extensionConfigInitCmd.Flags().String("description", "", "Optional store description (en)")
	extensionConfigInitCmd.Flags().String("maintainer", "", "Optional maintainer note (YAML comment)")
	extensionConfigInitCmd.Flags().Bool("force", false, "Overwrite existing .shopware-extension.yml")
	extensionConfigInitCmd.Flags().Bool("interactive", false, "Force interactive prompts")
}
