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

	"github.com/doutorfinancas/go-mad/core"
	"github.com/doutorfinancas/go-mad/database"
	"github.com/doutorfinancas/go-mad/generator"
	"github.com/go-sql-driver/mysql"
	"github.com/klauspost/compress/zstd"
	"github.com/shopware/shopware-cli/extension"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

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

		db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
		if err != nil {
			return err
		}

		service := generator.NewService()
		var opt []database.Option
		opt = append(opt, database.OptionValue("hex-encode", "1"))
		opt = append(opt, database.OptionValue("set-charset", "utf8mb4"))
		opt = append(opt, database.OptionValue("dump-trigger", ""))
		opt = append(opt, database.OptionValue("skip-definer", ""))
		opt = append(opt, database.OptionValue("trigger-delimiter", "//"))

		if skipLockTables {
			opt = append(opt, database.OptionValue("skip-lock-tables", "1"))
		}

		if quick {
			opt = append(opt, database.OptionValue("quick", "1"))
		}

		logger, _ := zap.NewProduction()
		dumper, err := database.NewMySQLDumper(db, logger, service, opt...)
		if err != nil {
			return err
		}

		pConf := core.Rules{Ignore: []string{}, NoData: []string{}, Where: map[string]string{}, Rewrite: map[string]core.Rewrite{}}

		if clean {
			pConf.NoData = append(pConf.NoData,
				"cart",
				"customer_recovery",
				"dead_message",
				"enqueue",
				"messenger_messages",
				"import_export_log",
				"increment",
				"elasticsearch_index_task",
				"log_entry",
				"message_queue_stats",
				"notification",
				"payment_token",
				"refresh_token",
				"version",
				"version_commit",
				"version_commit_data",
				"webhook_event_log",
			)
		}

		if anonymize {
			pConf.Rewrite = map[string]core.Rewrite{
				"customer": map[string]string{
					"first_name":     "faker.Person.FirstName()",
					"last_name":      "faker.Person.LastName()",
					"company":        "faker.Person.Name()",
					"title":          "faker.Person.Name()",
					"email":          "faker.Internet.Email()",
					"remote_address": "faker.Internet.Ipv4()",
				},
				"customer_address": map[string]string{
					"first_name":   "faker.Person.FirstName()",
					"last_name":    "faker.Person.LastName()",
					"company":      "faker.Person.Name()",
					"title":        "faker.Person.Name()",
					"street":       "faker.Address.StreetAddress()",
					"zipcode":      "faker.Address.PostCode()",
					"city":         "faker.Address.City()",
					"phone_number": "faker.Phone.Number()",
				},
				"log_entry": map[string]string{
					"provider": "",
				},
				"newsletter_recipient": map[string]string{
					"email":      "faker.Internet.Email()",
					"first_name": "faker.Person.FirstName()",
					"last_name":  "faker.Person.LastName()",
					"city":       "faker.Address.City()",
				},
				"order_address": map[string]string{
					"first_name":   "faker.Person.FirstName()",
					"last_name":    "faker.Person.LastName()",
					"company":      "faker.Person.Name()",
					"title":        "faker.Person.Name()",
					"street":       "faker.Address.StreetAddress()",
					"zipcode":      "faker.Address.PostCode()",
					"city":         "faker.Address.City()",
					"phone_number": "faker.Phone.Number()",
				},
				"order_customer": map[string]string{
					"first_name":     "faker.Person.FirstName()",
					"last_name":      "faker.Person.LastName()",
					"company":        "faker.Person.Name()",
					"title":          "faker.Person.Name()",
					"email":          "faker.Internet.Email()",
					"remote_address": "faker.Internet.Ipv4()",
				},
				"product_review": map[string]string{
					"email": "faker.Internet.Email()",
				},
				"user": map[string]string{
					"username":   "faker.Person.Name()",
					"first_name": "faker.Person.FirstName()",
					"last_name":  "faker.Person.LastName()",
					"email":      "faker.Internet.Email()",
				},
			}
		}

		var projectCfg *shop.Config
		if projectCfg, err = shop.ReadConfig(projectConfigPath, true); err != nil {
			return err
		}

		if projectCfg != nil && projectCfg.ConfigDump != nil {
			pConf.NoData = append(pConf.NoData, projectCfg.ConfigDump.NoData...)
			pConf.Ignore = append(pConf.Ignore, projectCfg.ConfigDump.Ignore...)
			for table, rewrites := range projectCfg.ConfigDump.Rewrite {
				_, ok := pConf.Rewrite[table]

				if !ok {
					pConf.Rewrite[table] = rewrites
				} else {
					for k, v := range rewrites {
						pConf.Rewrite[table][k] = v
					}
				}
			}
			pConf.Where = projectCfg.ConfigDump.Where
		}

		dumper.SetSelectMap(pConf.RewriteToMap())
		dumper.SetWhereMap(pConf.Where)
		if dErr := dumper.SetFilterMap(pConf.NoData, pConf.Ignore); dErr != nil {
			return dErr
		}

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

		if err = dumper.Dump(w); err != nil {
			if strings.Contains(err.Error(), "the RELOAD or FLUSH_TABLES privilege") {
				return fmt.Errorf("%s, you maybe want to disable locking with --skip-lock-tables", err.Error())
			}

			return err
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
		if port == "" {
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
	projectDatabaseDumpCmd.Flags().Bool("clean", false, "Ignores cart, enqueue, message_queue_stats")
	projectDatabaseDumpCmd.Flags().Bool("skip-lock-tables", false, "Skips locking the tables")
	projectDatabaseDumpCmd.Flags().Bool("anonymize", false, "Anonymize customer data")
	projectDatabaseDumpCmd.Flags().String("compression", "", "Compress the dump (gzip, zstd)")
	projectDatabaseDumpCmd.Flags().Bool("zstd", false, "Zstd the whole dump")
	projectDatabaseDumpCmd.Flags().Bool("quick", false, "Use quick option for mysqldump")
}
