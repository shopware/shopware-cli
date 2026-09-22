package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/tui"
)

var projectExtensionCmd = &cobra.Command{
	Use:   "extension",
	Short: "Manage installed Shopware extensions",
}

func projectExtensionOutputFormat(formatName string, jsonAlias bool) (tui.TableFormat, error) {
	format, err := tui.ParseTableFormat(formatName)
	if err != nil {
		return "", err
	}
	if jsonAlias {
		return tui.TableFormatJSON, nil
	}
	return format, nil
}

func init() {
	projectRootCmd.AddCommand(projectExtensionCmd)
}
