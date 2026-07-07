package projectupgrade

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// ExtensionState classifies how an installed extension relates to the
// selected upgrade target. The order encodes risk: lower values are riskier
// and sort to the top of the queue.
type ExtensionState int

const (
	// ExtensionBlocked means composer found no release of the extension that
	// is compatible with the target version. Blockers prevent the upgrade
	// from starting until the user resolves them.
	ExtensionBlocked ExtensionState = iota
	// ExtensionDeprecated means the package is abandoned upstream and should
	// be replaced rather than updated.
	ExtensionDeprecated
	// ExtensionUpdate means composer will move the extension to a newer
	// release as part of the upgrade.
	ExtensionUpdate
	// ExtensionOK means the installed release already works with the target
	// version; composer keeps it as-is.
	ExtensionOK
	// ExtensionRemove means the user decided to drop the extension from
	// composer.json during the upgrade (the only way past a blocker without
	// a vendor release).
	ExtensionRemove
)

// ExtensionRow is one entry of the upgrade wizard's extension queue.
type ExtensionRow struct {
	// Name is the composer package name (e.g. "swag/paypal").
	Name string
	// Current is the installed version.
	Current string
	// Target is the version composer resolves for the upgrade. Equal to
	// Current when the extension is kept as-is; empty when composer found no
	// compatible release.
	Target string
	// State classifies the row; see ExtensionState.
	State ExtensionState
	// Result is a short human-readable outcome shown in the queue table.
	Result string
	// Replacement is the package suggested by upstream when the extension is
	// abandoned (empty when none was suggested).
	Replacement string
}

// RequiredPluginVersions returns the installed version of every required
// shopware-platform-plugin, keyed by composer package name.
func RequiredPluginVersions(composerJsonPath string) (map[string]string, error) {
	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return nil, err
	}

	installed, err := packagist.ReadInstalledJson(filepath.Dir(composerJsonPath))
	if err != nil {
		return nil, err
	}

	plugins := make(map[string]string)
	for _, pkg := range installed.Packages {
		if pkg.Type != composerPluginType {
			continue
		}
		if _, ok := composerJson.Require[pkg.Name]; !ok {
			continue
		}
		plugins[pkg.Name] = strings.TrimPrefix(pkg.Version, "v")
	}

	return plugins, nil
}

var (
	// composerOperationRe matches composer's lock/package operation lines,
	// e.g. "  - Upgrading swag/paypal (v8.0.0 => v9.2.0)".
	composerOperationRe = regexp.MustCompile(`^\s*- (Upgrading|Downgrading|Installing|Removing) (\S+) \((.+)\)`)
	// composerAbandonedRe matches composer's abandoned-package warnings, e.g.
	// "Package swag/legacy is abandoned, you should avoid using it. Use swag/new instead."
	composerAbandonedRe = regexp.MustCompile(`Package (\S+) is abandoned, you should avoid using it\.(?: Use (.+) instead\.?| No replacement was suggested\.?)?`)
)

// composerOperations extracts the resolved version movements and abandoned
// warnings for the given packages from composer output lines.
func composerOperations(output []string, packages map[string]string) (targets map[string]string, abandoned map[string]string) {
	targets = make(map[string]string)
	abandoned = make(map[string]string)

	for _, line := range output {
		if m := composerOperationRe.FindStringSubmatch(line); m != nil {
			name := m[2]
			if _, ok := packages[name]; !ok {
				continue
			}
			switch m[1] {
			case "Upgrading", "Downgrading":
				if parts := strings.SplitN(m[3], " => ", 2); len(parts) == 2 {
					targets[name] = strings.TrimPrefix(strings.TrimSpace(parts[1]), "v")
				}
			}
			continue
		}

		if m := composerAbandonedRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			if _, ok := packages[name]; !ok {
				continue
			}
			abandoned[name] = strings.TrimSuffix(strings.TrimSpace(m[2]), ".")
		}
	}

	return targets, abandoned
}

// BuildExtensionQueue derives the wizard's extension compatibility queue from
// the installed plugin versions and composer's dry-run verdict. Rows are
// sorted by risk (blocked first, then deprecated, updates, ok) and
// alphabetically within the same state.
func BuildExtensionQueue(installed map[string]string, report CompatReport) []ExtensionRow {
	blocked := make(map[string]struct{}, len(report.BlockingPlugins))
	for _, name := range report.BlockingPlugins {
		blocked[name] = struct{}{}
	}

	targets, abandoned := composerOperations(report.Output, installed)

	rows := make([]ExtensionRow, 0, len(installed))
	for name, current := range installed {
		row := ExtensionRow{Name: name, Current: current}

		replacement, isAbandoned := abandoned[name]
		target, hasUpdate := targets[name]

		switch {
		case !report.OK && isBlocked(blocked, name):
			row.State = ExtensionBlocked
			row.Result = "no compatible release"
		case isAbandoned:
			row.State = ExtensionDeprecated
			row.Replacement = replacement
			row.Target = current
			if hasUpdate {
				row.Target = target
			}
			if replacement != "" {
				row.Result = "deprecated, replaced by " + replacement
			} else {
				row.Result = "deprecated, no replacement"
			}
		case hasUpdate:
			row.State = ExtensionUpdate
			row.Target = target
			row.Result = "will be updated"
		default:
			row.State = ExtensionOK
			row.Target = current
			row.Result = "compatible as installed"
		}

		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].State != rows[j].State {
			return rows[i].State < rows[j].State
		}
		return rows[i].Name < rows[j].Name
	})

	return rows
}

func isBlocked(blocked map[string]struct{}, name string) bool {
	_, ok := blocked[name]
	return ok
}

// CountBlockers returns how many rows still block the upgrade.
func CountBlockers(rows []ExtensionRow) int {
	n := 0
	for _, r := range rows {
		if r.State == ExtensionBlocked {
			n++
		}
	}
	return n
}
