package account

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

var accountCompanyListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Lists all available company for your Account",
	Aliases: []string{"ls"},
	Long:    ``,
	Run: func(_ *cobra.Command, _ []string) {
		t := liplogtable.New().
			Border(lipgloss.NormalBorder()).
			Headers("ID", "Name", "Customer ID", "Roles")

		for _, membership := range services.AccountClient.GetMemberships() {
			t.Row(
				strconv.FormatInt(int64(membership.Company.Id), 10),
				membership.Company.Name,
				membership.Company.CustomerNumber,
				strings.Join(membership.GetRoles(), ", "),
			)
		}

		fmt.Fprintln(os.Stdout, t.Render())
	},
}

func init() {
	accountCompanyRootCmd.AddCommand(accountCompanyListCmd)
}
