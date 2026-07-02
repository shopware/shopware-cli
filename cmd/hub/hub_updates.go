package hub

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

// eventLabels maps raw event.event values to human-readable group headings.
var eventLabels = map[string]string{
	"hub:release:created":  "New releases",
	"hub:release:updated":  "Updated releases",
	"hub:plugin:approved":  "Approved extensions",
	"hub:plugin:rejected":  "Rejected extensions",
	"hub:plugin:published": "Published extensions",
	"hub:review:received":  "New reviews",
}

// labelForEvent returns the human-readable label for an event type,
// falling back to the raw event string when no mapping exists.
func labelForEvent(event string) string {
	if label, ok := eventLabels[event]; ok {
		return label
	}
	return event
}

// hyperlink renders text as a clickable OSC 8 terminal hyperlink when the
// terminal supports it (i.e. stdout is a TTY). In non-TTY contexts the URL
// is appended in parentheses so the information is never lost.
func hyperlink(text, url string) string {
	if url == "" {
		return text
	}
	if isatty.IsTerminal(os.Stdout.Fd()) {
		// OSC 8 hyperlink: ESC ] 8 ; ; <url> ESC \ <text> ESC ] 8 ; ; ESC \
		return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
	}
	return fmt.Sprintf("%s (%s)", text, url)
}

// groupByEvent groups updates by their event.event value and returns the keys
// sorted by their human-readable label.
func groupByEvent(updates []account_api.HubUpdate) ([]string, map[string][]account_api.HubUpdate) {
	grouped := make(map[string][]account_api.HubUpdate)
	for _, u := range updates {
		key := u.Event.Event
		grouped[key] = append(grouped[key], u)
	}

	keys := slices.Collect(maps.Keys(grouped))
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Compare(labelForEvent(a), labelForEvent(b))
	})

	return keys, grouped
}

var hubUpdatesCmd = &cobra.Command{
	Use:   "updates",
	Short: "Show latest updates from the Shopware Hub",
	RunE: func(cmd *cobra.Command, _ []string) error {
		updates, err := services.AccountClient.FetchHubUpdates(cmd.Context())
		if err != nil {
			return fmt.Errorf("could not fetch hub updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println(tui.DimText.Render("  No updates available."))
			return nil
		}

		keys, grouped := groupByEvent(updates)

		headingStyle := lipgloss.NewStyle().Bold(true).Underline(true)
		itemStyle := lipgloss.NewStyle().PaddingLeft(2)
		linkStyle := tui.BlueText.Underline(true)
		dimStyle := tui.DimText

		for i, key := range keys {
			if i > 0 {
				fmt.Println()
			}

			label := labelForEvent(key)
			fmt.Println(headingStyle.Render(label))

			for _, u := range grouped[key] {
				title := u.Title
				if title == "" {
					title = dimStyle.Render("(no title)")
				}

				var line string
				if u.Link != "" {
					linkedTitle := hyperlink(linkStyle.Render(title), u.Link)
					line = itemStyle.Render(fmt.Sprintf("• %s", linkedTitle))
				} else {
					line = itemStyle.Render(fmt.Sprintf("• %s", title))
				}

				fmt.Println(line)

				if u.Event.CreatedAt != "" {
					date := formatDate(u.Event.CreatedAt)
					fmt.Println(itemStyle.Render(fmt.Sprintf("  %s", dimStyle.Render(date))))
				}
			}
		}

		return nil
	},
}

// formatDate trims the time portion from ISO 8601 timestamps for a cleaner display.
func formatDate(s string) string {
	if idx := strings.IndexByte(s, 'T'); idx > 0 {
		return s[:idx]
	}
	return s
}

func init() {
	hubRootCmd.AddCommand(hubUpdatesCmd)
}
