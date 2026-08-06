package executor

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/logging"
)

// defaultMySQLConfig returns the connection defaults used when no
// DATABASE_URL is available.
func defaultMySQLConfig() *mysql.Config {
	return &mysql.Config{
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
}

// applyDatabaseURL applies a Symfony DATABASE_URL to the connection config.
func applyDatabaseURL(cfg *mysql.Config, databaseURL string) error {
	parsedURI, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("could not parse DATABASE_URL: %w", err)
	}

	if parsedURI.User != nil {
		cfg.User = parsedURI.User.Username()

		if password, ok := parsedURI.User.Password(); ok {
			cfg.Passwd = password
		} else {
			// Reset password if it is not set
			cfg.Passwd = ""
		}
	}

	if parsedURI.Host != "" {
		cfg.Addr = parsedURI.Host

		if parsedURI.Port() == "" {
			cfg.Addr = net.JoinHostPort(parsedURI.Host, "3306")
		}
	}

	if parsedURI.Path != "" {
		cfg.DBName = strings.Trim(parsedURI.Path, "/")
	}

	return nil
}

// localDatabaseConnection builds the connection config from the project's
// local Symfony env files.
func localDatabaseConnection(ctx context.Context, projectRoot string) (*mysql.Config, error) {
	cfg := defaultMySQLConfig()

	if projectRoot == "" {
		return cfg, nil
	}

	if err := envfile.LoadSymfonyEnvFile(projectRoot); err != nil {
		return nil, err
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return cfg, nil
	}

	logging.FromContext(ctx).Info("Using DATABASE_URL env as default connection string. options can override specific parts (--username=foo)")

	if err := applyDatabaseURL(cfg, databaseURL); err != nil {
		return nil, err
	}

	return cfg, nil
}
