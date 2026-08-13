package main

import (
	"log/slog"
	"os"
	"shopware-cli/cmd/shopwarecli"
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

	// read build information
	// ...

	// update-checker
	// ...

	// create root command
	rootCmd, err := shopwarecli.NewRootCommand()
	if err != nil {
		slog.Error("Error creating root command", "Error", err)
		os.Exit(int(exitError))
	}

	// execute root command
	err = rootCmd.Execute()
	if err != nil {
		slog.Error("Error executing command", "Error", err)
		os.Exit(int(exitError))
	}
}
