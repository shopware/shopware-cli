package account

import (
	"fmt"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login into your Shopware Account",
	Long:  "",
	RunE: func(cmd *cobra.Command, _ []string) error {
		tui.PrintBanner()

		_, err := accountApi.NewApi(cmd.Context())
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println(tui.GreenText.Render("  Login successful!"))
		fmt.Println(tui.DimText.Render("  To logout, run: shopware-cli account logout"))
		fmt.Println()

		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(loginCmd)
}
