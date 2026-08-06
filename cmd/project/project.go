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
	h := update.HandleFromContext(ctx)
	if h == nil {
		return nil
	}
	return tui.NewUpdateCheckCmd(func(waitCtx context.Context) (string, bool) {
		result := h.Wait(waitCtx)
		if result.Release == nil {
			return "", false
		}
		return result.Release.Version, true
	}, h.MarkRendered)
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
