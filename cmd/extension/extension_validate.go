package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/logging"
)

var extensionValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a Extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		var ext extension.Extension

		if stat.IsDir() {
			ext, err = extension.GetExtensionByFolder(path)
		} else {
			ext, err = extension.GetExtensionByZip(path)
		}

		if err != nil {
			return fmt.Errorf("cannot open extension: %w", err)
		}

		context := extension.RunValidation(cmd.Context(), ext)

		if stat.IsDir() {
			context.ApplyIgnores([]extension.ConfigValidationIgnoreItem{
				{
					Identifier: "zip.disallowed_file",
					Message:    ".gitignore is not allowed in the zip file",
				},
			})
		}

		if context.HasErrors() || context.HasWarnings() {
			table := tablewriter.NewWriter(os.Stdout)
			table.Header([]string{"Type", "Identifier", "Message"})

			for _, msg := range context.Errors() {
				_ = table.Append([]string{"Error", msg.Identifier, msg.Message})
			}

			for _, msg := range context.Warnings() {
				_ = table.Append([]string{"Warning", msg.Identifier, msg.Message})
			}

			_ = table.Render()
		}

		if context.HasErrors() {
			return fmt.Errorf("validation failed")
		}

		logging.FromContext(cmd.Context()).Infof("Validation has been successful")

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionValidateCmd)
}
