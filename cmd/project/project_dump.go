package project

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
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

		dumper := mysqldump.NewMySQLDumper(db)
		dumper.LockTables = !skipLockTables
		dumper.Quick = quick
		dumper.Parallel = parallel
		dumper.InsertIntoLimit = insertIntoLimit

		var projectCfg *shop.Config
		if projectCfg, err = shop.ReadConfig(projectConfigPath, true); err != nil {
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
	cfg := &mysql.Config{
		Loc:                  time.UTC,
		Net:                  "tcp",
		ParseTime:            false,
		AllowNativePasswords: true,
		CheckConnLiveness:    true,
		User:                 "root",
		Passwd:               "root",
		Addr:                 "127.0.0.1:3306",
		DBName:               "shopware",
	}

	if projectRoot, err := findClosestShopwareProject(); err == nil {
		if err := loadDatabaseURLIntoConnection(cmd.Context(), projectRoot, cfg); err != nil {
			return nil, err
		}
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

func loadDatabaseURLIntoConnection(ctx context.Context, projectRoot string, cfg *mysql.Config) error {
	if err := extension.LoadSymfonyEnvFile(projectRoot); err != nil {
		return err
	}

	databaseUrl := os.Getenv("DATABASE_URL")

	if databaseUrl == "" {
		return nil
	}

	logging.FromContext(ctx).Info("Using DATABASE_URL env as default connection string. options can override specific parts (--username=foo)")

	parsedUri, err := url.Parse(databaseUrl)
	if err != nil {
		return fmt.Errorf("could not parse DATABASE_URL: %w", err)
	}

	if parsedUri.User != nil {
		cfg.User = parsedUri.User.Username()

		if password, ok := parsedUri.User.Password(); ok {
			cfg.Passwd = password
		} else {
			// Reset password if it is not set
			cfg.Passwd = ""
		}
	}

	if parsedUri.Host != "" {
		cfg.Addr = parsedUri.Host

		if parsedUri.Port() == "" {
			cfg.Addr = net.JoinHostPort(parsedUri.Host, "3306")
		}
	}

	if parsedUri.Path != "" {
		cfg.DBName = strings.Trim(parsedUri.Path, "/")
	}

	return nil
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
	projectDatabaseDumpCmd.Flags().Bool("zstd", false, "Zstd the whole dump")
	projectDatabaseDumpCmd.Flags().Bool("quick", false, "Use quick option for mysqldump")
	projectDatabaseDumpCmd.Flags().Int("parallel", 0, "Number of tables to dump concurrently (0 = disabled)")
	projectDatabaseDumpCmd.Flags().Int("insert-into-limit", 100, "Limit the number of rows per INSERT statement (ignored when --quick is set; quick forces one row per INSERT)")
}
