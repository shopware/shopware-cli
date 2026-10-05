package account

import (
	"github.com/spf13/cobra"
)

var accountCompanyProducerExtensionInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Sync local metadata with an Extension Store listing",
}

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionInfoCmd)
}
