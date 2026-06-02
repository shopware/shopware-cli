package projectupgrade

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/packagist"
)

// composerPluginType is the composer "type" used by Shopware platform plugins.
const composerPluginType = "shopware-platform-plugin"

// PluginAction describes how the resolver dealt with one plugin that blocked
// the upgrade.
type PluginAction struct {
	// Name is the composer package name (e.g. "swag/paypal").
	Name string
	// Removed is true when the plugin was dropped from composer.json because
	// composer could not resolve it against the target version.
	Removed bool
	// Reason is a short human-readable explanation surfaced in the UI.
	Reason string
}

// ResolveResult summarises the actions the resolver took.
type ResolveResult struct {
	Actions []PluginAction
}

// Removed returns the actions that resulted in the plugin being dropped.
func (r *ResolveResult) Removed() []PluginAction {
	out := make([]PluginAction, 0, len(r.Actions))
	for _, a := range r.Actions {
		if a.Removed {
			out = append(out, a)
		}
	}
	return out
}

// CompatReport is the outcome of a dry-run `composer require` against the
// target version. It is composer's own verdict, not a homegrown constraint
// check.
type CompatReport struct {
	// OK is true when composer could resolve the requested upgrade.
	OK bool
	// Output is the captured composer output. On failure it contains the
	// "Your requirements could not be resolved" block.
	Output []string
	// BlockingPlugins are the plugin names from the upgrade set that appear in
	// composer's conflict output. Best-effort; may be empty when composer's
	// message cannot be attributed to a specific plugin.
	BlockingPlugins []string
}

// requirePackages enumerates the composer require arguments for the upgrade:
// the first-party Shopware packages present in composer.json pinned to the
// target version, plus every required shopware-platform-plugin (passed without
// a constraint so composer picks the newest release compatible with the pinned
// core).
func requirePackages(composerJsonPath, targetVersion string) ([]string, []string, error) {
	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return nil, nil, err
	}

	args := make([]string, 0, len(ShopwarePackages))
	for _, pkg := range ShopwarePackages {
		if _, ok := composerJson.Require[pkg]; ok {
			args = append(args, pkg+":"+targetVersion)
		}
	}

	plugins, err := requiredPlugins(filepath.Dir(composerJsonPath), composerJson)
	if err != nil {
		return nil, nil, err
	}

	args = append(args, plugins...)
	return args, plugins, nil
}

// requiredPlugins returns the composer package names of every required
// shopware-platform-plugin, sorted for stable output.
func requiredPlugins(projectDir string, composerJson *packagist.ComposerJson) ([]string, error) {
	installed, err := packagist.ReadInstalledJson(projectDir)
	if err != nil {
		return nil, err
	}

	plugins := make([]string, 0)
	for _, pkg := range installed.Packages {
		if pkg.Type != composerPluginType {
			continue
		}
		if _, ok := composerJson.Require[pkg.Name]; !ok {
			continue
		}
		plugins = append(plugins, pkg.Name)
	}

	sort.Strings(plugins)
	return plugins, nil
}

// composerRequire runs `composer require --no-install -W <pkgs>` (optionally a
// dry run) through the executor and returns the combined output. The error is
// the process exit error, which composer returns non-zero on an unresolvable
// requirement.
func composerRequire(ctx context.Context, exec executor.Executor, dryRun bool, pkgs []string) ([]string, error) {
	args := []string{
		"require",
		"--no-interaction",
		"--no-install",
		"--no-scripts",
		"--update-with-all-dependencies",
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, pkgs...)

	out, err := exec.ComposerCommand(ctx, args...).CombinedOutput()
	return splitLines(out), err
}

// DryRunRequire asks composer whether the project can be upgraded to
// targetVersion without changing anything on disk. It runs
// `composer require --no-install --dry-run -W shopware/core:<target> <plugins…>`
// and reports composer's verdict.
func DryRunRequire(ctx context.Context, exec executor.Executor, composerJsonPath, targetVersion string) (CompatReport, error) {
	pkgs, plugins, err := requirePackages(composerJsonPath, targetVersion)
	if err != nil {
		return CompatReport{}, err
	}

	output, runErr := composerRequire(ctx, exec, true, pkgs)
	if runErr == nil {
		return CompatReport{OK: true, Output: output}, nil
	}

	return CompatReport{
		OK:              false,
		Output:          output,
		BlockingPlugins: blockingPlugins(output, plugins),
	}, nil
}

// ApplyRequire performs the real upgrade resolution: it runs
// `composer require --no-install -W shopware/core:<target> <plugins…>`, letting
// composer rewrite composer.json/composer.lock with the bumped constraints.
// When composer cannot resolve the set, the plugin(s) it names are removed from
// composer.json and the require is retried until it resolves or no plugin can
// be attributed to the failure.
func ApplyRequire(ctx context.Context, exec executor.Executor, composerJsonPath, targetVersion string) (*ResolveResult, error) {
	pkgs, plugins, err := requirePackages(composerJsonPath, targetVersion)
	if err != nil {
		return nil, err
	}

	result := &ResolveResult{}
	dropped := make(map[string]struct{})

	// Bounded by the number of plugins: each failed attempt drops at least one.
	for attempt := 0; attempt <= len(plugins); attempt++ {
		output, runErr := composerRequire(ctx, exec, false, pkgs)
		if runErr == nil {
			return result, nil
		}

		blockers := blockingPlugins(output, plugins)
		newlyDropped := false
		for _, name := range blockers {
			if _, ok := dropped[name]; ok {
				continue
			}
			if err := removePluginFromComposer(composerJsonPath, name); err != nil {
				return nil, err
			}
			dropped[name] = struct{}{}
			result.Actions = append(result.Actions, PluginAction{
				Name:    name,
				Removed: true,
				Reason:  "composer could not resolve it for " + targetVersion,
			})
			newlyDropped = true
		}

		if !newlyDropped {
			// Composer failed but the conflict is not attributable to a plugin
			// we can drop (e.g. a platform requirement). Surface the output.
			return result, fmt.Errorf("composer could not resolve the upgrade:\n%s", strings.Join(output, "\n"))
		}

		pkgs = filterPackages(pkgs, dropped)
	}

	return result, fmt.Errorf("composer could not resolve the upgrade after dropping all incompatible plugins")
}

// removePluginFromComposer drops a plugin from the root composer.json require
// block so composer can resolve the remaining set.
func removePluginFromComposer(composerJsonPath, name string) error {
	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return err
	}
	if _, ok := composerJson.Require[name]; !ok {
		return nil
	}
	delete(composerJson.Require, name)
	return composerJson.Save()
}

// filterPackages returns pkgs without the require arguments for any dropped
// plugin. Plugin args are bare names; core args carry a ":constraint" suffix
// and are never dropped.
func filterPackages(pkgs []string, dropped map[string]struct{}) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if _, ok := dropped[p]; ok {
			continue
		}
		out = append(out, p)
	}
	return out
}

// blockingPlugins returns the plugin names that appear anywhere in composer's
// output. Composer names the conflicting packages in its "could not be
// resolved" block; we only attribute failures to plugins we asked for so the
// resolver never drops first-party packages.
func blockingPlugins(output, plugins []string) []string {
	joined := strings.Join(output, "\n")
	blockers := make([]string, 0)
	for _, name := range plugins {
		if strings.Contains(joined, name) {
			blockers = append(blockers, name)
		}
	}
	return blockers
}

func splitLines(out []byte) []string {
	trimmed := strings.TrimRight(string(out), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
