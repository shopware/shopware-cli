package extension

import (
	"fmt"
	"path/filepath"
	"strings"

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

Examples:
  # Auto-detect type from manifest.xml / composer.json (interactive prompts when TTY)
  shopware-cli extension config init

  # Non-interactive
  shopware-cli extension config init --type plugin -n
  shopware-cli extension config init ./my-app --type app --name "My App" -n

  # Overwrite existing config
  shopware-cli extension config init --type plugin --force -n
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

		extType, _ := cmd.Flags().GetString("type")
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		maintainer, _ := cmd.Flags().GetString("maintainer")
		force, _ := cmd.Flags().GetBool("force")
		interactiveFlag, _ := cmd.Flags().GetBool("interactive")

		interactive := system.IsInteractionEnabled(cmd.Context())
		if interactiveFlag {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return fmt.Errorf("--interactive requires an interactive terminal (do not pass --no-interaction)")
			}
			interactive = true
		}

		// Non-interactive: --type is required unless we can detect from disk.
		if !interactive && strings.TrimSpace(extType) == "" {
			if detected, detErr := extension.DetectInitType(abs); detErr == nil {
				extType = detected
			} else {
				return fmt.Errorf("--type is required in non-interactive mode (app or plugin): %w", detErr)
			}
		}

		// Interactive: fill missing fields via prompts.
		if interactive {
			if err := askExtensionConfigInit(abs, &extType, &name, &description, &maintainer, &force); err != nil {
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

		logging.FromContext(cmd.Context()).Infof("Created %s", path)

		return nil
	},
}

func askExtensionConfigInit(abs string, extType, name, description, maintainer *string, force *bool) error {
	// Seed type from detection when empty.
	if strings.TrimSpace(*extType) == "" {
		if detected, err := extension.DetectInitType(abs); err == nil {
			*extType = detected
		}
	}

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

	typeValue := strings.ToLower(strings.TrimSpace(*extType))
	if typeValue != extension.InitTypeApp && typeValue != extension.InitTypePlugin {
		typeValue = extension.InitTypePlugin
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Extension type").
				Options(
					huh.NewOption("plugin (composer.json / shopware-platform-plugin)", extension.InitTypePlugin),
					huh.NewOption("app (manifest.xml)", extension.InitTypeApp),
				).
				Value(&typeValue),
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

	if err := form.Run(); err != nil {
		return err
	}

	*extType = typeValue

	return nil
}

func init() {
	extensionConfigCmd.AddCommand(extensionConfigInitCmd)

	extensionConfigInitCmd.Flags().String("type", "", "Extension type: app or plugin (required when non-interactive and type cannot be detected)")
	extensionConfigInitCmd.Flags().String("name", "", "Optional store meta title (en)")
	extensionConfigInitCmd.Flags().String("description", "", "Optional store description (en)")
	extensionConfigInitCmd.Flags().String("maintainer", "", "Optional maintainer note (YAML comment)")
	extensionConfigInitCmd.Flags().Bool("force", false, "Overwrite existing .shopware-extension.yml")
	extensionConfigInitCmd.Flags().Bool("interactive", false, "Force interactive prompts")
}
