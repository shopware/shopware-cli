package upgrade

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// extensionDetail is the per-extension overlay (panels 3b–3e): one layout,
// with the status strip, versions, and user actions varying by status.
type extensionDetail struct {
	result      backend.ExtensionResult
	targetLabel string
}

func newExtensionDetail(result backend.ExtensionResult, targetLabel string) extensionDetail {
	return extensionDetail{result: result, targetLabel: targetLabel}
}

func (d *extensionDetail) Init() tea.Cmd { return nil }

func (d *extensionDetail) ID() string { return "extension-detail" }

func (d *extensionDetail) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}

	switch app.KeyString(key) {
	case "esc", "enter", "q":
		return nil, app.Emit(overlayClosedMsg{})
	}
	return d, nil
}

// headline maps the status to the overlay's title and status strip.
func (d *extensionDetail) headline() (title, badge string, kind tui.Variant, text string) {
	switch d.result.Status {
	case backend.ExtOK:
		return "Compatible extension", "OK", tui.VariantSuccess,
			"A compatible Composer release is available for the selected Shopware version."
	case backend.ExtNeedsUpdate:
		return "Needs update extension", "NEEDS UPDATE", tui.VariantWarning,
			"A compatible Composer release is available; the upgrade updates the extension automatically."
	case backend.ExtMismatch:
		return "Extension needs review", "NEEDS REVIEW", tui.VariantWarning,
			"Store compatibility metadata and Composer constraints do not match. Composer resolution blocks this upgrade."
	case backend.ExtDeprecated:
		return "Replace extension", "REPLACE REQUIRED", tui.VariantError,
			"This extension is deprecated and has no planned support for the selected Shopware version."
	case backend.ExtBlocked:
		return "Blocked extension", "BLOCKED", tui.VariantError,
			"No compatible Composer release exists for the selected Shopware version."
	case backend.ExtReview:
		return "Local extension", "REVIEW", tui.VariantWarning,
			"The wizard does not check custom project extensions. Review it manually before upgrading."
	}
	return "Extension", "", tui.VariantInfo, ""
}

func (d *extensionDetail) View(width, height int) string {
	modal := tui.NewModal(tui.ModalOptions{MaxWidth: 100, AreaWidth: width, AreaHeight: height})
	innerWidth := modal.ContentWidth()

	title, badge, kind, text := d.headline()

	var b strings.Builder
	b.WriteString(tui.SpreadRow(innerWidth,
		tui.BoldStyle.Render(title)+"  "+tui.LabelStyle.Render(d.result.Extension.Name),
		tui.DimStyle.Render(d.targetLabel),
	))
	b.WriteString("\n\n")
	b.WriteString(tui.NewStatusStrip(tui.StatusStripOptions{Variant: kind, Label: badge, Message: text}).Render())
	b.WriteString("\n\n")

	b.WriteString(tui.NewTwoColumn(tui.TwoColumnOptions{Width: innerWidth, LeftWidth: innerWidth * 2 / 5, Left: d.viewLeft(), Right: d.viewRight()}).Render())
	b.WriteString("\n\n")

	b.WriteString(tui.ShortcutBar(tui.Shortcut{Key: "esc", Label: "Back"}))

	return modal.Render(b.String())
}

