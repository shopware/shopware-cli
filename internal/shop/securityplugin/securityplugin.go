// Package securityplugin reports whether the Shopware Security Plugin
// (SwagPlatformSecurity) is present in a project, active in the shop and at
// the latest version the Store offers for the project's Shopware version.
package securityplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-version"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/logging"
)

const (
	// TechnicalName is the plugin's technical name in the shop and the Store.
	TechnicalName = "SwagPlatformSecurity"
	// DocsURL explains what the plugin does and how to install it.
	DocsURL = "https://developer.shopware.com/docs/guides/hosting/installation-updates/security-best-practices/security-plugin.html"
)

// composerNames are the package names the plugin is locked under: the Shopware
// Store Composer repository and the name the plugin itself declares, used when
// it is required from a path repository.
var composerNames = []string{"store.shopware.com/swagplatformsecurity", "swag/platform-security"}

// State is the overall result of a check, ordered by what the developer has to
// do next.
type State int

const (
	// StateUnknown means the activation state or the latest compatible version
	// could not be determined, so no positive status can be given.
	StateUnknown State = iota
	// StateMissing means the plugin is not part of the project.
	StateMissing
	// StateInactive means the plugin is in the project but not installed and
	// activated in the shop.
	StateInactive
	// StateOutdated means a newer compatible version is available.
	StateOutdated
	// StateUpToDate means the plugin is active at the latest compatible version.
	StateUpToDate
)

// Status is the result of a check. Version is empty when the plugin is missing
// and Latest is empty when the Store could not name a compatible version.
// Package is the Composer package the plugin is locked as, empty when it is
// not managed by Composer.
type Status struct {
	State   State
	Version string
	Latest  string
	Package string
	// Unregistered marks an inactive plugin the shop does not list yet, so its
	// plugin list has to be refreshed before the plugin can be installed.
	Unregistered bool
	// Downloaded marks an outdated plugin whose latest version is already in
	// the project files, so only the shop has to apply the update.
	Downloaded bool
}

// Label describes the status in plain text.
func (s Status) Label() string {
	switch s.State {
	case StateMissing:
		return "not installed"
	case StateInactive:
		return withVersion("not active", s.Version)
	case StateOutdated:
		return fmt.Sprintf("outdated (%s, latest %s)", s.Version, s.Latest)
	case StateUpToDate:
		return withVersion("active, up to date", s.Version)
	case StateUnknown:
		return "unknown"
	}
	return "unknown"
}

func withVersion(label, v string) string {
	if v == "" {
		return label
	}
	return label + " (" + v + ")"
}

// NextSteps says how to resolve the status, one step per entry; empty when
// there is nothing to do or nothing can be recommended. How the plugin gets
// into a project depends on the project's Composer repositories, so a missing
// plugin points to the docs.
func (s Status) NextSteps() []string {
	const extension = "Run: shopware-cli project extension "

	switch s.State {
	case StateMissing:
		return []string{"See " + DocsURL}
	case StateInactive:
		// Activating also installs a plugin that is not installed yet, but
		// only one the shop lists; listing refreshes the shop's plugin list.
		activate := extension + "activate " + TechnicalName
		if s.Unregistered {
			return []string{extension + "list", activate}
		}
		return []string{activate}
	case StateOutdated:
		// Composer only downloads the new version; the shop applies it. The
		// update refreshes the shop's plugin list first.
		update := extension + "update " + TechnicalName
		switch {
		case s.Downloaded:
			return []string{update}
		case s.Package == "":
			return []string{"See " + DocsURL}
		}
		return []string{"Run: shopware-cli project composer update " + s.Package, update}
	case StateUnknown, StateUpToDate:
		return nil
	}
	return nil
}

// Checker gathers the plugin's status. PluginList and StorePlugins are the
// data sources and can be replaced in tests; a nil PluginList means the shop
// cannot be asked, which leaves the activation state unknown.
type Checker struct {
	ProjectRoot     string
	ShopwareVersion string
	// PluginList returns the output of `bin/console plugin:list --json`.
	PluginList   func(ctx context.Context) ([]byte, error)
	StorePlugins func(ctx context.Context, locale, shopwareVersion string, technicalNames []string) ([]account_api.StorePlugin, error)
}

// NewChecker builds a Checker that asks the project's shop through exec and
// the Shopware Store for the latest version compatible with shopwareVersion.
func NewChecker(projectRoot, shopwareVersion string, exec executor.Executor) *Checker {
	c := &Checker{
		ProjectRoot:     projectRoot,
		ShopwareVersion: shopwareVersion,
		StorePlugins:    account_api.GetStorePluginsByName,
	}
	if exec != nil {
		c.PluginList = func(ctx context.Context) ([]byte, error) {
			return exec.ConsoleCommand(ctx, "plugin:list", "--json").Output()
		}
	}
	return c
}

