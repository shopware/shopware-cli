package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-version"
)

// upgradeManifestName is the temporary Composer manifest written into the
// project root for the resolution check. It lives next to composer.json so
// relative path repositories keep resolving, and is removed afterwards
// together with its lock file. Composer picks it up through the COMPOSER
// environment variable and never touches the real composer.json/composer.lock.
const upgradeManifestName = ".shopware-cli-upgrade-composer.json"

// PackageChange is one resolved lock-file difference: the version a package
// moves to when the upgrade runs.
type PackageChange struct {
	Name string
	// From is empty for new installs; To is empty for removals.
	From string
	To   string
	// Op is "install", "upgrade", "downgrade", or "remove".
	Op string
}

// ResolveResult is the outcome of the "Composer can resolve this upgrade" check.
type ResolveResult struct {
	OK bool
	// Report is Composer's raw output; when the resolution fails it is the
	// diagnostic to attach to the upgrade report and support tickets.
	Report string
	// Changes are the lock-file differences of a successful resolution — the
	// exact versions Composer will install.
	Changes []PackageChange
}

// SecurityBlocked reports whether the resolution failed because Composer
// (>= 2.9 with audit blocking enabled) refused to load packages affected by
// known security advisories.
func (r ResolveResult) SecurityBlocked() bool {
	return !r.OK && strings.Contains(r.Report, "affected by security advisories")
}

// ResolvedVersion returns the version a package resolves to, or "" when the
// resolution does not change it.
func (r ResolveResult) ResolvedVersion(pkg string) string {
	for _, change := range r.Changes {
		if change.Name == pkg {
			return change.To
		}
	}
	return ""
}

// CheckComposerResolvable verifies that Composer can find an installable set
// of dependencies for the target version without modifying any project file.
// It runs `composer update --no-install` against a temporary manifest, which
// resolves and writes a temporary lock file; diffing that against the real
// composer.lock yields the exact versions the upgrade will install.
func (u *ProjectUpgrader) CheckComposerResolvable(ctx context.Context, target string) (ResolveResult, error) {
	manifest, err := u.renderUpgradeManifest(target)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("prepare upgrade manifest: %w", err)
	}

	manifestPath := filepath.Join(u.projectRoot, upgradeManifestName)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		return ResolveResult{}, fmt.Errorf("write upgrade manifest: %w", err)
	}
	lockPath := filepath.Join(u.projectRoot, lockNameFor(upgradeManifestName))
	defer func() {
		_ = os.Remove(manifestPath)
		_ = os.Remove(lockPath)
	}()

	// Run through the project's executor so the resolution uses the same PHP
	// and Composer the actual upgrade will use (in-container for Docker
	// environments). The manifest lives inside the project root, which is
	// mounted there.
	proc := u.executor.WithEnv(u.composerEnv(map[string]string{"COMPOSER": upgradeManifestName})).
		ComposerCommand(ctx, "update",
			"--no-install", "--no-interaction", "--no-scripts", "--no-plugins", "--no-progress", "--no-ansi",
			"--with-all-dependencies")

	output, err := proc.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ResolveResult{}, ctx.Err()
		}
		return ResolveResult{OK: false, Report: string(output)}, nil
	}

	return ResolveResult{
		OK:      true,
		Report:  string(output),
		Changes: diffLocks(filepath.Join(u.projectRoot, "composer.lock"), lockPath),
	}, nil
}

// diffLocks compares the current lock file with the freshly resolved one and
// returns the package-level differences, sorted by name.
func diffLocks(currentPath, resolvedPath string) []PackageChange {
	current := lockVersions(currentPath)
	resolved := lockVersions(resolvedPath)
	if resolved == nil {
		return nil
	}

	var changes []PackageChange
	for name, to := range resolved {
		from, existed := current[name]
		switch {
		case !existed:
			changes = append(changes, PackageChange{Name: name, To: to, Op: "install"})
		case from != to:
			changes = append(changes, PackageChange{Name: name, From: from, To: to, Op: changeOp(from, to)})
		}
	}
	for name, from := range current {
		if _, kept := resolved[name]; !kept {
			changes = append(changes, PackageChange{Name: name, From: from, Op: "remove"})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Name < changes[j].Name })
	return changes
}

// lockVersions reads a composer lock file into a package -> version map.
func lockVersions(path string) map[string]string {
	lock, err := composer.ReadLock(path)
	if err != nil {
		return nil
	}

	versions := make(map[string]string, len(lock.Packages))
	for _, pkg := range lock.Packages {
		versions[pkg.Name] = strings.TrimPrefix(pkg.Version, "v")
	}
	return versions
}

// changeOp classifies a version change as upgrade or downgrade.
func changeOp(from, to string) string {
	fromVersion, errFrom := version.NewVersion(from)
	toVersion, errTo := version.NewVersion(to)
	if errFrom != nil || errTo != nil {
		return "upgrade"
	}
	if toVersion.LessThan(fromVersion) {
		return "downgrade"
	}
	return "upgrade"
}

// VersionMap returns the resolved package -> version map (installs and
// up/downgrades; removals are excluded).
func (r ResolveResult) VersionMap() map[string]string {
	versions := make(map[string]string, len(r.Changes))
	for _, change := range r.Changes {
		if change.To != "" {
			versions[change.Name] = change.To
		}
	}
	return versions
}

// ApplyResolvedVersions overwrites each extension's Available version with
// the exact release the resolution picked, where one was found.
func ApplyResolvedVersions(results []ExtensionResult, resolve ResolveResult) {
	for i := range results {
		if results[i].Extension.Package == "" {
			continue
		}
		if to := resolve.ResolvedVersion(results[i].Extension.Package); to != "" {
			results[i].Available = to
		}
	}
}

// lockNameFor maps a Composer manifest file name to its lock file name, the
// same way Composer does (composer.json -> composer.lock).
func lockNameFor(manifest string) string {
	ext := filepath.Ext(manifest)
	return manifest[:len(manifest)-len(ext)] + ".lock"
}