func (d *extensionDetail) viewLeft() string {
	r := d.result
	var b strings.Builder

	b.WriteString(tui.BoldStyle.Render("Version"))
	b.WriteString("\n")
	b.WriteString(detailRow("Current", r.Extension.Version))

	switch r.Status {
	case backend.ExtNeedsUpdate:
		b.WriteString(detailRow("Required", r.Available+" or higher"))
		b.WriteString(detailRow("Result", warnStyle.Render("update required")))
	case backend.ExtOK:
		b.WriteString(detailRow("Target", r.Available))
		b.WriteString(detailRow("Result", okStyle.Render("compatible upgrade")))
	case backend.ExtMismatch:
		b.WriteString(detailRow("Store metadata", okStyle.Render("compatible")))
		b.WriteString(detailRow("Composer", failStyle.Render("not compatible")))
		b.WriteString(detailRow("Result", warnStyle.Render("mismatch")))
	case backend.ExtDeprecated:
		b.WriteString(detailRow("Status", failStyle.Render("deprecated")))
		if r.Replacement != "" {
			b.WriteString(detailRow("Replacement", r.Replacement))
		}
	case backend.ExtBlocked:
		b.WriteString(detailRow("Compatible release", "none"))
		b.WriteString(detailRow("Result", failStyle.Render("blocked")))
	case backend.ExtReview:
		b.WriteString(detailRow("Result", warnStyle.Render("manual review")))
	}

	if r.Extension.Package != "" {
		b.WriteString(tui.BoldStyle.Render("Package"))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(tui.LabelStyle.Render(r.Extension.Package))
		b.WriteString("\n\n")
	}
	if r.Extension.Path != "" && !r.Extension.ComposerManaged {
		b.WriteString(tui.BoldStyle.Render("Path"))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(tui.DimStyle.Render(r.Extension.Path))
		b.WriteString("\n\n")
	}
	if r.StoreLabel != "" {
		b.WriteString(tui.BoldStyle.Render("Store"))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(tui.DimStyle.Render(r.StoreLabel))
		b.WriteString("\n")
	}

	return b.String()
}

func (d *extensionDetail) viewRight() string {
	r := d.result
	var b strings.Builder
	b.WriteString(userActionStyle.Render("User action"))
	b.WriteString("\n\n")

	bullet := func(s string) {
		b.WriteString(tui.DimStyle.Render("• "))
		b.WriteString(tui.LabelStyle.Render(s))
		b.WriteString("\n\n")
	}

	switch r.Status {
	case backend.ExtOK:
		bullet("No fix is needed")
		bullet("Keep this extension in the upgrade plan")
		bullet("Return to the extension queue")
	case backend.ExtNeedsUpdate:
		bullet("The upgrade updates it to " + r.Available + "+")
		bullet("Nothing to do manually")
		bullet("Return to the extension queue")
	case backend.ExtMismatch:
		bullet("Check the extension package constraints")
		bullet("Update or replace the extension")
		bullet("Recheck compatibility")
	case backend.ExtDeprecated:
		bullet("Replace with the suggested extension")
		bullet("Remove it if no longer needed")
		bullet("Recheck compatibility")
	case backend.ExtBlocked:
		bullet("Ask the vendor for a compatible release")
		bullet("Remove or replace the extension")
		bullet("Recheck compatibility")
	case backend.ExtReview:
		bullet("Review the extension code for breaking changes")
		bullet("Test it against the target version")
	}

	if r.ChangelogURL != "" {
		b.WriteString(tui.StyledLink(r.ChangelogURL, "View changelog →", tui.LinkStyle))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Open release notes in browser"))
		b.WriteString("\n")
	}

	if note := d.note(); note != "" {
		b.WriteString("\n")
		b.WriteString(tui.BoldStyle.Render("Note"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render(note))
		b.WriteString("\n")
	}

	return b.String()
}

func (d *extensionDetail) note() string {
	switch d.result.Status {
	case backend.ExtNeedsUpdate:
		return "Extensions are passed to composer update with an\nopen constraint, so the matching release is\ninstalled together with the platform."
	case backend.ExtMismatch:
		return "The Shopware Store label may be outdated.\nComposer is the source of truth."
	case backend.ExtDeprecated:
		return "Deprecated extensions may stop working in\nfuture Shopware versions."
	case backend.ExtOK, backend.ExtBlocked, backend.ExtReview:
		return ""
	}
	return ""
}

func detailRow(label, value string) string {
	return "  " + tui.DimStyle.Render(tui.PadRight(label, 18)) + value + "\n\n"
}
