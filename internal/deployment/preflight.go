package deployment

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/shopware/shopware-cli/internal/tui"
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
// It does not connect to remote targets or check their readiness.
type Preflight struct {
	ProjectDirectory string
	ConfigPath       string
	ArchivePath      string
	Targets          []PreflightTarget
}

// PreflightTarget contains ordered, backend-provided details for one target.
type PreflightTarget struct {
	Fields []PreflightField
}

type PreflightField struct {
	Label string
	Value string
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
	color := false
	if file, ok := output.(*os.File); ok {
		color = os.Getenv("NO_COLOR") == "" && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
	}
	styled := func(style lipgloss.Style, value string) string {
		if color {
			return style.Render(value)
		}
		return value
	}
	var summary strings.Builder
	summary.WriteString(styled(tui.SectionTitleStyle, "Deployment preflight") + "\n")
	field := func(label, value string) {
		// Keep paths and configuration values on one line in terminal output.
		if strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) }) {
			value = strconv.Quote(value)
		}
		valueStyle := lipgloss.NewStyle()
		switch label {
		case "Environment", "Deployment", "SSH target", "SSH host":
			valueStyle = tui.SectionTitleStyle
		case "Active path":
			valueStyle = tui.BoldStyle
		}
		fmt.Fprintf(&summary, "  %s %s\n", styled(tui.DimStyle, label+":"), styled(valueStyle, value))
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
		for _, detail := range target.Fields {
			field(detail.Label, detail.Value)
		}
	}
	summary.WriteByte('\n')
	if _, err := io.WriteString(output, summary.String()); err != nil {
		return fmt.Errorf("cannot display deployment preflight: %w", err)
	}
	return nil
}