type lifecycle int

const (
	lifecycleUnknown lifecycle = iota
	lifecycleInactive
	lifecycleActive
)

// Check never changes the plugin's state and never fails: anything that cannot
// be determined is reported as unknown rather than as a positive status.
func (c *Checker) Check(ctx context.Context) Status {
	pkg, onDisk := projectPlugin(c.ProjectRoot)
	installed := onDisk
	state := lifecycleUnknown
	unregistered := false

	listed, found, ok := c.shopPlugin(ctx)
	switch {
	case !ok && onDisk == "":
		// The plugin can be known to the shop alone, so without the shop its
		// absence from the project files proves nothing.
		return Status{State: StateUnknown}
	case ok && !found && onDisk == "":
		return Status{State: StateMissing}
	case ok && found && listed.active():
		state = lifecycleActive
	case ok:
		state = lifecycleInactive
		unregistered = !found
	}
	if found && listed.Version != "" {
		// The shop's record is the version that is actually running.
		installed = listed.Version
	}

	status := Status{Version: strings.TrimPrefix(installed, "v"), Latest: c.latestVersion(ctx), Package: pkg}

	switch {
	case state == lifecycleInactive:
		status.State = StateInactive
		status.Unregistered = unregistered
	case isOlder(status.Version, status.Latest):
		status.State = StateOutdated
		status.Downloaded = isCurrent(strings.TrimPrefix(onDisk, "v"), status.Latest)
	case state == lifecycleActive && isCurrent(status.Version, status.Latest):
		status.State = StateUpToDate
	default:
		status.State = StateUnknown
	}
	return status
}

type shopPlugin struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Active      bool            `json:"active"`
	InstalledAt json.RawMessage `json:"installedAt"`
}

func (p shopPlugin) active() bool {
	installed := len(p.InstalledAt) > 0 && string(p.InstalledAt) != "null"
	return installed && p.Active
}

// shopPlugin looks the plugin up in the shop's plugin list. ok is false when
// the shop could not be asked (environment down, Shopware not installed).
func (c *Checker) shopPlugin(ctx context.Context) (plugin shopPlugin, found, ok bool) {
	if c.PluginList == nil {
		return shopPlugin{}, false, false
	}

	out, err := c.PluginList(ctx)
	if err != nil {
		logging.FromContext(ctx).Debugf("Cannot list plugins for the Security Plugin check: %v", err)
		return shopPlugin{}, false, false
	}

	// Deprecation notices can precede the JSON document.
	start := bytes.IndexByte(out, '[')
	if start < 0 {
		return shopPlugin{}, false, false
	}

	var plugins []shopPlugin
	if err := json.Unmarshal(out[start:], &plugins); err != nil {
		logging.FromContext(ctx).Debugf("Cannot parse the plugin list for the Security Plugin check: %v", err)
		return shopPlugin{}, false, false
	}

	for _, p := range plugins {
		if p.Name == TechnicalName {
			return p, true, true
		}
	}
	return shopPlugin{}, false, true
}

// latestVersion asks the Store for the newest plugin version compatible with
// the project's Shopware version; empty when it cannot be determined.
func (c *Checker) latestVersion(ctx context.Context) string {
	if c.StorePlugins == nil || c.ShopwareVersion == "" {
		return ""
	}

	plugins, err := c.StorePlugins(ctx, "en_GB", c.ShopwareVersion, []string{TechnicalName})
	if err != nil {
		logging.FromContext(ctx).Debugf("Cannot look up the Security Plugin in the Store: %v", err)
		return ""
	}
	for _, p := range plugins {
		if p.Name == TechnicalName {
			return p.Version
		}
	}
	return ""
}

// projectPlugin finds the plugin in the project's files: as a Composer package
// in composer.lock, returned with its package name, or as a directory in
// custom/plugins.
func projectPlugin(projectRoot string) (pkg, installed string) {
	if lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock")); err == nil {
		for _, name := range composerNames {
			if locked := lock.GetPackage(name); locked != nil && locked.Version != "" {
				return name, locked.Version
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, "custom", "plugins", TechnicalName, "composer.json"))
	if err != nil {
		return "", ""
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Version == "" {
		// Present, but without a version that could be compared.
		return "", "unknown version"
	}
	return "", manifest.Version
}

func isOlder(installed, latest string) bool {
	a, b, ok := parseVersions(installed, latest)
	return ok && a.LessThan(b)
}

func isCurrent(installed, latest string) bool {
	a, b, ok := parseVersions(installed, latest)
	return ok && !a.LessThan(b)
}

func parseVersions(installed, latest string) (a, b *version.Version, ok bool) {
	a, errA := version.NewVersion(installed)
	b, errB := version.NewVersion(latest)
	return a, b, errA == nil && errB == nil
}
