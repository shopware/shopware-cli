package account

import (
	"github.com/spf13/cobra"
)

var accountCompanyProducerCmd = &cobra.Command{
	Use:   "producer",
	Short: "Configure Extension Store listings and upload versions",
}

func init() {
	accountRootCmd.AddCommand(accountCompanyProducerCmd)
}
