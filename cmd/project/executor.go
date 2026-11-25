package project

import (
	"database/sql"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// resolveExecutor returns the Executor for the current environment.
func resolveExecutor(cmd *cobra.Command, projectRoot string) (executor.Executor, error) {
	cmdExecutor, _, err := resolveEnvironmentExecutor(cmd, projectRoot)
	return cmdExecutor, err
}

// resolveEnvironmentExecutor returns the Executor and the configuration of
// the current environment.
func resolveEnvironmentExecutor(cmd *cobra.Command, projectRoot string) (executor.Executor, *shop.EnvironmentConfig, error) {
	actualProjectConfigPath := shop.SearchConfigPath(cmd.Context(), projectRoot, projectConfigPath)
	cfg, err := shop.ReadConfig(cmd.Context(), actualProjectConfigPath, projectConfigPath == "")
	if err != nil {
		return nil, nil, err
	}

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, nil, err
	}

	cfg, err = cfg.WithEnvironment(environmentName)
	if err != nil {
		return nil, nil, err
	}

	cmdExecutor, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return nil, nil, err
	}

	return cmdExecutor, envCfg, nil
}

// resolveProjectDatabaseConnection resolves the database credentials of the
// current environment through its executor.
func resolveProjectDatabaseConnection(cmd *cobra.Command, endpoints ...executor.DatabaseEndpoint) (*executor.DatabaseConnection, error) {
	projectRoot, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return nil, err
	}

	cmdExecutor, err := resolveExecutor(cmd, projectRoot)
	if err != nil {
		return nil, err
	}

	return cmdExecutor.DatabaseConnection(cmd.Context(), endpoints...)
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
