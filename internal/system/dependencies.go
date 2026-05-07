package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type MissingDependency struct {
	Name   string
	Reason string
}

// CheckProjectDependencies returns the dependencies required to set up a
// Shopware project that are not currently available. When useDocker is true
// only Docker is required; otherwise PHP 8.2+ and Composer must be present.
func CheckProjectDependencies(ctx context.Context, useDocker bool) []MissingDependency {
	var missing []MissingDependency

	if useDocker {
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
	b.WriteString(tui.BoldText.Render("To create a Shopware project, install one of:"))
	b.WriteString("\n\n")

	arrow := tui.GreenText.Render("→")
	b.WriteString("  " + arrow + " " + tui.RecommendedText.Render("Docker") + " " + tui.DimText.Render("(recommended)") + "\n")
	b.WriteString("    " + tui.BlueText.Render("https://docs.docker.com/get-docker/") + "\n")
	if !useDocker {
		b.WriteString("    Then re-run with " + tui.BoldText.Render("--docker") + "\n")
	}
	b.WriteString("\n")
	b.WriteString("  " + arrow + " " + tui.BoldText.Render("PHP 8.2+ and Composer") + "\n")
	b.WriteString("    PHP:      " + tui.BlueText.Render("https://www.php.net/manual/en/install.php") + "\n")
	b.WriteString("    Composer: " + tui.BlueText.Render("https://getcomposer.org/") + "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BlueColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
}
