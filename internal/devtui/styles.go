package devtui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

var (
	valueStyle = lipgloss.NewStyle().
			Foreground(tui.TextColor)

	urlStyle = lipgloss.NewStyle().
			Foreground(tui.LinkColor)

	secretStyle = lipgloss.NewStyle().
			Foreground(tui.WarnColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(tui.MutedColor)

	activeBadgeStyle = lipgloss.NewStyle().
				Foreground(tui.SuccessColor).
				Bold(true).
				Padding(0, 1)

	warningBadgeStyle = lipgloss.NewStyle().
				Foreground(tui.WarnColor).
				Bold(true).
				Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(tui.ErrorColor)

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.BorderColor).
			Padding(1, 1)

	sidebarItemStyle = lipgloss.NewStyle().
				Foreground(tui.MutedColor).
				Padding(0, 1)

	selectedSidebarItemStyle = lipgloss.NewStyle().
					Foreground(tui.TextColor).
					Background(tui.SubtleBgColor).
					Bold(true).
					Padding(0, 1)

	activeSidebarItemStyle = lipgloss.NewStyle().
				Foreground(tui.SuccessColor).
				Padding(0, 1)

	activeSelectedSidebarItemStyle = lipgloss.NewStyle().
					Foreground(tui.TextColor).
					Background(tui.SelectedBgColor).
					Bold(true).
					Padding(0, 1)

	contentPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.BorderColor).
				Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Foreground(tui.TextColor).
				Bold(true).
				Padding(0, 0, 1)

	activeBtnStyle = lipgloss.NewStyle().
			Foreground(tui.TextColor).
			Background(tui.BrandColor).
			Padding(0, 2)

	inactiveBtnStyle = lipgloss.NewStyle().
				Foreground(tui.MutedColor).
				Background(tui.SubtleBgColor).
				Padding(0, 2)
)

func renderConfirmButtons(yesLabel, noLabel string, yesActive bool) string {
	var yes, no string
	if yesActive {
		yes = activeBtnStyle.Render(yesLabel)
		no = inactiveBtnStyle.Render(noLabel)
	} else {
		yes = inactiveBtnStyle.Render(yesLabel)
		no = activeBtnStyle.Render(noLabel)
	}
	return yes + "  " + no
}

// buildTabHeader renders the tui-example-style tab header with numbered tabs
// and a right-aligned branding line. The active tab's bottom border is open
// so it flows into the content box below.
func buildTabHeader(activeTab int, width int) string {
	tabWidths := make([]int, len(tabNames))
	for i, name := range tabNames {
		tabWidths[i] = 8 + len(name)
	}

	activeNumStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.TextColor).
		Background(tui.BrandColor)
	activeLabelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.BrandColor)
	inactiveNumStyle := lipgloss.NewStyle().
		Foreground(tui.MutedColor).
		Background(tui.SubtleBgColor)
	inactiveLabelStyle := lipgloss.NewStyle().
		Foreground(tui.MutedColor)

	bc := tui.BorderColor
	bdr := func(s string) string {
		return lipgloss.NewStyle().Foreground(bc).Render(s)
	}

	tabAreaWidth := 1
	for _, w := range tabWidths {
		tabAreaWidth += w + 1
	}

	// Row 1: tab top border
	var r1 strings.Builder
	r1.WriteString(bdr("╭"))
	for i, w := range tabWidths {
		r1.WriteString(bdr(strings.Repeat("─", w)))
		if i < len(tabWidths)-1 {
			r1.WriteString(bdr("┬"))
		}
	}
	r1.WriteString(bdr("╮"))

	// Row 2: tab labels + right-aligned branding
	var r2 strings.Builder
	for i, name := range tabNames {
		r2.WriteString(bdr("│"))
		num := fmt.Sprintf(" %d ", i+1)
		if i == activeTab {
			r2.WriteString("  " + activeNumStyle.Render(num) + " " + activeLabelStyle.Render(name) + "  ")
		} else {
			r2.WriteString("  " + inactiveNumStyle.Render(num) + " " + inactiveLabelStyle.Render(name) + "  ")
		}
	}
	r2.WriteString(bdr("│"))

	branding := tui.BrandingLine()
	fill := width - tabAreaWidth - tui.BrandingLineWidth()
	if fill < 0 {
		fill = 0
	}
	r2.WriteString(strings.Repeat(" ", fill) + branding)

	// Row 3: junction — active tab open bottom meets content box top border
	var r3 strings.Builder
	if activeTab == 0 {
		r3.WriteString(bdr("│"))
	} else {
		r3.WriteString(bdr("├"))
	}

	for i, w := range tabWidths {
		if i == activeTab {
			r3.WriteString(strings.Repeat(" ", w))
		} else {
			r3.WriteString(bdr(strings.Repeat("─", w)))
		}

		if i < len(tabWidths)-1 {
			switch {
			case i == activeTab:
				r3.WriteString(bdr("└"))
			case i+1 == activeTab:
				r3.WriteString(bdr("┘"))
			default:
				r3.WriteString(bdr("┴"))
			}
		}
	}

	if activeTab == len(tabWidths)-1 {
		r3.WriteString(bdr("└"))
	} else {
		r3.WriteString(bdr("┴"))
	}

	remaining := width - tabAreaWidth - 1
	if remaining > 0 {
		r3.WriteString(bdr(strings.Repeat("─", remaining)))
	}
	r3.WriteString(bdr("╮"))

	return r1.String() + "\n" + r2.String() + "\n" + r3.String()
}
