package project

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/sqlshell"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var projectSQLCmd = &cobra.Command{
	Use:   "sql [query]",
	Short: "Run SQL queries against the project database",
	Long: "Connects to the project database using the connection details of the current environment (local, docker, ...), " +
		"so you don't need to know the host or credentials. " +
		"Without arguments an interactive SQL shell is opened; a query can be passed as argument or a script piped via stdin.",
	Example: `  shopware-cli project sql "SELECT id, tax_rate FROM tax"
  shopware-cli project sql --format json "SELECT * FROM sales_channel" | jq
  shopware-cli project sql < script.sql
  shopware-cli project sql`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveSQLFormat(cmd)
		if err != nil {
			return err
		}

		conn, dbConn, cleanup, err := connectProjectDatabase(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		if len(args) > 0 {
			return sqlshell.Run(cmd.Context(), conn, strings.Join(args, " "), cmd.OutOrStdout(), format)
		}

		if !isTerminalStream(cmd.InOrStdin()) {
			script, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}

			return sqlshell.Run(cmd.Context(), conn, string(script), cmd.OutOrStdout(), format)
		}

		if !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("no query given and interaction is disabled, pass a query as argument or pipe a script via stdin")
		}

		logging.FromContext(cmd.Context()).Infof("Connected to database %q at %s. Type \"exit\" or press Ctrl+D to quit.", dbConn.Database, dbConn.Addr())

		return sqlshell.InteractiveShell(cmd.Context(), conn, format)
	},
}

func resolveSQLFormat(cmd *cobra.Command) (sqlshell.Format, error) {
	formatFlag, _ := cmd.Flags().GetString("format")

	if formatFlag == "" {
		if isTerminalStream(cmd.OutOrStdout()) {
			return sqlshell.FormatTable, nil
		}

		return sqlshell.FormatTSV, nil
	}

	return sqlshell.ParseFormat(formatFlag)
}

// isTerminalStream reports whether a Cobra in/out stream is an interactive
// terminal. Streams replaced via cmd.SetIn/cmd.SetOut are never terminals.
func isTerminalStream(stream any) bool {
	file, ok := stream.(*os.File)

	return ok && term.IsTerminal(file.Fd())
}

func init() {
	projectRootCmd.AddCommand(projectSQLCmd)
	projectSQLCmd.Flags().String("format", "", "output format: table, tsv, json (default: table when stdout is a terminal, tsv otherwise)")
}
