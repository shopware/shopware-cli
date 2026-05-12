package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/shopware/shopware-cli/internal/envfile"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: env-bridge <command> [args...]")
		os.Exit(1)
	}

	envDir := os.Getenv("PROJECT_ROOT")
	if envDir == "" {
		var err error
		envDir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting working directory: %v\n", err)
			os.Exit(1)
		}
	}

	if err := envfile.LoadSymfonyEnvFile(envDir); err != nil {
		fmt.Fprintf(os.Stderr, "error loading env files: %v\n", err)
		os.Exit(1)
	}

	binary, err := exec.LookPath(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	if err := syscall.Exec(binary, os.Args[1:], os.Environ()); err != nil {
		os.Exit(1)
	}
}
