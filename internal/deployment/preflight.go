package deployment

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type PreflightAction string

const (
	PreflightRollout  PreflightAction = "rollout"
	PreflightRollback PreflightAction = "rollback"
)

type PreflightOptions struct {
	Action      PreflightAction
	Environment string
	Deployment  Deployment
}

// Preflight describes the resolved local source and configured remote targets.
// It does not connect to SSH or check remote readiness.
type Preflight struct {
	ProjectDirectory string
	ConfigPath       string
	ArchivePath      string
	Targets          []PreflightTarget
}

type PreflightTarget struct {
	Name           string
	Host           string
	User           string
	Port           int
	Directory      string
	DeploymentRoot string
	Migration      bool
}

type PreflightProvider interface {
	DeploymentPreflight(context.Context, Deployment, PreflightAction) (Preflight, error)
}

// WritePreflight prints a backend's preview before confirmation or activation.
// Backends without this optional capability retain their existing behavior.
func WritePreflight(ctx context.Context, backend Backend, options PreflightOptions, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	provider, ok := backend.(PreflightProvider)
	if !ok {
		return nil
	}
	preview, err := provider.DeploymentPreflight(ctx, options.Deployment, options.Action)
	if err != nil {
		return fmt.Errorf("cannot prepare deployment preflight: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot resolve working directory: %w", err)
	}
	environment := options.Environment
	if environment == "" {
		environment = "local"
	}
	var summary strings.Builder
	summary.WriteString("Deployment preflight\n")
	field := func(label, value string) {
		// Keep paths and configuration values on one line in terminal output.
		if strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) }) {
			value = strconv.Quote(value)
		}
		fmt.Fprintf(&summary, "  %s: %s\n", label, value)
	}
	field("Action", string(options.Action))
	field("Working directory", cwd)
	field("Project directory", preview.ProjectDirectory)
	if preview.ConfigPath != "" {
		field("Project config", preview.ConfigPath)
	}
	field("Environment", environment)
	field("Deployment", options.Deployment.Reference)
	if preview.ArchivePath != "" {
		field("Archive", preview.ArchivePath)
	}
	for _, target := range preview.Targets {
		summary.WriteByte('\n')
		if target.Name != "" {
			field("SSH target", target.Name)
		}
		field("SSH host", target.Host)
		user := target.User
		if user == "" {
			user = "SSH config/default"
		}
		field("SSH user", user)
		port := "SSH config/default"
		// The SSH transport only passes -p for non-default ports.
		if target.Port != 0 && target.Port != 22 {
			port = strconv.Itoa(target.Port)
		}
		field("SSH port", port)
		field("Active path", target.Directory)
		field("Deployment root", target.DeploymentRoot)
		if target.Migration {
			field("Role", "Deployment Helper host")
		}
	}
	summary.WriteByte('\n')
	if _, err := io.WriteString(output, summary.String()); err != nil {
		return fmt.Errorf("cannot display deployment preflight: %w", err)
	}
	return nil
}
