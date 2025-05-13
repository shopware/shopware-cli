package account

import (
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var accountCompanyListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Lists all available company for your Account",
	Aliases: []string{"ls"},
	Long:    ``,
	Run: func(_ *cobra.Command, _ []string) {
		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"ID", "Name", "Customer ID", "Roles"})

		for _, membership := range services.AccountClient.GetMemberships() {
			table.Append([]string{
				strconv.FormatInt(int64(membership.Company.Id), 10),
				membership.Company.Name,
				membership.Company.CustomerNumber,
				strings.Join(membership.GetRoles(), ", "),
			})
		}

		table.Render()
	},
}

func init() {
	accountCompanyRootCmd.AddCommand(accountCompanyListCmd)
}
