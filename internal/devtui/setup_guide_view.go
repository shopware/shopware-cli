package devtui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

func (sg setupGuide) viewContent() string {
	switch sg.step {
	case setupStepWelcome:
		return sg.viewWelcome()
	case setupStepAdminUser:
		return sg.viewAdminUser()
	case setupStepDockerPHP:
		return sg.viewDockerPHP()
	case setupStepReview:
		return sg.viewReview()
	case setupStepDone:
		return sg.viewDone()
	}
	return ""
}

func stepBadge(stepNum, totalSteps int) string {
	return tui.TextBadge(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))
}

// totalSteps returns the number of numbered wizard steps:
// admin account, PHP version, review.
func (sg setupGuide) totalSteps() int {
	return 3
}

// stepNum returns the 1-based index of the given wizard step within the
// currently active step sequence. Steps outside the numbered sequence
// (welcome, done) return 0.
func (sg setupGuide) stepNum(step setupStep) int {
	switch step {
	case setupStepAdminUser:
		return 1
	case setupStepDockerPHP:
		return 2
	case setupStepReview:
		return 3
	case setupStepWelcome, setupStepDone:
		return 0
	}
	return 0
}

func (sg setupGuide) viewWelcome() string {
	var b strings.Builder
	b.WriteString(tui.TextBadge("Setup"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Set up Docker development environment"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("This project needs a development environment configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("before you can use "))
	b.WriteString(tui.BoldText.Render("shopware-cli project dev"))
	b.WriteString(tui.DimStyle.Render("."))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("The setup will create a Docker-based local environment"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("with the following services:"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Shopware"))
	b.WriteString(tui.DimStyle.Render(" — your shop at http://127.0.0.1:8000"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Adminer"))
	b.WriteString(tui.DimStyle.Render(" — database GUI"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • "))
	b.WriteString(valueStyle.Render("Mailpit"))
	b.WriteString(tui.DimStyle.Render(" — email testing"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("This will create a "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString(tui.DimStyle.Render(" configuration file."))
	b.WriteString("\n\n")
	b.WriteString(renderConfirmButtons("Start setup", "Quit", sg.confirmYes))
	b.WriteString("\n\n")
	return tui.RenderPhaseCardCowsay("Let me help you to set up Docker!", b.String())
}

func (sg setupGuide) viewAdminUser() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepAdminUser), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Admin Account"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The login for your local Shopware admin panel and API."))
	b.WriteString("\n\n")
	b.WriteString(valueStyle.Render("Choose a username"))
	b.WriteString("\n")
	b.WriteString(sg.username.View())
	b.WriteString("\n\n")
	b.WriteString(valueStyle.Render("Choose a password"))
	b.WriteString(tui.DimStyle.Render("  at least 8 characters"))
	b.WriteString("\n")
	b.WriteString(sg.password.View())
	if sg.passwordErr != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(sg.passwordErr))
	}
	b.WriteString("\n\n")
	b.WriteString(renderShowPasswordCheckbox(sg.password.EchoMode == textinput.EchoNormal, sg.credFocus == credFocusShowPassword))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Will be written to "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString(tui.DimStyle.Render(" — use a throwaway password for local dev."))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewDockerPHP() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepDockerPHP), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Docker Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Select the PHP version for your Docker containers."))
	if sg.phpConstraint != "" {
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Filtered by shopware/core require.php: "))
		b.WriteString(valueStyle.Render(sg.phpConstraint))
	}
	b.WriteString("\n\n")

	opts := make([]tui.SelectOption, len(sg.phpVersions))
	for i, v := range sg.phpVersions {
		opts[i] = tui.SelectOption{Label: "PHP " + v}
	}
	b.WriteString(tui.RenderSelectList("PHP Version", "", opts, sg.phpCursor))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewReview() string {
	c := sg.currentConfig()
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepReview), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Review Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The following configuration will be written."))
	b.WriteString("\n\n")

	divider := tui.SectionDivider(60)
	b.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render("Docker")))
	b.WriteString(tui.KVRow("Shop URL", urlStyle.Render(c.url)))
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

func (sg setupGuide) viewDone() string {
	var b strings.Builder
	if sg.err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor).Render("Configuration failed"))
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(sg.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("You can manually create "))
		b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("or try again with shopware-cli project dev"))
	} else {
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
	}
	b.WriteString("\n\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) footerHint() string {
	// phaseHeaderFooter always appends a "ctrl+c Exit" badge, so don't
	// repeat it here.
	switch sg.step {
	case setupStepWelcome:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case setupStepAdminUser:
		if sg.credFocus == credFocusShowPassword {
			return tui.ShortcutBar(
				tui.Shortcut{Key: "↑/↓/tab", Label: "Navigate"},
				tui.Shortcut{Key: "enter", Label: "Toggle"},
			)
		}
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓/tab", Label: "Navigate"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepDockerPHP:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepReview:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case setupStepDone:
		return tui.ShortcutBar(tui.Shortcut{Key: "enter", Label: "Continue"})
	}
	return ""
}
