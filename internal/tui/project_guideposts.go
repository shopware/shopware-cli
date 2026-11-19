package tui

import (
	"fmt"
	"strings"
)

// ProjectWorkflowGuideposts renders commands to run from the newly created
// project directory. ciTemplate is empty when no CI template was created.
func ProjectWorkflowGuideposts(ciTemplate string) string {
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(DimText.Render("  Run these commands from your project directory.") + "\n")

	sections := []struct {
		title string
		lines []string
	}{
		{"Check and improve your project", []string{
			"shopware-cli project validate .",
			"shopware-cli project format .",
			"shopware-cli project fix . --dry-run",
		}},
		{"Check and improve an extension", []string{
			"shopware-cli extension validate custom/plugins/MyPlugin",
			"shopware-cli extension format custom/plugins/MyPlugin",
			"shopware-cli extension fix custom/plugins/MyPlugin --dry-run",
		}},
	}
	for _, section := range sections {
		fmt.Fprintf(&out, "\n%s\n\n", SectionHeadingStyle.Render(section.title))
		for _, line := range section.lines {
			fmt.Fprintf(&out, "  %s\n", BoldText.Render(line))
		}
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, DimText.Render("  Replace custom/plugins/MyPlugin with your extension's path."))
	fmt.Fprintln(&out, DimText.Render("  format changes files; fix --dry-run previews changes."))

	fmt.Fprintf(&out, "\n%s\n\n", SectionHeadingStyle.Render("Export project data"))
	fmt.Fprintf(&out, "  %s\n", BoldText.Render("shopware-cli project dump --output dump.sql"))
	fmt.Fprintf(&out, "  %s\n", BoldText.Render("shopware-cli project dump --help"))
	fmt.Fprintln(&out, DimText.Render("  Start your local environment and configure its database first."))
	fmt.Fprintln(&out, DimText.Render("  Choose a new output filename; existing files are overwritten."))
	fmt.Fprintln(&out, DimText.Render("  Exports can contain sensitive data; keep them out of version control."))

	fmt.Fprintf(&out, "\n%s\n\n", SectionHeadingStyle.Render("CI template"))
	if ciTemplate == "" {
		fmt.Fprintln(&out, DimText.Render("  No CI template created."))
	} else {
		fmt.Fprintf(&out, "  %s  %s\n", GreenText.Render("Created:"), BoldText.Render(ciTemplate))
	}
	return out.String()
}
