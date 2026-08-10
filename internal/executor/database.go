package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/shopware/shopware-cli/internal/envfile"
)

// DatabaseConnection describes how to reach the project database from the
// host machine.
type DatabaseConnection struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

// Addr returns the host:port address of the database.
func (c *DatabaseConnection) Addr() string {
	return net.JoinHostPort(c.Host, c.Port)
}

// MySQLConfig translates the credentials into a driver configuration.
func (c *DatabaseConnection) MySQLConfig() *mysql.Config {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = c.Addr()
	cfg.User = c.Username
	cfg.Passwd = c.Password
	cfg.DBName = c.Database
	cfg.Loc = time.UTC
	// 0 makes the driver fetch max_allowed_packet from the server on connect
	// (see go-sql-driver's connector.go), so statements from large dumps are
	// neither rejected client-side nor sent beyond what the server accepts.
	cfg.MaxAllowedPacket = 0

	return cfg
}

// Open opens a single dedicated connection to the database, so session state
// (SET, USE, ...) survives across statements. The returned cleanup closes the
// connection and its pool.
func (c *DatabaseConnection) Open(ctx context.Context) (*sql.Conn, func(), error) {
	db, err := sql.Open("mysql", c.MySQLConfig().FormatDSN())
	if err != nil {
		return nil, nil, err
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := db.Conn(connectCtx)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("could not connect to database %q at %s: %w", c.Database, c.Addr(), err)
	}

	cleanup := func() {
		_ = conn.Close()
		_ = db.Close()
	}

	return conn, cleanup, nil
}

func defaultDatabaseConnection() *DatabaseConnection {
	return &DatabaseConnection{
		Host:     "127.0.0.1",
		Port:     "3306",
		Username: "root",
		Password: "root",
		Database: "shopware",
	}
}

// applyDatabaseURL merges a Symfony DATABASE_URL into conn. Parts missing in
// the URL keep their current value, except the password which is cleared when
// a user without password is given.
func applyDatabaseURL(conn *DatabaseConnection, databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("could not parse DATABASE_URL: %w", err)
	}

	// A bare word like "shopware" parses as a path-only URL. Silently keeping
	// the defaults would send commands to the wrong database, so reject it.
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return errors.New("invalid DATABASE_URL: expected a URL like mysql://user:password@host:3306/dbname")
	}

	if parsed.User != nil {
		conn.Username = parsed.User.Username()

		if password, ok := parsed.User.Password(); ok {
			conn.Password = password
		} else {
			conn.Password = ""
		}
	}

	if host := parsed.Hostname(); host != "" {
		conn.Host = host
	}

	if port := parsed.Port(); port != "" {
		conn.Port = port
	}

	if dbName := strings.Trim(parsed.Path, "/"); dbName != "" {
		conn.Database = dbName
	}

	return nil
}

// databaseConnectionFromEnv resolves the connection for executors that run on
// the host. Precedence: executor env overrides > real environment variables >
// Symfony env files, matching how the spawned processes see DATABASE_URL.
func databaseConnectionFromEnv(projectRoot string, extraEnv map[string]string) (*DatabaseConnection, error) {
	conn := defaultDatabaseConnection()

	databaseURL := extraEnv["DATABASE_URL"]

	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	if databaseURL == "" {
		fileValue, err := envfile.ReadValue(projectRoot, "DATABASE_URL")
		if err != nil {
			return nil, err
		}
		databaseURL = fileValue
	}

	if databaseURL == "" {
		return conn, nil
	}

	if err := applyDatabaseURL(conn, databaseURL); err != nil {
		return nil, err
	}

	return conn, nil
}
