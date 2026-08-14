package project

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/update"
)

func updateBackgroundCmd(ctx context.Context) tea.Cmd {
	handle := update.HandleFromContext(ctx)
	if handle == nil {
		return nil
	}

	return tui.NewUpdateCheckCmd(func(waitCtx context.Context) bool {
		result := handle.Wait(waitCtx)
		return result.Release != nil
	})
}

var (
	projectConfigPath string
	environmentName   string
)

var projectRootCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage your Shopware Project",
}

func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(projectRootCmd)
	projectRootCmd.PersistentFlags().StringVar(&projectConfigPath, "project-config", shop.DefaultConfigFileName(), "Path to config")
	projectRootCmd.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "Target environment name")
}
