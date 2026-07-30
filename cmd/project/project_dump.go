package project

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

const (
	CompressionGzip = "gzip"
	CompressionZstd = "zstd"
)

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

		db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
		if err != nil {
			return err
		}
		defer func() {
			_ = db.Close()
		}()

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

		projectCfg.ConfigDump.NormalizeFakerExpressions()

		dumper.SelectMap = projectCfg.ConfigDump.Rewrite
		dumper.WhereMap = projectCfg.ConfigDump.Where
		dumper.NoData = projectCfg.ConfigDump.NoData
		dumper.Ignore = projectCfg.ConfigDump.Ignore

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
	// the executor decides where the database lives and how to reach it
	// (local, docker, or through the ssh connection of a remote environment)
	projectRoot, _ := findClosestShopwareProject()

	cmdExecutor, err := resolveExecutor(cmd, projectRoot)
	if err != nil {
		return nil, err
	}

	cfg, err := cmdExecutor.DatabaseConnection(cmd.Context())
	if err != nil {
		return nil, err
	}

	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	db, _ := cmd.Flags().GetString("database")

	if host != "" {
		if port != "" {
			cfg.Addr = fmt.Sprintf("%s:%s", host, port)
		} else {
			cfg.Addr = host
		}
	}

	if db != "" {
		cfg.DBName = db
	}

	if username != "" {
		cfg.User = username
		cfg.Passwd = ""
	}

	if password != "" {
		cfg.Passwd = password
	}

	return cfg, nil
}

func init() {
	projectRootCmd.AddCommand(projectDatabaseDumpCmd)
	projectDatabaseDumpCmd.Flags().String("host", "", "hostname")
	projectDatabaseDumpCmd.Flags().String("database", "", "database name")
	projectDatabaseDumpCmd.Flags().StringP("username", "u", "", "mysql user")
	projectDatabaseDumpCmd.Flags().StringP("password", "p", "", "mysql password")
	projectDatabaseDumpCmd.Flags().String("port", "", "mysql port")

	projectDatabaseDumpCmd.Flags().String("output", "dump.sql", "file or - (for stdout)")
	projectDatabaseDumpCmd.Flags().Bool("clean", false, "Ignores cart, messenger_messages, message_queue_stats,...")
	projectDatabaseDumpCmd.Flags().Bool("skip-lock-tables", false, "Skips locking the tables")
	projectDatabaseDumpCmd.Flags().Bool("anonymize", false, "Anonymize customer data")
	projectDatabaseDumpCmd.Flags().String("compression", "", "Compress the dump (gzip, zstd)")
	projectDatabaseDumpCmd.Flags().Bool("quick", false, "Use quick option for mysqldump")
	projectDatabaseDumpCmd.Flags().Int("parallel", 0, "Number of tables to dump concurrently (0 = disabled)")
	projectDatabaseDumpCmd.Flags().Int("insert-into-limit", 0, "Limit the number of rows per INSERT statement (0 = auto, takes priority over --quick when set)")
}
