package project

import (
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/go-sql-driver/mysql"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

const (
	CompressionGzip = "gzip"
	CompressionZstd = "zstd"
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

		output, _ := cmd.Flags().GetString("output")
		clean, _ := cmd.Flags().GetBool("clean")
		skipLockTables, _ := cmd.Flags().GetBool("skip-lock-tables")
		anonymize, _ := cmd.Flags().GetBool("anonymize")
		compression, _ := cmd.Flags().GetString("compression")
		quick, _ := cmd.Flags().GetBool("quick")
		parallel, _ := cmd.Flags().GetInt("parallel")
		insertIntoLimit, _ := cmd.Flags().GetInt("insert-into-limit")
		limits, _ := cmd.Flags().GetStringArray("limit")

		db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
		if err != nil {
			return err
		}

		dumper := mysqldump.NewMySQLDumper(db)
		dumper.LockTables = !skipLockTables
		dumper.Quick = quick
		dumper.Parallel = parallel
		dumper.InsertIntoLimit = insertIntoLimit

		var projectCfg *shop.Config
		if projectCfg, err = shop.ReadConfig(cmd.Context(), projectConfigPath, true); err != nil {
			return err
		}

		if projectCfg.ConfigDump == nil {
			projectCfg.ConfigDump = &shop.ConfigDump{}
		}

		if clean {
			projectCfg.ConfigDump.EnableClean()
		}

		if anonymize {
			projectCfg.ConfigDump.EnableAnonymization()
		}

		if len(limits) > 0 && projectCfg.ConfigDump.Limit == nil {
			projectCfg.ConfigDump.Limit = make(map[string]mysqldump.TableLimit)
		}

		for _, limit := range limits {
			table, rows, ok := strings.Cut(limit, "=")
			if !ok {
				return fmt.Errorf("invalid --limit %q, expected format table=rows (e.g. order=100)", limit)
			}

			rowCount, err := strconv.Atoi(rows)
			if err != nil || rowCount < 1 {
				return fmt.Errorf("invalid --limit %q, rows must be a positive number", limit)
			}

			cfgLimit := projectCfg.ConfigDump.Limit[table]
			cfgLimit.Rows = rowCount
			projectCfg.ConfigDump.Limit[table] = cfgLimit
		}

		projectCfg.ConfigDump.NormalizeFakerExpressions()

		dumper.SelectMap = projectCfg.ConfigDump.Rewrite
		dumper.WhereMap = projectCfg.ConfigDump.Where
		dumper.NoData = projectCfg.ConfigDump.NoData
		dumper.Ignore = projectCfg.ConfigDump.Ignore
		dumper.LimitMap = projectCfg.ConfigDump.Limit

		var w io.Writer
		if output == "-" {
			w = os.Stdout
		} else {
			if compression == CompressionGzip {
				output += ".gz"
			}

			if compression == CompressionZstd {
				output += ".zst"
			}

			if w, err = os.Create(output); err != nil {
				return err
			}
		}

		if compression == CompressionGzip {
			w = gzip.NewWriter(w)
		}

		if compression == CompressionZstd {
			w, err = zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
			if err != nil {
				return err
			}
		}

		if err = dumper.Dump(cmd.Context(), w); err != nil {
			if strings.Contains(err.Error(), "the RELOAD or FLUSH_TABLES privilege") {
				return fmt.Errorf("%s, you maybe want to disable locking with --skip-lock-tables", err.Error())
			}

			return err
		}

		if compression == CompressionZstd {
			if err = w.(*zstd.Encoder).Close(); err != nil {
				return err
			}
		}

		if compression == CompressionGzip {
			if err = w.(*gzip.Writer).Close(); err != nil {
				return err
			}
		}

		logging.FromContext(cmd.Context()).Infof("Successfully created the dump %s", output)

		return nil
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
	if _, err := findClosestShopwareProject(); err != nil {
		return executor.NewLocal("").DatabaseConnection(cmd.Context())
	}

	return resolveProjectDatabaseConnection(cmd)
}

func init() {
	projectRootCmd.AddCommand(projectDatabaseDumpCmd)
	projectDatabaseDumpCmd.Flags().String("host", "", "hostname")
	projectDatabaseDumpCmd.Flags().String("database", "", "database name")
	projectDatabaseDumpCmd.Flags().StringP("username", "u", "", "mysql user")
	projectDatabaseDumpCmd.Flags().StringP("password", "p", "", "mysql password (omit value to be prompted interactively)")
	projectDatabaseDumpCmd.Flags().Lookup("password").NoOptDefVal = passwordFlagPrompt
	projectDatabaseDumpCmd.Flags().String("port", "", "mysql port")

	projectDatabaseDumpCmd.Flags().String("output", "dump.sql", "file or - (for stdout)")
	projectDatabaseDumpCmd.Flags().Bool("clean", false, "Ignores cart, messenger_messages, message_queue_stats,...")
	projectDatabaseDumpCmd.Flags().Bool("skip-lock-tables", false, "Skips locking the tables")
	projectDatabaseDumpCmd.Flags().Bool("anonymize", false, "Anonymize customer data")
	projectDatabaseDumpCmd.Flags().String("compression", "", "Compress the dump (gzip, zstd)")
	projectDatabaseDumpCmd.Flags().Bool("quick", false, "Use quick option for mysqldump")
	projectDatabaseDumpCmd.Flags().Int("parallel", 0, "Number of tables to dump concurrently (0 = disabled)")
	projectDatabaseDumpCmd.Flags().Int("insert-into-limit", 0, "Limit the number of rows per INSERT statement (0 = auto, takes priority over --quick when set)")
	projectDatabaseDumpCmd.Flags().StringArray("limit", nil, "Limit the rows of a table (e.g. order=100 dumps only the 100 newest orders). Tables referencing the limited table are filtered automatically; ancestors of self-referencing rows (e.g. product variants) are kept so the dump stays importable. Can be specified multiple times")
}
