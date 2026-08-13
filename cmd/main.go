package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	shopwarecli "github.com/shopware/shopware-cli/cmd/shopware-cli"
)

type ExitCode int

const (
	exitOK      ExitCode = 0
	exitError   ExitCode = 1
	exitCancel  ExitCode = 2
	exitAuth    ExitCode = 4
	exitPending ExitCode = 8
)

func main() {
	ctx := context.Background()

	// Convert swx commands to shopware-cli commands
	// ...
	
	// read build information
	// ...

	// update-checker ?
	// ...

	// create root command
	rootCmd, err := shopwarecli.NewRootCommand()
	if err != nil {
		slog.Error("Error creating root command", "Error", err)
		os.Exit(int(exitError))
	}

	// tracking timer
	start := time.Now()
	ctx = context.WithValue(ctx, "startTime", start)
	// ctx = context.WithVersion(ctx, buildVersion)

	// execute root command
	err = rootCmd.ExecuteContext(ctx)
	if err != nil {
		slog.Error("Error executing command", "Error", err)
		os.Exit(int(exitError))
	}
}

func normalizeArgs(argv []string) []string {
      args := argv[1:]

      if isSwxAlias(argv[0]) {
          return append([]string{"project", "console"},
          args...)
      }

      return args
}