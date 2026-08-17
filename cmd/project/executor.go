package project

import (
	"database/sql"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// resolveExecutor returns the Executor for the current environment.
func resolveExecutor(cmd *cobra.Command, projectRoot string) (executor.Executor, error) {
	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
	if err != nil {
		return nil, err
	}

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}

	return executor.New(projectRoot, envCfg, cfg)
}

// resolveProjectDatabaseConnection resolves the database credentials of the
// current environment through its executor.
func resolveProjectDatabaseConnection(cmd *cobra.Command) (*executor.DatabaseConnection, error) {
	projectRoot, err := findClosestShopwareProject()
	if err != nil {
		return nil, err
	}

	cmdExecutor, err := resolveExecutor(cmd, projectRoot)
	if err != nil {
		return nil, err
	}

	return cmdExecutor.DatabaseConnection(cmd.Context())
}

// connectProjectDatabase resolves the database of the current environment and
// opens a single dedicated connection to it. The returned cleanup closes
// connection and pool.
func connectProjectDatabase(cmd *cobra.Command) (*sql.Conn, *executor.DatabaseConnection, func(), error) {
	dbConn, err := resolveProjectDatabaseConnection(cmd)
	if err != nil {
		return nil, nil, nil, err
	}

	conn, cleanup, err := dbConn.Open(cmd.Context())
	if err != nil {
		return nil, nil, nil, err
	}

	return conn, dbConn, cleanup, nil
}
