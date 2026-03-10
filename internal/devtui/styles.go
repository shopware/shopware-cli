package devtui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
)

const generalLabelWidth = 16

var (
	surfaceColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#F5F7FA"),
		Dark:  lipgloss.Color("#12182A"),
	}
	panelColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#FFFFFF"),
		Dark:  lipgloss.Color("#171E33"),
	}
	panelAccentColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#E4EAF7"),
		Dark:  lipgloss.Color("#222D48"),
	}
	borderColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#CBD5E1"),
		Dark:  lipgloss.Color("#46506A"),
	}
	mutedBorderColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#E2E8F0"),
		Dark:  lipgloss.Color("#313A53"),
	}
	textColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#0F172A"),
		Dark:  lipgloss.Color("#D7DEF5"),
	}
	mutedTextColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#64748B"),
		Dark:  lipgloss.Color("#8B95B5"),
	}
	accentColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#0F766E"),
		Dark:  lipgloss.Color("#36D7B7"),
	}
	accentBgColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#D7F5F0"),
		Dark:  lipgloss.Color("#103D3A"),
	}
	warningColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#A16207"),
		Dark:  lipgloss.Color("#F8C146"),
	}
	warningBgColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#FEF3C7"),
		Dark:  lipgloss.Color("#3A2D12"),
	}
	errorColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#DC2626"),
		Dark:  lipgloss.Color("#F87171"),
	}
	errorBgColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#FEE2E2"),
		Dark:  lipgloss.Color("#3D1F26"),
	}
)

var (
	appStyle = lipgloss.NewStyle().
			Background(surfaceColor).
			Foreground(textColor).
			Padding(0, 1)

	surfaceTextStyle = lipgloss.NewStyle().
				Background(surfaceColor).
				Foreground(textColor)

	surfaceMutedTextStyle = lipgloss.NewStyle().
				Background(surfaceColor).
				Foreground(mutedTextColor)

	panelTextStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(textColor)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(textColor).
			Background(panelAccentColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			BorderBackground(surfaceColor).
			Padding(0, 2).
			MarginRight(1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor).
				Background(surfaceColor).
				Padding(1, 2).
				MarginRight(1)

	sectionStyle = lipgloss.NewStyle().
			Background(panelColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedBorderColor).
			BorderBackground(surfaceColor).
			Padding(1, 2)

	sectionTitleStyle = lipgloss.NewStyle().
				Background(panelColor).
				Foreground(textColor).
				Bold(true)

	labelStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(mutedTextColor).
			Width(generalLabelWidth)

	subLabelStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(mutedTextColor).
			Width(generalLabelWidth).
			PaddingLeft(2)

	valueStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(textColor)

	urlStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(accentColor)

	secretStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(warningColor)

	helpStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(mutedTextColor)

	keyStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Background(panelAccentColor).
			Padding(0, 1).
			Bold(true)

	activeBadgeStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Background(accentBgColor).
				Padding(0, 1).
				Bold(true)

	warningBadgeStyle = lipgloss.NewStyle().
				Foreground(warningColor).
				Background(warningBgColor).
				Padding(0, 1).
				Bold(true)

	errorStyle = lipgloss.NewStyle().
			Background(panelColor).
			Foreground(errorColor)

	errorBadgeStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Background(errorBgColor).
			Padding(0, 1).
			Bold(true)

	sidebarStyle = lipgloss.NewStyle().
			Background(panelColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedBorderColor).
			BorderBackground(surfaceColor).
			Padding(1, 1)

	sidebarItemStyle = lipgloss.NewStyle().
				Background(panelColor).
				Foreground(mutedTextColor).
				Padding(0, 1)

	selectedSidebarItemStyle = lipgloss.NewStyle().
					Foreground(textColor).
					Background(panelAccentColor).
					Bold(true).
					Padding(0, 1)

	activeSidebarItemStyle = lipgloss.NewStyle().
				Background(panelColor).
				Foreground(accentColor).
				Padding(0, 1)

	activeSelectedSidebarItemStyle = lipgloss.NewStyle().
					Foreground(textColor).
					Background(accentBgColor).
					Bold(true).
					Padding(0, 1)

	contentPanelStyle = lipgloss.NewStyle().
				Background(panelColor).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(mutedBorderColor).
				BorderBackground(surfaceColor).
				Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Background(panelColor).
				Foreground(textColor).
				Bold(true).
				Padding(0, 0, 1)

	overlayStyle = lipgloss.NewStyle().
			Background(panelColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			BorderBackground(surfaceColor).
			Padding(1, 3)
)

func renderSection(title, body string) string {
	return renderSectionWidth(title, body, 0)
}

func renderSectionWidth(title, body string, width int) string {
	contentWidth := lipgloss.Width(body)
	if titleWidth := lipgloss.Width(title); titleWidth > contentWidth {
		contentWidth = titleWidth
	}
	if contentWidth == 0 {
		contentWidth = 1
	}
	// If a fixed width is given, subtract sectionStyle's horizontal framing
	// (border + padding) so the inner content fills the target width.
	if width > 0 {
		inner := width - sectionStyle.GetHorizontalFrameSize()
		if inner > contentWidth {
			contentWidth = inner
		}
	}

	header := sectionTitleStyle.Width(contentWidth).Render(title)
	spacer := panelTextStyle.Width(contentWidth).Render("")
	content := panelTextStyle.Width(contentWidth).Render(body)

	return sectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, spacer, content))
}

func renderKeyHint(key, action string) string {
	return keyStyle.Render(strings.ToUpper(key)) + surfaceMutedTextStyle.Render(" "+action)
}

func renderFooter(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, part)
	}

	if len(filtered) == 0 {
		return ""
	}

	separator := surfaceMutedTextStyle.Render("  |  ")
	style := surfaceMutedTextStyle.Padding(1, 0, 0)
	return style.Render(strings.Join(filtered, separator))
}

func renderKVRow(label, value string, valueRenderer lipgloss.Style) string {
	if value == "" {
		value = "not configured"
		valueRenderer = helpStyle
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		labelStyle.Render(label),
		valueRenderer.Render(value),
	)
}

func renderSubKVRow(label, value string, valueRenderer lipgloss.Style) string {
	if value == "" {
		value = "not configured"
		valueRenderer = helpStyle
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		subLabelStyle.Render(label),
		valueRenderer.Render(value),
	)
}

func clampMin(value, minimum int) int {
	if value < minimum {
		return minimum
	}

	return value
}

// padLines pads each line of a rendered string to the given width using the
// provided style so that whitespace has the correct background color.
func padLines(s string, width int, style lipgloss.Style) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w < width {
			lines[i] = line + style.Render(strings.Repeat(" ", width-w))
		}
	}
	return strings.Join(lines, "\n")
}
