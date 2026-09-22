package account

import (
	"github.com/spf13/cobra"
)

var accountCompanyProducerExtensionCmd = &cobra.Command{
	Use:   "extension",
	Short: "List, sync, and upload Extension Store extensions",
}

func init() {
	accountCompanyProducerCmd.AddCommand(accountCompanyProducerExtensionCmd)
}
