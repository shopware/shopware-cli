package project

import (
	"context"
	"errors"
	"fmt"
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
		"Without arguments an interactive SQL shell is opened; a query can be passed as argument, loaded with --file, or a script piped via stdin.",
	Example: `  shopware-cli project sql "SELECT id, tax_rate FROM tax"
  shopware-cli project sql --format json "SELECT * FROM sales_channel" | jq
  shopware-cli project sql --file script.sql
  shopware-cli project sql < script.sql
  shopware-cli project sql`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveSQLFormat(cmd)
		if err != nil {
			return err
		}

		script, provided, err := resolveSQLInput(cmd, args)
		if err != nil {
			return err
		}

		if err := errIfNoSQLInput(cmd.Context(), provided, isTerminalStream(cmd.InOrStdin())); err != nil {
			return err
		}

		conn, dbConn, cleanup, err := connectProjectDatabase(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		if provided {
			return sqlshell.Run(cmd.Context(), conn, script, cmd.OutOrStdout(), format)
		}

		logging.FromContext(cmd.Context()).Infof("Connected to database %q at %s. Type \"exit\" or press Ctrl+D to quit.", dbConn.Database, dbConn.Addr())

		return sqlshell.InteractiveShell(cmd.Context(), conn, format)
	},
}

// resolveSQLInput returns the SQL to execute and whether a script was
// provided. When provided is false, the caller should open the interactive
// shell (or error if interaction is disabled). --file, a query argument and
// stdin are mutually exclusive sources; --file wins over stdin, and combining
// --file with a query argument is an error.
func resolveSQLInput(cmd *cobra.Command, args []string) (string, bool, error) {
	file, _ := cmd.Flags().GetString("file")
	if file != "" {
		if len(args) > 0 {
			return "", false, errors.New("cannot pass a query together with --file")
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return "", false, fmt.Errorf("failed to read SQL file %q: %w", file, err)
		}

		return string(content), true, nil
	}

	if len(args) > 0 {
		return strings.Join(args, " "), true, nil
	}

	if !isTerminalStream(cmd.InOrStdin()) {
		content, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", false, err
		}

		script := string(content)

		return script, strings.TrimSpace(script) != "", nil
	}

	return "", false, nil
}

func errIfNoSQLInput(ctx context.Context, provided, canOpenShell bool) error {
	if provided || (canOpenShell && system.IsInteractionEnabled(ctx)) {
		return nil
	}

	return errors.New("no query given and interaction is disabled, pass a query as argument, use --file, or pipe a script via stdin")
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
	projectSQLCmd.Flags().String("file", "", "path to a SQL file to execute (instead of a query argument or stdin)")
}
