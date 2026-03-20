package tui

import (
	"fmt"
	"image/color"
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

var hasDarkBG = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

const maxPanelWidth = 80

// TerminalWidth returns the current terminal width, falling back to
// maxPanelWidth if it cannot be determined.
func TerminalWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return maxPanelWidth
	}
	return w
}

// PanelWidth returns the terminal width capped at maxPanelWidth.
func PanelWidth() int {
	w := TerminalWidth()
	if w > maxPanelWidth {
		return maxPanelWidth
	}
	return w
}

var (
	BrandColor   = lipgloss.Color("#076FFF")
	SuccessColor = lipgloss.Color("#04B575")
	MutedColor   = adaptiveMuted()
	ErrorColor   = lipgloss.Color("#FF4D4D")

	// TextColor is the primary foreground for labels, headings, and prominent content.
	TextColor = adaptiveColor("#FFFFFF", "#1A1A1A")

	// BorderColor is used for box borders, dividers, and separator lines.
	BorderColor = adaptiveColor("#303030", "#BCBCBC")

	// SubtleBgColor is used for inactive tab backgrounds, badges, and other subtle background fills.
	SubtleBgColor = adaptiveColor("#444444", "#D0D0D0")

	// LinkColor is used for clickable hyperlinks.
	LinkColor = adaptiveColor("#5F87FF", "#0550AE")

	// WarnColor is used for warning-level indicators.
	WarnColor = adaptiveColor("#FFAA00", "#B35800")

	// SelectedBgColor is used for highlighted/selected rows in lists and tables.
	SelectedBgColor = adaptiveColor("#303030", "#E4E4E4")

	Checkmark = lipgloss.NewStyle().Foreground(SuccessColor).Bold(true).Render("✓")

	DimStyle  = lipgloss.NewStyle().Foreground(MutedColor)
	BoldStyle = lipgloss.NewStyle().Bold(true)

	SectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(BrandColor)
)

func adaptiveColor(dark, light string) color.Color {
	if hasDarkBG {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

func adaptiveMuted() color.Color {
	return adaptiveColor("#999999", "#666666")
}

// Hyperlink returns an OSC 8 hyperlink sequence wrapping label with the given URL.
func Hyperlink(url, label string) string {
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, label)
}

// StyledLink renders a clickable terminal hyperlink with the given lipgloss style.
func StyledLink(url, label string, style lipgloss.Style) string {
	return Hyperlink(url, style.Render(label))
}

// ShopwareTheme returns a huh form theme styled with the Shopware brand colors.
func ShopwareTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeCharm(isDark)

		brand := lipgloss.Color("#076FFF")

		var green, cream, muted color.Color
		if isDark {
			green = lipgloss.Color("#02BF87")
			cream = lipgloss.Color("#FFFDF5")
			muted = lipgloss.Color("#999999")
		} else {
			green = lipgloss.Color("#02BA84")
			cream = lipgloss.Color("#FFFDF5")
			muted = lipgloss.Color("#666666")
		}

		t.Focused.Title = t.Focused.Title.Foreground(brand).Bold(true).MarginBottom(0)
		t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(brand).Bold(true).MarginBottom(1)
		t.Focused.Directory = t.Focused.Directory.Foreground(brand)
		t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(brand)
		t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(brand)
		t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(brand)
		t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(brand)
		t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(green)
		t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(green).SetString("✓ ")
		t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(cream).Background(brand)
		t.Focused.Next = t.Focused.FocusedButton
		t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(brand)
		t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(brand)
		t.Focused.Base = t.Focused.Base.BorderForeground(brand).PaddingLeft(2)
		t.Focused.Card = t.Focused.Base

		t.Focused.Description = t.Focused.Description.Foreground(muted)

		t.Blurred = t.Focused
		t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
		t.Blurred.Card = t.Blurred.Base
		t.Blurred.NextIndicator = lipgloss.NewStyle()
		t.Blurred.PrevIndicator = lipgloss.NewStyle()

		t.Group.Title = t.Focused.Title
		t.Group.Description = t.Focused.Description

		return t
	})
}
