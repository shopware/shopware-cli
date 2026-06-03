package project

import (
	"github.com/spf13/cobra"
)

var projectStorageRootCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage and migrate Shopware file storages",
	Long: `Tools to move Shopware's file datastores (media, documents, ...) from the
local disk to an S3 compatible storage and to generate the matching Shopware
filesystem configuration.`,
}

func init() {
	projectRootCmd.AddCommand(projectStorageRootCmd)
}
