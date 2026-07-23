package upgradetui

import (
	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/picker"
)

// versionPickerKey marks results coming from the version picker overlay.
type versionPickerKey struct{}

const (
	versionColWidth     = 16
	supportLeftColWidth = 14
)

// newVersionPicker builds the "Select a supported Shopware version" overlay: a
// filterable table of every upgrade candidate with its support window.
func newVersionPicker(catalog *upgrade.Catalog) *picker.Overlay {
	items := make([]picker.Item, len(catalog.Options))
	for i, opt := range catalog.Options {
		left := opt.SupportLeft()
		if left == "" {
			left = "—"
		}
		kind := opt.SupportType
		if kind == "" {
			kind = "—"
		}
		items[i] = picker.Item{
			Label: tui.PadRight(opt.Version.String(), versionColWidth) + tui.PadRight(left, supportLeftColWidth) + kind,
			Value: opt.Version.String(),
		}
	}

	return picker.New(picker.Options{
		Key:         versionPickerKey{},
		Title:       "Select a supported Shopware version",
		Help:        "Current project: Shopware " + catalog.Current.String(),
		Items:       items,
		Placeholder: "Filter versions: 6.7",
		Header: tui.PadRight("Version", versionColWidth) +
			tui.PadRight("Support left", supportLeftColWidth) + "Support type",
		MaxWidth: 66,
	})
}
