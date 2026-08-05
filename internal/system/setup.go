package system

import (
	"context"
	"errors"
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
// otherwise PHP 8.2+ must be present locally (Composer is not required — a
// PHAR copy is downloaded on demand when it is missing, see ResolveComposer).
// If phpConstraint is non-nil and the checked PHP does not satisfy it, that
// mismatch is reported as well. phpBinary selects the PHP executable to
// check; when empty, the ambient PHP (PHP_BINARY or PATH) is checked instead.
func CheckProjectDependencies(ctx context.Context, useDocker bool, phpConstraint PHPVersionChecker, phpBinary string) []MissingDependency {
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

	installed, err := installedPHPVersion(ctx, phpBinary)
	phpOk := err == nil && phpVersionAtLeast(installed, "8.2")
	switch {
	case err != nil:
		missing = append(missing, MissingDependency{Name: "PHP 8.2+", Reason: "not installed"})
	case !phpOk:
		missing = append(missing, MissingDependency{Name: "PHP 8.2+", Reason: "found PHP " + strings.TrimSpace(installed)})
	default:
		if phpConstraint != nil && !phpConstraint.Check(installed) {
			missing = append(missing, MissingDependency{
				Name:   fmt.Sprintf("PHP %s", phpConstraint),
				Reason: "found PHP " + strings.TrimSpace(installed),
			})
		}
	}

	return missing
}

// installedPHPVersion returns the version of the given PHP binary, or of the
// ambient PHP (PHP_BINARY or PATH) when phpBinary is empty.
func installedPHPVersion(ctx context.Context, phpBinary string) (string, error) {
	if phpBinary != "" {
		return GetPHPVersionOfBinary(ctx, phpBinary)
	}
	return GetInstalledPHPVersion(ctx)
}

// ValidateProjectDependencies runs CheckProjectDependencies and, when
// something is missing, prints the rendered explanation to stderr and returns
// an error. action and dockerHint are passed through to
// RenderMissingDependencies to phrase the help text for the calling command.
// phpBinary optionally selects the PHP executable to check instead of the
// ambient one.
func ValidateProjectDependencies(ctx context.Context, useDocker bool, phpConstraint PHPVersionChecker, action, dockerHint, phpBinary string) error {
	missing := CheckProjectDependencies(ctx, useDocker, phpConstraint, phpBinary)
	if len(missing) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stderr, RenderMissingDependencies(useDocker, missing, action, dockerHint))
	return errors.New("missing required dependencies")
}

// phpDependencyConstraint returns the constraint text from a PHP-related
// missing dependency (e.g. "8.2+" or "~8.2.0 || ~8.3.0") and whether one was
// found.
func phpDependencyConstraint(missing []MissingDependency) (string, bool) {
	for _, m := range missing {
		if after, ok := strings.CutPrefix(m.Name, "PHP "); ok {
			return after, true
		}
	}
	return "", false
}

// phpBinaryExample returns an illustrative PHP_BINARY value for the given
// constraint (e.g. "PHP_BINARY=/usr/bin/php8.3").
func phpBinaryExample(constraint string) string {
	for _, v := range []string{"8.5", "8.4", "8.3", "8.2"} {
		if strings.Contains(constraint, v) {
			return "PHP_BINARY=/usr/bin/php" + v
		}
	}
	return "PHP_BINARY=/usr/bin/php8.2"
}

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
		b.WriteString("  ")
		b.WriteString(arrow)
		b.WriteString(" ")
		b.WriteString(tui.BoldText.Render("PHP 8.2+"))
		b.WriteString("\n")
		b.WriteString("    PHP: ")
		b.WriteString(tui.BlueText.Render("https://www.php.net/downloads.php"))
		b.WriteString("\n")
	default:
		phpConstraint, hasPHP := phpDependencyConstraint(missing)

		b.WriteString(tui.BoldText.Render(fmt.Sprintf("To %s, either:", action)))
		b.WriteString("\n\n")
		b.WriteString("  ")
		b.WriteString(arrow)
		b.WriteString(" ")
		b.WriteString(tui.RecommendedText.Render("Docker"))
		b.WriteString(" ")
		b.WriteString(tui.DimText.Render("(recommended)"))
		b.WriteString(": ")
		if !useDocker && dockerHint != "" {
			b.WriteString(dockerHint)
		} else {
			b.WriteString(tui.DimText.Render("re-run with " + tui.BoldText.Render("--docker")))
		}
		b.WriteString("\n")
		b.WriteString("    ")
		b.WriteString(tui.BlueText.Render("https://docs.docker.com/get-docker/"))
		b.WriteString("\n")
		b.WriteString("\n")

		if hasPHP {
			phpText := fmt.Sprintf("Install a PHP version matching %s, or point PHP_BINARY at one", phpConstraint)
			b.WriteString("  ")
			b.WriteString(arrow)
			b.WriteString(" ")
			b.WriteString(tui.BoldText.Render(phpText))
			b.WriteString("\n")
			b.WriteString("    ")
			b.WriteString(tui.DimText.Render("(e.g. " + phpBinaryExample(phpConstraint) + ")"))
			b.WriteString("\n")
			b.WriteString("    PHP: ")
			b.WriteString(tui.BlueText.Render("https://www.php.net/downloads.php"))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(arrow)
			b.WriteString(" ")
			b.WriteString(tui.BoldText.Render("PHP 8.2+"))
			b.WriteString("\n")
			b.WriteString("    PHP: ")
			b.WriteString(tui.BlueText.Render("https://www.php.net/downloads.php"))
			b.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BlueColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
}
