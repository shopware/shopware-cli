// Package pluginmigrate moves extensions living in custom/ under Composer
// management: Shopware Store plugins are required from the Store packagist
// (and their local copy removed), everything else is registered as a Composer
// path repository. Afterwards `project upgrade` can resolve every extension.
package pluginmigrate

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

// StoreRepositoryURL is the Shopware packagist serving Store plugins.
const StoreRepositoryURL = "https://packages.shopware.com"

// PluginMigrator migrates a project's custom/ extensions to Composer.
//
// External dependencies live in unexported fields with production defaults so
// tests can replace them per instance.
type PluginMigrator struct {
	projectRoot string
	executor    executor.Executor

	// storePackageNames fetches the package names available on the Shopware
	// packagist for the given token.
	storePackageNames func(ctx context.Context, token string) (map[string]struct{}, error)
	// publishedVersions looks the given package names up in the repositories
	// the project already uses (packagist.org + configured composer repos).
	publishedVersions func(ctx context.Context, names []string) map[string][]string
}

// NewPluginMigrator creates a migrator for the project at projectRoot. The
// executor runs composer and the console in the project's environment.
func NewPluginMigrator(projectRoot string, exec executor.Executor) *PluginMigrator {
	m := &PluginMigrator{
		projectRoot: projectRoot,
		executor:    exec,

		storePackageNames: fetchStorePackageNames,
	}
	m.publishedVersions = m.fetchPublishedVersions
	return m
}

func fetchStorePackageNames(ctx context.Context, token string) (map[string]struct{}, error) {
	return fetchPackageNames(ctx, StoreRepositoryURL, token)
}

func fetchPackageNames(ctx context.Context, repoURL, token string) (map[string]struct{}, error) {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("parse repository URL: %w", err)
	}

	// Composer keys auth.json bearer tokens by hostname, and go-composer looks
	// them up the same way.
	auth := &composer.Auth{BearerAuth: map[string]string{parsed.Host: token}}
	packages, err := repository.New(repoURL, auth).GetPackages(ctx)
	if err != nil {
		return nil, err
	}

	names := make(map[string]struct{}, len(packages))
	for name := range packages {
		names[name] = struct{}{}
	}
	return names, nil
}

// Availability describes which reachable Composer repositories can serve the
// scanned extensions.
type Availability struct {
	// Store lists the package names on packages.shopware.com the token can
	// access (nil without a token).
	Store map[string]struct{}
	// Published maps a scanned extension's composer name to the versions the
	// project's regular repositories offer (packagist.org plus the composer
	// repositories configured in composer.json). Needs no token.
	Published map[string][]string
}

// FetchAvailability queries packages.shopware.com (when a token is given —
// this doubles as the token validation) and the project's regular Composer
// repositories for the scanned extensions.
func (m *PluginMigrator) FetchAvailability(ctx context.Context, token string, extensions []ScannedExtension) (Availability, error) {
	var avail Availability

	if token != "" {
		store, err := m.storePackageNames(ctx, token)
		if err != nil {
			return Availability{}, err
		}
		avail.Store = store
	}

	names := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if ext.ComposerName != "" {
			names = append(names, ext.ComposerName)
		}
	}
	avail.Published = m.publishedVersions(ctx, names)

	return avail, nil
}

// fetchPublishedVersions looks each package up in the repositories the project
// already uses, mirroring Composer's resolution order. Failed lookups only
// make the extension fall back to a path repository, so they are not fatal.
func (m *PluginMigrator) fetchPublishedVersions(ctx context.Context, names []string) map[string][]string {
	if len(names) == 0 {
		return nil
	}

	repos := m.projectRepositories()
	published := make(map[string][]string)
	for _, name := range names {
		pkg, _, err := repos.GetPackage(ctx, name)
		if err != nil {
			continue
		}
		versions := make([]string, 0, len(pkg.Versions))
		for _, v := range pkg.Versions {
			versions = append(versions, v.Version)
		}
		published[name] = versions
	}
	return published
}

// projectRepositories builds the repository set the project itself resolves
// against: the composer repositories from composer.json (with auth.json
// credentials) plus public packagist.org.
func (m *PluginMigrator) projectRepositories() *repository.Set {
	composerJSON, err := composer.ReadJson(filepath.Join(m.projectRoot, "composer.json"))
	if err != nil {
		return repository.NewSet(repository.New(repository.PackagistURL, nil))
	}

	auth, err := composer.ReadAuth(filepath.Join(m.projectRoot, "auth.json"))
	if err != nil {
		// An unreadable auth.json must not drop COMPOSER_AUTH credentials.
		auth = &composer.Auth{}
	}
	_ = auth.MergeEnv()

	return repository.FromComposer(composerJSON, auth, true)
}

// ScannedExtension is one extension found below custom/.
type ScannedExtension struct {
	Name         string
	ComposerName string
	Version      string
	// Path is absolute; RelPath is relative to the project root
	// (e.g. custom/plugins/SwagDemo).
	Path    string
	RelPath string
}

// Scan lists the extensions living below custom/ — the ones Composer does not
// manage. It is read-only.
func (m *PluginMigrator) Scan(ctx context.Context) []ScannedExtension {
	found := extension.FindExtensionsFromProject(logging.DisableLogger(ctx), m.projectRoot, false)

	// Extension paths come back symlink-resolved; resolve the root the same
	// way so the prefix comparison holds (e.g. /var vs /private/var on macOS).
	root := m.projectRoot
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	customDir := filepath.Join(root, "custom")

	var result []ScannedExtension
	for _, ext := range found {
		extPath := filepath.Clean(ext.GetPath())
		if abs, err := filepath.Abs(extPath); err == nil {
			extPath = abs
		}
		if resolved, err := filepath.EvalSymlinks(extPath); err == nil {
			extPath = resolved
		}
		rel, err := filepath.Rel(customDir, extPath)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}

		name, err := ext.GetName()
		if err != nil {
			continue
		}

		scanned := ScannedExtension{
			Name:    name,
			Path:    extPath,
			RelPath: filepath.ToSlash(filepath.Join("custom", rel)),
		}
		if composerName, err := ext.GetComposerName(); err == nil {
			scanned.ComposerName = composerName
		}
		if v, err := ext.GetVersion(); err == nil && v != nil {
			scanned.Version = v.String()
		}

		result = append(result, scanned)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
