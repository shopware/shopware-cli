package project

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// passwordFlagPrompt is the sentinel value Cobra sets via NoOptDefVal when
// --password/-p is passed without an argument. Any user with this literal
// string as their password would need to use DATABASE_URL or an env file instead.
const passwordFlagPrompt = "__INTERACTIVE__"

var projectDatabaseDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dumps the Shopware database",
	RunE: func(cmd *cobra.Command, _ []string) error {
		mysqlConfig, err := assembleConnectionURI(cmd)
		if err != nil {
			return err
		}

		compressionFlag, _ := cmd.Flags().GetString("compression")
		compression, err := mysqldump.ParseCompression(compressionFlag)
		if err != nil {
			return err
		}

		projectCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		clean, _ := cmd.Flags().GetBool("clean")
		skipLockTables, _ := cmd.Flags().GetBool("skip-lock-tables")
		anonymize, _ := cmd.Flags().GetBool("anonymize")
		quick, _ := cmd.Flags().GetBool("quick")
		parallel, _ := cmd.Flags().GetInt("parallel")
		insertIntoLimit, _ := cmd.Flags().GetInt("insert-into-limit")
		limits, _ := cmd.Flags().GetStringArray("limit")

		return shop.DumpDatabase(cmd.Context(), mysqlConfig, projectCfg.ConfigDump, shop.DumpDatabaseOptions{
			Output:          output,
			Compression:     compression,
			Clean:           clean,
			Anonymize:       anonymize,
			SkipLockTables:  skipLockTables,
			Quick:           quick,
			Parallel:        parallel,
			InsertIntoLimit: insertIntoLimit,
			LimitOverrides:  limits,
		})
	},
}

func assembleConnectionURI(cmd *cobra.Command) (*mysql.Config, error) {
	dbConn, err := resolveDumpDatabaseConnection(cmd)
	if err != nil {
		return nil, err
	}

	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	db, _ := cmd.Flags().GetString("database")

	if host != "" {
		dbConn.Host = host
	}

	if port != "" {
		dbConn.Port = port
	}

	if db != "" {
		dbConn.Database = db
	}

	if username != "" {
		dbConn.Username = username
		dbConn.Password = ""
	}

	if cmd.Flags().Changed("password") {
		if password == passwordFlagPrompt {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return nil, errors.New("cannot prompt for password: interaction disabled")
			}

			if !term.IsTerminal(os.Stdin.Fd()) {
				return nil, errors.New("cannot prompt for password: stdin is not a terminal")
			}

			fmt.Fprint(cmd.ErrOrStderr(), "Enter MySQL password: ") //nolint:errcheck // prompt output is best-effort, ReadPassword surfaces real terminal errors
			pass, err := term.ReadPassword(os.Stdin.Fd())
			fmt.Fprintln(cmd.ErrOrStderr()) //nolint:errcheck // trailing newline is best-effort

			if err != nil {
				return nil, fmt.Errorf("could not read password: %w", err)
			}

			dbConn.Password = string(pass)
		} else {
			dbConn.Password = password
		}
	}

	return dbConn.MySQLConfig(), nil
}

// resolveDumpDatabaseConnection resolves credentials like the other database
// commands, but keeps dump usable outside a Shopware project: there the
// process environment and the connection flags are all that is needed.
func resolveDumpDatabaseConnection(cmd *cobra.Command) (*executor.DatabaseConnection, error) {
	if _, err := findClosestShopwareProject(false); err != nil {
		return executor.NewLocal("").DatabaseConnection(cmd.Context())
	}

	return resolveProjectDatabaseConnection(cmd)
}

func init() {
	projectRootCmd.AddCommand(projectDatabaseDumpCmd)
	projectDatabaseDumpCmd.Flags().String("host", "", "Hostname")
	projectDatabaseDumpCmd.Flags().String("database", "", "Database name")
	projectDatabaseDumpCmd.Flags().StringP("username", "u", "", "Mysql user")
	projectDatabaseDumpCmd.Flags().StringP("password", "p", "", "Mysql password (omit value to be prompted interactively)")
	projectDatabaseDumpCmd.Flags().Lookup("password").NoOptDefVal = passwordFlagPrompt
	projectDatabaseDumpCmd.Flags().String("port", "", "Mysql port")

	projectDatabaseDumpCmd.Flags().String("output", "dump.sql", "File or - (for stdout)")
	projectDatabaseDumpCmd.Flags().Bool("clean", false, "Ignores cart, messenger_messages, message_queue_stats,...")
	projectDatabaseDumpCmd.Flags().Bool("skip-lock-tables", false, "Skips locking the tables")
	projectDatabaseDumpCmd.Flags().Bool("anonymize", false, "Anonymize customer data")
	projectDatabaseDumpCmd.Flags().String("compression", "", "Compress the dump (gzip, zstd)")
	projectDatabaseDumpCmd.Flags().Bool("quick", false, "Use quick option for mysqldump")
	projectDatabaseDumpCmd.Flags().Int("parallel", 0, "Number of tables to dump concurrently (0 = disabled)")
	projectDatabaseDumpCmd.Flags().Int("insert-into-limit", 0, "Limit the number of rows per INSERT statement (0 = auto, takes priority over --quick when set)")
	projectDatabaseDumpCmd.Flags().StringArray("limit", nil, "Limit the rows of a table (e.g. order=100 dumps only the 100 newest orders). Tables referencing the limited table are filtered automatically; ancestors of self-referencing rows (e.g. product variants) are kept so the dump stays importable. Requires the CREATE and DROP privileges to freeze the kept rows into staging tables. Can be specified multiple times")
}
