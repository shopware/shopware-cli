package account

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/table"
)

var accountCompanyListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Lists all available company for your Account",
	Aliases: []string{"ls"},
	Long:    ``,
	Run: func(_ *cobra.Command, _ []string) {
		table := table.NewWriter(os.Stdout)
		table.Header([]string{"ID", "Name", "Customer ID", "Roles"})

		for _, membership := range services.AccountClient.GetMemberships() {
			_ = table.Append([]string{
				strconv.FormatInt(int64(membership.Company.Id), 10),
				membership.Company.Name,
				membership.Company.CustomerNumber,
				strings.Join(membership.GetRoles(), ", "),
			})
		}

		_ = table.Render()
	},
}

func init() {
	accountCompanyRootCmd.AddCommand(accountCompanyListCmd)
}
