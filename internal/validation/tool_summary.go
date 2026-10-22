package validation

import (
	"fmt"
	"io"
	"strings"
)

func toolDetail(tool ToolResult) string {
	if tool.SkipReason != "" {
		return tool.SkipReason
	}
	if tool.Err != nil {
		return tool.Err.Error()
	}
	errors, warnings := 0, 0
	for _, finding := range tool.Findings {
		if finding.Severity == SeverityError {
			errors++
		} else {
			warnings++
		}
	}
	var details []string
	if errors > 0 {
		details = append(details, countNoun(errors, "error"))
	}
	if warnings > 0 {
		details = append(details, countNoun(warnings, "warning"))
	}
	if tool.Note != "" {
		details = append(details, tool.Note)
	}
	if len(details) == 0 {
		return "no findings"
	}
	return strings.Join(details, " · ")
}

func writeToolSummary(w io.Writer, tools []ToolResult, title string, verbose, markdown bool) error {
	var output strings.Builder
	if markdown {
		fmt.Fprintf(&output, "## %s\n\n```text\n", title)
	} else {
		fmt.Fprintf(&output, "%s:\n", title)
	}
	width := 0
	for _, tool := range tools {
		width = max(width, len(tool.Name))
	}
	for _, tool := range tools {
		detail := strings.Join(strings.Fields(toolDetail(tool)), " ")
		fmt.Fprintf(&output, "  %-*s  %-9s  %s\n", width, tool.Name, tool.Status(), detail)
	}
	if markdown {
		output.WriteString("```\n")
	}
	completed, failed, total, errors, warnings := false, false, 0, 0, 0
	for _, tool := range tools {
		if tool.SkipReason != "" {
			continue
		}
		completed = true
		failed = failed || tool.Err != nil
		showOutput := tool.Err != nil || verbose
		if len(tool.Findings) == 0 && tool.Err == nil && (!showOutput || tool.Stdout+tool.Stderr == "") {
			continue
		}
		if markdown {
			fmt.Fprintf(&output, "\n### %s\n\n", tool.Name)
		} else {
			fmt.Fprintf(&output, "\n%s:\n", tool.Name)
		}
		if tool.Err != nil {
			fmt.Fprintf(&output, "  Error: %s\n", tool.Err)
		}
		findings := sortedFindings(tool.Findings)
		lastPath := ""
		for i, finding := range findings {
			total++
			if finding.Severity == SeverityError {
				errors++
			} else {
				warnings++
			}
			if markdown {
				location := finding.Path
				if location == "" {
					location = "general"
				}
				if finding.Line > 0 {
					location += fmt.Sprintf(":%d", finding.Line)
				}
				fmt.Fprintf(&output, "- **%s** %s: %s (`%s`)\n", finding.Severity, location, finding.Message, finding.Identifier)
			} else {
				if i == 0 || finding.Path != lastPath {
					fmt.Fprintf(&output, "  %s\n", finding.Path)
					lastPath = finding.Path
				}
				fmt.Fprintf(&output, "    %d  %-7s  [%s]  %s\n", finding.Line, finding.Severity, finding.Identifier, finding.Message)
			}
			if finding.Tip != "" {
				fmt.Fprintf(&output, "    Tip: %s\n", finding.Tip)
			}
		}
		if showOutput {
			appendToolOutput(&output, tool, markdown)
		}
	}
	if !completed {
		output.WriteString("\nNo tools completed; 0 problems reported\n")
	} else if !failed || total > 0 {
		fmt.Fprintf(&output, "\n%s\n", summaryLine(total, errors, warnings))
	}
	_, err := io.WriteString(w, output.String())
	return err
}

func appendToolOutput(output *strings.Builder, tool ToolResult, markdown bool) {
	for _, stream := range []struct{ name, content string }{{"stdout", tool.Stdout}, {"stderr", tool.Stderr}} {
		if stream.content == "" {
			continue
		}
		if markdown {
			// A longer fence keeps captured backticks inside the output block.
			fence := "```"
			for strings.Contains(stream.content, fence) {
				fence += "`"
			}
			fmt.Fprintf(output, "\n%s:\n\n%stext\n%s\n%s\n", stream.name, fence, stream.content, fence)
		} else {
			fmt.Fprintf(output, "\n  %s:\n    %s\n", stream.name, strings.ReplaceAll(strings.TrimRight(stream.content, "\n"), "\n", "\n    "))
		}
	}
}

func writeToolDiagnostics(w io.Writer, tools []ToolResult, verbose bool) error {
	var output strings.Builder
	for _, tool := range tools {
		if tool.Err == nil && !verbose {
			continue
		}
		if tool.Err != nil {
			fmt.Fprintf(&output, "%s: %s\n", tool.Name, tool.Err)
		}
		if tool.Stdout+tool.Stderr != "" {
			if tool.Err == nil {
				fmt.Fprintf(&output, "%s:\n", tool.Name)
			}
			appendToolOutput(&output, tool, false)
		}
	}
	_, err := io.WriteString(w, output.String())
	return err
}
