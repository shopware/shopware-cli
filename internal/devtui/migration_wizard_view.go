package devtui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

func (sg migrationWizard) viewContent() string {
	switch sg.step {
	case migrationStepWelcome:
		return sg.viewWelcome()
	case migrationStepAdminUser:
		return sg.viewAdminUser()
	case migrationStepDockerPHP:
		return sg.viewDockerPHP()
	case migrationStepReview:
		return sg.viewReview()
	case migrationStepDone:
		return sg.viewDone()
	}
	return ""
}

func (sg migrationWizard) viewWelcome() string {
	var b strings.Builder
	b.WriteString(tui.TextBadge("Setup"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Let's set up your Docker development environment"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Run "))
	b.WriteString(tui.BoldText.Render("shopware-cli project dev"))
	b.WriteString(tui.DimStyle.Render(" to create a local Docker development environment. You'll get:"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Shopware"))
	b.WriteString(tui.DimStyle.Render(" — your local shop at "))
	b.WriteString(tui.RenderStyledLink("http://127.0.0.1:8000"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Adminer"))
	b.WriteString(tui.DimStyle.Render(" — a simple way to browse the database"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Mailpit"))
	b.WriteString(tui.DimStyle.Render(" — a place to test outgoing emails"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("In the next few steps you'll choose the admin login, Docker/PHP settings, and whether to enable the PHP profiler."))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("When you finish, we'll create the environment and save its settings in "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString(tui.DimStyle.Render("."))
	b.WriteString("\n\n")
	b.WriteString(renderConfirmButtons("Start setup", "Quit", sg.confirmYes))
	b.WriteString("\n\n")
	return tui.RenderPhaseCardCowsay("Let me help you to set up Docker!", b.String())
}

func (sg migrationWizard) viewAdminUser() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Admin Account"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The login for your local Shopware admin panel and API."))
	b.WriteString("\n\n")
	sg.Render(&b)
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Will be written to "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString(tui.DimStyle.Render(" — use a throwaway password for local dev."))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg migrationWizard) viewDockerPHP() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Docker Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Select the PHP version for your Docker containers."))
	b.WriteString("\n\n")

	opts := make([]tui.SelectOption, len(sg.phpVersions))
	for i, v := range sg.phpVersions {
		label := "PHP " + v
		if i == len(sg.phpVersions)-1 {
			label += " " + tui.DimStyle.Render("(Recommended)")
		}
		opts[i] = tui.SelectOption{Label: label}
	}
	b.WriteString(tui.RenderSelectList("PHP Version", "", opts, sg.phpCursor))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg migrationWizard) viewReview() string {
	c := sg.currentConfig()
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Review Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The following configuration will be written."))
	b.WriteString("\n\n")

	divider := tui.SectionDivider(60)
	b.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render("Docker")))
	b.WriteString(tui.KVRow("Shop URL", tui.RenderStyledLink(c.url)))
	b.WriteString(tui.KVRow("Username", valueStyle.Render(c.username)))
	b.WriteString(tui.KVRow("Password", secretStyle.Render(strings.Repeat("•", len(c.password)))))
	b.WriteString(divider)
	b.WriteString(tui.KVRow("PHP Version", valueStyle.Render("PHP "+c.phpVersion)))

	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("This will create:"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(tui.BoldText.Render("compose.yaml"))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(renderConfirmButtons("Save & start", "Quit", sg.confirmYes))
	b.WriteString("\n\n")

	return tui.RenderPhaseCard(b.String())
}

func (sg migrationWizard) viewDone() string {
	if sg.err != nil {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor).Render("Configuration failed"))
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(sg.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("You can manually create "))
		b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("or try again with shopware-cli project dev"))
		b.WriteString("\n\n")
		return tui.RenderPhaseCard(b.String())
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.SuccessColor).Render("✓ Configuration saved"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Your project is now configured for Docker development."))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The environment will start on the next screen."))

	if sg.deploymentHelperAdded {
		b.WriteString("\n\n")
		b.WriteString(tui.BoldText.Render("Note: "))
		b.WriteString(tui.DimStyle.Render("Added "))
		b.WriteString(valueStyle.Render("shopware/deployment-helper"))
		b.WriteString(tui.DimStyle.Render(" to "))
		b.WriteString(tui.BoldText.Render("composer.json"))
		b.WriteString(tui.DimStyle.Render("."))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Run "))
		b.WriteString(tui.BoldText.Render("composer update shopware/deployment-helper"))
		b.WriteString(tui.DimStyle.Render(" before installing Shopware."))
	}

	b.WriteString("\n\n")
	b.WriteString(tui.BoldText.Render("Press Enter to start the Docker containers and open the project dev workspace."))
	b.WriteString("\n\n")
	return tui.RenderPhaseCardCowsay("Setup's complete!", b.String())
}

func (sg migrationWizard) footerHint() string {
	// phaseHeaderFooter always appends a "ctrl+c Exit" badge, so don't
	// repeat it here.
	switch sg.step {
	case migrationStepWelcome:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case migrationStepAdminUser:
		return sg.FooterHint("Continue")
	case migrationStepDockerPHP:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case migrationStepReview:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case migrationStepDone:
		return tui.ShortcutBar(tui.Shortcut{Key: "enter", Label: "Continue"})
	}
	return ""
}
