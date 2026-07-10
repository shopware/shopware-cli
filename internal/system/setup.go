package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type MissingDependency struct {
	Name   string
	Reason string
}

type Incompatibility struct {
	Title       string
	Description string
}

// CheckIncompatibilities returns soft-warning issues with the chosen setup
// (e.g. macOS Docker without libkrun, or a project folder on a Windows-mounted
// path under WSL) that don't block project creation but degrade performance.
func CheckIncompatibilities(useDocker bool, projectFolder string) []Incompatibility {
	var incompatibilities []Incompatibility

	if useDocker && runtime.GOOS == "darwin" && !IsDockerUsingLibkrun() {
		incompatibilities = append(incompatibilities, Incompatibility{
			Title:       "Using Docker on macOS without libkrun (Docker VMM) may cause severe performance issues with file watching",
			Description: "Consider enabling libkrun in Docker Desktop settings for improved host mount performance",
		})
	}

	if IsWSL() && IsWSLWindowsMount(projectFolder) {
		incompatibilities = append(incompatibilities, Incompatibility{
			Title:       "Creating a project in a Windows-mounted directory (/mnt/c, etc.) under WSL is known to cause severe performance issues",
			Description: "Consider creating the project in the native Linux filesystem instead (e.g., ~/projects/)",
		})
	}

	return incompatibilities
}

// PHPVersionChecker is satisfied by anything that can verify a PHP version
// string against a constraint (e.g. shop.PHPConstraint). The interface
// keeps the system package free of cyclic imports.
type PHPVersionChecker interface {
	Check(phpVersion string) bool
	String() string
}

// CheckProjectDependencies returns the dependencies required to set up a
// Shopware project that are not currently available. When useDocker is true
// and we are not already inside a container, only Docker is required;
// otherwise PHP 8.2+ and Composer must be present locally (matching the
// fallback in runComposerInstall). If phpConstraint is non-nil and the local
// PHP does not satisfy it, that mismatch is reported as well.
func CheckProjectDependencies(ctx context.Context, useDocker bool, phpConstraint PHPVersionChecker) []MissingDependency {
	var missing []MissingDependency

	if useDocker && !IsInsideContainer() {
		if _, err := exec.LookPath("docker"); err != nil {
			missing = append(missing, MissingDependency{Name: "Docker", Reason: "not installed"})
		} else {
			cmd := exec.CommandContext(ctx, "docker", "info")
			if err := cmd.Run(); err != nil {
				missing = append(missing, MissingDependency{Name: "Docker", Reason: "not running"})
			}
		}
		return missing
	}

	phpOk, err := IsPHPVersionAtLeast(ctx, "8.2")
	switch {
	case err != nil:
		missing = append(missing, MissingDependency{Name: "PHP 8.2+", Reason: "not installed"})
	case !phpOk:
		installed, _ := GetInstalledPHPVersion(ctx)
		missing = append(missing, MissingDependency{Name: "PHP 8.2+", Reason: fmt.Sprintf("found PHP %s", strings.TrimSpace(installed))})
	default:
		if phpConstraint != nil {
			installed, _ := GetInstalledPHPVersion(ctx)
			if installed != "" && !phpConstraint.Check(installed) {
				missing = append(missing, MissingDependency{
					Name:   fmt.Sprintf("PHP %s", phpConstraint),
					Reason: fmt.Sprintf("found PHP %s", strings.TrimSpace(installed)),
				})
			}
		}
	}

	if _, err := exec.LookPath("composer"); err != nil {
		missing = append(missing, MissingDependency{Name: "Composer", Reason: "not installed"})
	}

	return missing
}

// ValidateProjectDependencies runs CheckProjectDependencies and, when
// something is missing, prints the rendered explanation to stderr and returns
// an error. action and dockerHint are passed through to
// RenderMissingDependencies to phrase the help text for the calling command.
func ValidateProjectDependencies(ctx context.Context, useDocker bool, phpConstraint PHPVersionChecker, action, dockerHint string) error {
	missing := CheckProjectDependencies(ctx, useDocker, phpConstraint)
	if len(missing) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stderr, RenderMissingDependencies(useDocker, missing, action, dockerHint))
	return fmt.Errorf("missing required dependencies")
}

// RenderMissingDependencies returns a styled, bordered block describing the
// missing dependencies and the two supported setup paths (Docker preferred,
// PHP+Composer alternative). action names what the user was trying to do
// (e.g. "create a Shopware project"); dockerHint is an optional, already
// styled line explaining how to switch to Docker (shown when Docker is
// suggested but was not the chosen setup).
func RenderMissingDependencies(useDocker bool, missing []MissingDependency, action, dockerHint string) string {
	var b strings.Builder

	b.WriteString(tui.RedText.Bold(true).Render("Missing Dependencies"))
	b.WriteString("\n\n")
	b.WriteString("The following requirement")
	if len(missing) == 1 {
		b.WriteString(" is")
	} else {
		b.WriteString("s are")
	}
	b.WriteString(" not met:\n\n")

	cross := tui.RedText.Render("✗")
	for _, m := range missing {
		fmt.Fprintf(&b, "  %s %s %s\n", cross, tui.BoldText.Render(m.Name), tui.DimText.Render("("+m.Reason+")"))
	}

	b.WriteString("\n")
	arrow := tui.GreenText.Render("→")
	insideContainer := IsInsideContainer()

	dockerOnlyMissing := useDocker && len(missing) == 1 && missing[0].Name == "Docker"
	switch {
	case dockerOnlyMissing && missing[0].Reason == "not running":
		b.WriteString(tui.BoldText.Render("Start Docker and try again."))
	case dockerOnlyMissing && missing[0].Reason == "not installed":
		b.WriteString(tui.BoldText.Render("Install Docker and try again."))
	case insideContainer:
		b.WriteString(tui.BoldText.Render(fmt.Sprintf("To %s from inside this container, install:", action)))
		b.WriteString("\n\n")
		b.WriteString("  " + arrow + " " + tui.BoldText.Render("PHP 8.2+ and Composer") + "\n")
		b.WriteString("    PHP:      " + tui.BlueText.Render("https://www.php.net/downloads.php") + "\n")
		b.WriteString("    Composer: " + tui.BlueText.Render("https://getcomposer.org/") + "\n")
	default:
		b.WriteString(tui.BoldText.Render(fmt.Sprintf("To %s, install one of:", action)))
		b.WriteString("\n\n")
		b.WriteString("  " + arrow + " " + tui.RecommendedText.Render("Docker") + " " + tui.DimText.Render("(recommended)") + "\n")
		b.WriteString("    " + tui.BlueText.Render("https://docs.docker.com/get-docker/") + "\n")
		if !useDocker && dockerHint != "" {
			b.WriteString("    " + dockerHint + "\n")
		}
		b.WriteString("\n")
		b.WriteString("  " + arrow + " " + tui.BoldText.Render("PHP 8.2+ and Composer") + "\n")
		b.WriteString("    PHP:      " + tui.BlueText.Render("https://www.php.net/downloads.php") + "\n")
		b.WriteString("    Composer: " + tui.BlueText.Render("https://getcomposer.org/") + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BlueColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
}
