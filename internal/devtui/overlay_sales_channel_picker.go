package devtui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

type salesChannelPickerResultMsg struct {
	Cancelled bool
	Opts      extension.StorefrontWatcherOptions
}

type salesChannelPickerKey struct{}

type salesChannelsLoadedMsg struct {
	channels []salesChannelEntry
	err      error
}

type salesChannelEntry struct {
	id     string
	name   string
	domain string
	theme  *adminSdk.Theme
}

type salesChannelPicker struct {
	executor executor.Executor
	loading  bool
	err      error
	channels []salesChannelEntry
	inner    *listPicker
}

func newSalesChannelPicker(exec executor.Executor) *salesChannelPicker {
	return &salesChannelPicker{executor: exec, loading: true}
}

func (sp *salesChannelPicker) Init() tea.Cmd {
	exec := sp.executor
	return func() tea.Msg {
		client, err := exec.AdminAPIClient(context.Background())
		if err != nil {
			return salesChannelsLoadedMsg{err: err}
		}

		apiCtx := adminSdk.NewApiContext(context.Background())
		channels, err := client.SalesChannel.ListStorefront(apiCtx)
		if err != nil {
			return salesChannelsLoadedMsg{err: err}
		}

		entries := make([]salesChannelEntry, 0, len(channels))
		for _, sc := range channels {
			theme, err := client.SalesChannel.FindThemeForSalesChannel(apiCtx, sc.Id)
			if err != nil {
				return salesChannelsLoadedMsg{err: err}
			}
			entry := salesChannelEntry{id: sc.Id, name: sc.Name, theme: theme}
			if len(sc.Domains) > 0 {
				entry.domain = sc.Domains[0].Url
			}
			entries = append(entries, entry)
		}

		return salesChannelsLoadedMsg{channels: entries}
	}
}

func (sp *salesChannelPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	switch msg := msg.(type) {
	case salesChannelsLoadedMsg:
		sp.loading = false
		sp.err = msg.err
		sp.channels = msg.channels
		if sp.err == nil && len(sp.channels) > 0 {
			items := make([]listPickerItem, len(sp.channels))
			for i, c := range sp.channels {
				items[i] = listPickerItem{Label: c.name, Detail: c.domain, Value: c.id}
			}
			sp.inner = newListPicker(salesChannelPickerKey{}, "Select Sales Channel",
				"Pick the storefront the watcher should target. theme:dump is run with this channel's theme and domain.",
				items, 0)
		}
		return sp, nil

	case pickerResultMsg:
		if _, ok := msg.Key.(salesChannelPickerKey); !ok {
			return sp, nil
		}
		if msg.Cancelled {
			return nil, emit(salesChannelPickerResultMsg{Cancelled: true})
		}
		entry := sp.channels[msg.Index]
		opts := extension.StorefrontWatcherOptions{DomainURL: entry.domain}
		if entry.theme != nil {
			opts.ThemeID = entry.theme.Id
		}
		return nil, emit(salesChannelPickerResultMsg{Opts: opts})

	case tea.KeyPressMsg:
		if sp.inner == nil {
			if msg.String() == "esc" || msg.String() == keyEnter {
				return nil, emit(salesChannelPickerResultMsg{Cancelled: true})
			}
			return sp, nil
		}
		next, cmd := sp.inner.Update(msg)
		if next == nil {
			sp.inner = nil
		}
		return sp, cmd
	}

	return sp, nil
}

func (sp *salesChannelPicker) View(width, height int) string {
	if sp.inner != nil {
		return sp.inner.View(width, height)
	}

	modalWidth := min(width-4, 70)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Select Sales Channel"))
	b.WriteString("\n\n")

	switch {
	case sp.loading:
		b.WriteString("  ")
		b.WriteString(tui.StatusBadge("loading", tui.BrandColor))
		b.WriteString(" ")
		b.WriteString(helpStyle.Render("Fetching sales channels from the admin API…"))
		b.WriteString("\n\n")
		b.WriteString(tui.ShortcutBar(tui.Shortcut{Key: "esc", Label: "Cancel"}))

	case sp.err != nil:
		b.WriteString(errorStyle.Render("Could not load sales channels:"))
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(sp.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(tui.ShortcutBar(tui.Shortcut{Key: "esc", Label: "Close"}))

	default:
		b.WriteString(helpStyle.Render("No storefront sales channels found."))
		b.WriteString("\n\n")
		b.WriteString(tui.ShortcutBar(tui.Shortcut{Key: "esc", Label: "Close"}))
	}

	return centeredModal(b.String(), modalWidth, width, height)
}
