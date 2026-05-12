package system

import (
	"context"
	"fmt"
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
// string against a constraint (e.g. packagist.PHPConstraint). The interface
// keeps the system package free of cyclic packagist imports.
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

// RenderMissingDependencies returns a styled, bordered block describing the
// missing dependencies and the two supported setup paths (Docker preferred,
// PHP+Composer alternative).
func RenderMissingDependencies(useDocker bool, missing []MissingDependency) string {
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

	if insideContainer {
		b.WriteString(tui.BoldText.Render("To create a Shopware project from inside this container, install:"))
		b.WriteString("\n\n")
		b.WriteString("  " + arrow + " " + tui.BoldText.Render("PHP 8.2+ and Composer") + "\n")
		b.WriteString("    PHP:      " + tui.BlueText.Render("https://www.php.net/downloads.php") + "\n")
		b.WriteString("    Composer: " + tui.BlueText.Render("https://getcomposer.org/") + "\n")
	} else {
		b.WriteString(tui.BoldText.Render("To create a Shopware project, install one of:"))
		b.WriteString("\n\n")
		b.WriteString("  " + arrow + " " + tui.RecommendedText.Render("Docker") + " " + tui.DimText.Render("(recommended)") + "\n")
		b.WriteString("    " + tui.BlueText.Render("https://docs.docker.com/get-docker/") + "\n")
		if !useDocker {
			b.WriteString("    Then re-run with " + tui.BoldText.Render("--docker") + "\n")
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
