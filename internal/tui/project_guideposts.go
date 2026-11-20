package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ProjectWorkflowGuideposts renders commands to run from the newly created
// project directory. ciTemplate is empty when no CI template was created.
// styled=false skips style rendering, including terminal background detection.
func ProjectWorkflowGuideposts(ciTemplate string, docker, styled bool) string {
	render := func(style lipgloss.Style, text string) string {
		if !styled {
			return text
		}
		return style.Render(text)
	}
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(render(DimText, "  Run these commands from your project directory.") + "\n")
	if docker {
		out.WriteString(render(DimText, "  validate, format and fix need host PHP 8.2+, Composer and Node.js 20+ (npm).") + "\n")
	}

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
		fmt.Fprintf(&out, "\n%s\n\n", render(SectionHeadingStyle, section.title))
		for _, line := range section.lines {
			fmt.Fprintf(&out, "  %s\n", render(BoldText, line))
		}
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, render(DimText, "  Replace custom/plugins/MyPlugin with your extension's path."))
	fmt.Fprintln(&out, render(DimText, "  format changes files; fix --dry-run previews changes."))

	fmt.Fprintf(&out, "\n%s\n\n", render(SectionHeadingStyle, "Export project data"))
	fmt.Fprintf(&out, "  %s\n", render(BoldText, "shopware-cli project dump --output dump.sql"))
	fmt.Fprintf(&out, "  %s\n", render(BoldText, "shopware-cli project dump --help"))
	fmt.Fprintln(&out, render(DimText, "  Start your local environment and configure its database first."))
	fmt.Fprintln(&out, render(DimText, "  Choose a new output filename; existing files are overwritten."))
	fmt.Fprintln(&out, render(DimText, "  Exports can contain sensitive data; keep them out of version control."))

	fmt.Fprintf(&out, "\n%s\n\n", render(SectionHeadingStyle, "CI template"))
	if ciTemplate == "" {
		fmt.Fprintln(&out, render(DimText, "  No CI template created."))
	} else {
		fmt.Fprintf(&out, "  %s  %s\n", render(GreenText, "Created:"), render(BoldText, ciTemplate))
	}
	return out.String()
}
