package extension

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/sbom"
	spdx "github.com/shyim/go-spdx"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

// DefaultExtensionSBOMOutput is the filename written into the extension root
// when no output path is given.
const DefaultExtensionSBOMOutput = "sbom.cdx.json"

const sbomPropertyComposerConstraint = "shopware-cli:composer:constraint"

// SBOMOptions configures GenerateSBOM.
type SBOMOptions struct {
	// ToolVersion is reported in the SBOM metadata tools entry (shopware-cli version).
	ToolVersion string
}

// ExtensionBOM is a CycloneDX document for a single extension. It extends the
// project SBOM with the CycloneDX 1.6 fields an extension needs: packages the
// shop provides are listed as external components with a version range instead
// of a resolved version, because they are not part of the shipped ZIP.
type ExtensionBOM struct {
	*sbom.BOM
	Metadata   extensionBOMMetadata `json:"metadata"`
	Components []ExtensionComponent `json:"components"`
}

type extensionBOMMetadata struct {
	sbom.Metadata
	Component    *ExtensionComponent `json:"component,omitempty"`
	Manufacturer *sbomOrganization   `json:"manufacturer,omitempty"`
}

// ExtensionComponent is a CycloneDX component with the fields go-composer/sbom does not model.
type ExtensionComponent struct {
	sbom.Component
	VersionRange string         `json:"versionRange,omitempty"`
	IsExternal   bool           `json:"isExternal,omitempty"`
	Properties   []sbomProperty `json:"properties,omitempty"`
}

type sbomOrganization struct {
	Name string   `json:"name"`
	URL  []string `json:"url,omitempty"`
}

type sbomProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WriteSBOM generates the SBOM for the extension folder root once and writes
// the same document to every output path.
func WriteSBOM(ctx context.Context, ext Extension, root string, opts SBOMOptions, outputPaths ...string) error {
	bom, err := GenerateSBOM(ctx, ext, root, opts)
	if err != nil {
		return err
	}

	data, err := MarshalSBOM(bom)
	if err != nil {
		return err
	}

	for _, outputPath := range outputPaths {
		if err := WriteSBOMFile(outputPath, data); err != nil {
			return err
		}

		logging.FromContext(ctx).Infof("Wrote SBOM with %d components to %s", len(bom.Components), outputPath)
	}

	return nil
}

// MarshalSBOM renders the document as indented JSON.
func MarshalSBOM(bom *ExtensionBOM) ([]byte, error) {
	// Keep version ranges such as >=6.6.5 readable instead of \u003e=6.6.5.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bom); err != nil {
		return nil, fmt.Errorf("cannot marshal SBOM: %w", err)
	}

	return buf.Bytes(), nil
}

// WriteSBOMFile writes data to outputPath through a temporary file in the same
// directory. A failed write leaves an existing file untouched, and an existing
// symlink at outputPath is replaced instead of followed.
func WriteSBOMFile(outputPath string, data []byte) error {
	staged, err := StageSBOMFile(outputPath, data)
	if err != nil {
		return err
	}

	if err := os.Rename(staged, outputPath); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("cannot write SBOM %s: %w", outputPath, err)
	}

	return nil
}

// StageSBOMFile writes data to a temporary file next to outputPath and returns
// its path, so the caller can rename it into place once the rest of its work
// succeeded.
func StageSBOMFile(outputPath string, data []byte) (string, error) {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create SBOM output directory: %w", err)
	}

	file, err := os.CreateTemp(dir, "."+filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("cannot write SBOM %s: %w", outputPath, err)
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = os.Chmod(file.Name(), 0o644) //nolint:gosec
	}
	if writeErr != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("cannot write SBOM %s: %w", outputPath, writeErr)
	}

	return file.Name(), nil
}

// GenerateSBOM builds a CycloneDX document for the extension located at root.
//
// Bundled components are what physically ends up in the extension folder:
// composer packages from vendor/composer/installed.json and npm packages from
// the package-lock.json files of the Administration and Storefront sources.
// Composer requirements that are not bundled (Shopware itself, and since
// Shopware 6.5 any package the shop installs for the plugin) are listed as
// external components with their version constraint.
func GenerateSBOM(ctx context.Context, ext Extension, root string, opts SBOMOptions) (*ExtensionBOM, error) {
	// An app is described by manifest.xml. A composer.json in an app belongs to
	// its backend and must not replace the app's name or version.
	var composerJSON *composer.Json
	composerPath := filepath.Join(root, "composer.json")
	if _, err := os.Stat(composerPath); err == nil && ext.GetType() != TypePlatformApp {
		if composerJSON, err = composer.ReadJson(composerPath); err != nil {
			return nil, fmt.Errorf("cannot read composer.json: %w", err)
		}
	}

	rootName, rootVersion, err := sbomRootIdentity(ext, composerJSON)
	if err != nil {
		return nil, err
	}

	installed, err := readInstalledPackages(root)
	if err != nil {
		return nil, err
	}

	if len(installed.devPackages) > 0 {
		logging.FromContext(ctx).Warnf("Composer dev dependencies are bundled in the extension and listed in the SBOM: %s", strings.Join(installed.devPackages, ", "))
	}

	toolVersion := opts.ToolVersion
	if toolVersion == "" {
		toolVersion = "dev"
	}

	base, err := sbom.Generate(&composer.Lock{Packages: installed.packages}, sbom.Options{
		ApplicationName:    rootName,
		ApplicationVersion: rootVersion,
		ToolGroup:          "shopware",
		ToolName:           "shopware-cli",
		ToolVersion:        toolVersion,
	})
	if err != nil {
		return nil, err
	}

	bom := &ExtensionBOM{
		BOM:      base,
		Metadata: extensionBOMMetadata{Metadata: base.Metadata},
		// An extension without dependencies has an empty array, never null.
		Components: make([]ExtensionComponent, 0, len(base.Components)),
	}

	rootComponent := ExtensionComponent{Component: *base.Metadata.Component}
	bom.Metadata.Component = &rootComponent

	devSet := make(map[string]bool, len(installed.devPackages))
	for _, name := range installed.devPackages {
		devSet[name] = true
	}

	installedByName := make(map[string]composer.LockPackage, len(installed.packages))
	for _, pkg := range installed.packages {
		installedByName[pkg.Name] = pkg
	}

	bundledComposer := make(map[string]string, len(base.Components))
	for _, component := range base.Components {
		extComponent := ExtensionComponent{Component: component}
		name := strings.TrimPrefix(component.Group+"/"+component.Name, "/")
		bundledComposer[name] = component.BOMRef

		// go-composer copies licenses and dist hashes as they are; normalize them
		// like the extension's own license and drop malformed digests.
		extComponent.Licenses = nil
		for _, license := range installedByName[name].License {
			extComponent.Licenses = append(extComponent.Licenses, sbomLicense(license))
		}
		extComponent.Hashes = slices.DeleteFunc(slices.Clone(component.Hashes), func(hash sbom.Hash) bool {
			return !isValidSBOMHash(hash)
		})

		if devSet[name] {
			extComponent.Properties = append(extComponent.Properties, sbomProperty{Name: "shopware-cli:composer:dev", Value: "true"})
		}
		bom.Components = append(bom.Components, extComponent)
	}

	directRefs := make([]string, 0)

	if composerJSON != nil {
		enrichRootComponent(&rootComponent, composerJSON)
		bom.Metadata.Manufacturer = manufacturerFromComposer(composerJSON)

		externals := externalComposerComponents(composerJSON, bundledComposer)
		bom.Components = append(bom.Components, externals...)

		directRefs = append(directRefs, directComposerRefs(composerJSON, bundledComposer, externals)...)
	} else if license, err := ext.GetLicense(); err == nil && license != "" {
		rootComponent.Licenses = append(rootComponent.Licenses, sbomLicense(license))
	}

	npmComponents, npmDirectRefs, npmEdges, err := bundledNpmComponents(ext, root)
	if err != nil {
		return nil, err
	}
	bom.Components = append(bom.Components, npmComponents...)
	bom.Dependencies = append(bom.Dependencies, npmEdges...)
	directRefs = append(directRefs, npmDirectRefs...)

	linkRootDependencies(bom, base.Metadata.Component.BOMRef, directRefs)

	return bom, nil
}

// sbomRootIdentity prefers composer.json from the folder, because the package
// command writes an overwritten version there after the extension was loaded.
func sbomRootIdentity(ext Extension, composerJSON *composer.Json) (string, string, error) {
	if composerJSON != nil && composerJSON.Name != "" {
		v := composerJSON.Version
		if v == "" {
			if extVersion, err := ext.GetVersion(); err == nil {
				v = extVersion.String()
			}
		}
		return composerJSON.Name, v, nil
	}

	name, err := ext.GetName()
	if err != nil {
		return "", "", fmt.Errorf("cannot get extension name: %w", err)
	}

	extVersion, err := ext.GetVersion()
	if err != nil {
		return "", "", fmt.Errorf("cannot get extension version: %w", err)
	}

	return name, extVersion.String(), nil
}

func enrichRootComponent(component *ExtensionComponent, composerJSON *composer.Json) {
	component.Description = composerJSON.Description

	// Without a composer name (an app may ship a composer.json) the identity
	// from the extension itself is kept.
	if composerJSON.Name != "" {
		group, name := splitComposerPackageName(composerJSON.Name)
		component.Group = group
		component.Name = name

		purl := "pkg:composer/" + composerJSON.Name
		if component.Version != "" {
			purl += "@" + url.PathEscape(component.Version)
		}
		component.PURL = purl
	}

	if composerJSON.License != nil {
		for _, license := range composerJSON.License.Strings() {
			component.Licenses = append(component.Licenses, sbomLicense(license))
		}
	}

	if composerJSON.Homepage != "" {
		component.ExternalReferences = append(component.ExternalReferences, sbom.ExternalReference{Type: "website", URL: composerJSON.Homepage})
	}
}

// manufacturerFromComposer uses the shopware manufacturerLink and the first
// author, which is what the Shopware Store displays as the extension vendor.
func manufacturerFromComposer(composerJSON *composer.Json) *sbomOrganization {
	org := &sbomOrganization{}

	if len(composerJSON.Authors) > 0 {
		org.Name = composerJSON.Authors[0].Name
		if composerJSON.Authors[0].Homepage != "" {
			org.URL = append(org.URL, composerJSON.Authors[0].Homepage)
		}
	}

	if links, ok := composerJSON.Extra.GetMap("manufacturerLink"); ok {
		for _, key := range []string{"en-GB", "de-DE"} {
			if link, ok := links[key].(string); ok && link != "" && !slices.Contains(org.URL, link) {
				org.URL = append(org.URL, link)
			}
		}
	}

	if org.Name == "" {
		return nil
	}

	return org
}

func externalComposerComponents(composerJSON *composer.Json, bundled map[string]string) []ExtensionComponent {
	names := make([]string, 0, len(composerJSON.Require))
	for name := range composerJSON.Require {
		names = append(names, name)
	}
	sort.Strings(names)

	components := make([]ExtensionComponent, 0, len(names))
	for _, name := range names {
		if _, ok := bundled[name]; ok || isComposerPlatformRequirement(name) {
			continue
		}

		constraint := composerJSON.Require[name]
		group, pkgName := splitComposerPackageName(name)
		purl := "pkg:composer/" + name

		component := ExtensionComponent{
			Component: sbom.Component{
				Type:   "library",
				BOMRef: purl,
				Group:  group,
				Name:   pkgName,
				PURL:   purl,
			},
			IsExternal:   true,
			VersionRange: composerConstraintToVers(constraint),
			Properties:   []sbomProperty{{Name: sbomPropertyComposerConstraint, Value: constraint}},
		}

		if name == "php" {
			component.Type = "platform"
			component.Group = ""
			component.Name = "php"
			component.PURL = ""
			component.BOMRef = "platform:php"
		}

		components = append(components, component)
	}

	return components
}

// directComposerRefs returns the components for the composer.json
// requirements: the bundled package where there is one, otherwise the external
// component.
func directComposerRefs(composerJSON *composer.Json, bundled map[string]string, externals []ExtensionComponent) []string {
	externalByName := make(map[string]string, len(externals))
	for _, external := range externals {
		externalByName[strings.TrimPrefix(external.Group+"/"+external.Name, "/")] = external.BOMRef
	}

	refs := make([]string, 0, len(composerJSON.Require))
	for name := range composerJSON.Require {
		if ref, ok := bundled[name]; ok {
			refs = append(refs, ref)
		} else if ref, ok := externalByName[name]; ok {
			refs = append(refs, ref)
		}
	}

	// Installed dev requirements are only present in a working copy; link them
	// so they stay reachable from the root.
	for name := range composerJSON.RequireDev {
		if ref, ok := bundled[name]; ok {
			refs = append(refs, ref)
		}
	}

	return refs
}

func isComposerPlatformRequirement(name string) bool {
	// Platform packages have no vendor prefix; php-http/discovery is a regular package.
	if name == "php" || strings.Contains(name, "/") {
		return false
	}

	return strings.HasPrefix(name, "php-") ||
		strings.HasPrefix(name, "ext-") ||
		strings.HasPrefix(name, "lib-") ||
		name == "composer-plugin-api" ||
		name == "composer-runtime-api" ||
		name == "composer"
}

type installedPackages struct {
	packages    []composer.LockPackage
	devPackages []string
}

// readInstalledPackages reads vendor/composer/installed.json, the list of
// composer packages that are physically present in the extension folder.
func readInstalledPackages(root string) (installedPackages, error) {
	installedPath := filepath.Join(root, "vendor", "composer", "installed.json")

	content, err := os.ReadFile(installedPath)
	if os.IsNotExist(err) {
		return installedPackages{}, nil
	}
	if err != nil {
		return installedPackages{}, fmt.Errorf("cannot read %s: %w", installedPath, err)
	}

	var installed struct {
		Packages        []composer.LockPackage `json:"packages"`
		DevPackageNames []string               `json:"dev-package-names"`
	}

	if err := json.Unmarshal(content, &installed); err != nil {
		return installedPackages{}, fmt.Errorf("cannot parse %s: %w", installedPath, err)
	}

	sort.Slice(installed.Packages, func(i, j int) bool {
		return installed.Packages[i].Name < installed.Packages[j].Name
	})
	sort.Strings(installed.DevPackageNames)

	return installedPackages{packages: installed.Packages, devPackages: installed.DevPackageNames}, nil
}

type npmLockPackage struct {
	// Name is set when it differs from the install path, e.g. for aliases.
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
	License              any               `json:"license"`
	Dev                  bool              `json:"dev"`
	Link                 bool              `json:"link"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	// PeerDependencies are installed by npm 7 and newer.
	PeerDependencies map[string]string `json:"peerDependencies"`
}

// bundledNpmComponents lists the production npm dependencies of the
// Administration and Storefront sources. They end up in the compiled
// JavaScript, so they are part of the shipped extension. It also returns the
// direct dependencies of the npm projects and the package-to-package edges.
func bundledNpmComponents(ext Extension, root string) ([]ExtensionComponent, []string, []sbom.Dependency, error) {
	indexByPURL := map[string]int{}
	components := make([]ExtensionComponent, 0)
	directRefs := make([]string, 0)
	edges := map[string]map[string]struct{}{}

	for _, lockPath := range npmLockFiles(ext, root) {
		content, err := os.ReadFile(lockPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("cannot read %s: %w", lockPath, err)
		}

		var lock struct {
			LockfileVersion int                       `json:"lockfileVersion"`
			Packages        map[string]npmLockPackage `json:"packages"`
		}
		if err := json.Unmarshal(content, &lock); err != nil {
			return nil, nil, nil, fmt.Errorf("cannot parse %s: %w", lockPath, err)
		}

		// lockfileVersion 1 (npm 6 and older) has no "packages" map, so its
		// packages would silently be missing from the SBOM.
		if lock.LockfileVersion < 2 {
			return nil, nil, nil, fmt.Errorf("cannot read npm packages from %s: lockfileVersion %d is not supported, regenerate it with npm 7 or newer", lockPath, lock.LockfileVersion)
		}

		keys := make([]string, 0, len(lock.Packages))
		for key := range lock.Packages {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		purlByKey := make(map[string]string, len(keys))

		for _, key := range keys {
			pkg := lock.Packages[key]
			idx := strings.LastIndex(key, "node_modules/")
			// devOptional packages stay: they can be reached through an optional
			// production dependency.
			if key == "" || idx == -1 || pkg.Dev || pkg.Link || pkg.Version == "" {
				continue
			}

			name := key[idx+len("node_modules/"):]
			if pkg.Name != "" {
				name = pkg.Name
			}
			purl := npmPURL(name, pkg.Version)
			purlByKey[key] = purl

			group, pkgName := "", name
			if strings.HasPrefix(name, "@") {
				group, pkgName, _ = strings.Cut(name, "/")
			}

			component := ExtensionComponent{
				Component: sbom.Component{
					Type:    "library",
					BOMRef:  purl,
					Group:   group,
					Name:    pkgName,
					Version: pkg.Version,
					PURL:    purl,
				},
			}

			if license, ok := pkg.License.(string); ok && license != "" {
				component.Licenses = append(component.Licenses, sbomLicense(license))
			}

			if hash, ok := npmIntegrityToHash(pkg.Integrity); ok {
				component.Hashes = append(component.Hashes, hash)
			}

			if pkg.Resolved != "" {
				component.ExternalReferences = append(component.ExternalReferences, sbom.ExternalReference{Type: "distribution", URL: pkg.Resolved})
			}

			// The same package can be installed several times (nested node_modules,
			// or both lock files); keep one entry and fill in what the first one lacks.
			if idx, ok := indexByPURL[purl]; ok {
				existing := &components[idx]
				if len(existing.Licenses) == 0 {
					existing.Licenses = component.Licenses
				}
				if len(existing.Hashes) == 0 {
					existing.Hashes = component.Hashes
				}
				if len(existing.ExternalReferences) == 0 {
					existing.ExternalReferences = component.ExternalReferences
				}
				continue
			}

			indexByPURL[purl] = len(components)
			components = append(components, component)
		}

		directRefs = append(directRefs, collectNpmEdges(lock.Packages, purlByKey, edges)...)
	}

	return components, directRefs, npmDependencies(components, edges), nil
}

// collectNpmEdges adds an edge from every included package to the packages
// it requires and returns the direct dependencies of the npm project.
func collectNpmEdges(packages map[string]npmLockPackage, purlByKey map[string]string, edges map[string]map[string]struct{}) []string {
	directRefs := make([]string, 0)

	for key, pkg := range packages {
		from, included := purlByKey[key]
		if key != "" && !included {
			continue
		}

		for _, dependencies := range []map[string]string{pkg.Dependencies, pkg.OptionalDependencies, pkg.PeerDependencies} {
			for name := range dependencies {
				target, ok := resolveNpmDependency(key, name, purlByKey)
				if !ok || target == from {
					continue
				}

				if key == "" {
					directRefs = append(directRefs, target)
					continue
				}

				if edges[from] == nil {
					edges[from] = map[string]struct{}{}
				}
				edges[from][target] = struct{}{}
			}
		}
	}

	return directRefs
}

// resolveNpmDependency finds the install path npm uses for a dependency of the
// package at key: its own node_modules first, then those of its ancestors.
func resolveNpmDependency(key, name string, purlByKey map[string]string) (string, bool) {
	dir := key
	for {
		candidate := "node_modules/" + name
		if dir != "" {
			candidate = dir + "/" + candidate
		}
		if purl, ok := purlByKey[candidate]; ok {
			return purl, true
		}

		if dir == "" {
			return "", false
		}

		if idx := strings.LastIndex(dir, "/node_modules/"); idx != -1 {
			dir = dir[:idx]
		} else {
			dir = ""
		}
	}
}

// npmDependencies returns one dependency entry per npm component, in
// component order, with its sorted direct dependencies.
func npmDependencies(components []ExtensionComponent, edges map[string]map[string]struct{}) []sbom.Dependency {
	dependencies := make([]sbom.Dependency, 0, len(components))
	for _, component := range components {
		dependsOn := make([]string, 0, len(edges[component.BOMRef]))
		for target := range edges[component.BOMRef] {
			dependsOn = append(dependsOn, target)
		}
		slices.Sort(dependsOn)
		dependencies = append(dependencies, sbom.Dependency{Ref: component.BOMRef, DependsOn: dependsOn})
	}

	return dependencies
}

func npmLockFiles(ext Extension, root string) []string {
	rootDir := "src"
	if ext != nil {
		if rel, err := filepath.Rel(ext.GetPath(), ext.GetRootDir()); err == nil && !strings.HasPrefix(rel, "..") {
			rootDir = rel
		}
	}

	candidates := []string{
		filepath.Join(root, rootDir, "Resources", "app", "package-lock.json"),
		filepath.Join(root, rootDir, "Resources", "app", "administration", "package-lock.json"),
		filepath.Join(root, rootDir, "Resources", "app", "storefront", "package-lock.json"),
	}

	files := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			files = append(files, candidate)
		}
	}

	return files
}

func npmPURL(name, pkgVersion string) string {
	return "pkg:npm/" + strings.ReplaceAll(name, "@", "%40") + "@" + url.PathEscape(pkgVersion)
}

// npmIntegrityToHash converts a Subresource Integrity string (sha512-<base64>)
// into a CycloneDX hash with a hex digest.
func npmIntegrityToHash(integrity string) (sbom.Hash, bool) {
	// An integrity string may list several hashes; the first one is used.
	fields := strings.Fields(integrity)
	if len(fields) == 0 {
		return sbom.Hash{}, false
	}

	algo, encoded, ok := strings.Cut(fields[0], "-")
	if !ok {
		return sbom.Hash{}, false
	}

	names := map[string]string{"sha1": "SHA-1", "sha256": "SHA-256", "sha384": "SHA-384", "sha512": "SHA-512"}
	alg, known := names[algo]
	if !known {
		return sbom.Hash{}, false
	}

	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return sbom.Hash{}, false
	}

	hash := sbom.Hash{Alg: alg, Content: hex.EncodeToString(digest)}

	return hash, isValidSBOMHash(hash)
}

// isValidSBOMHash reports whether the hash is a hex digest of the length
// CycloneDX requires for its algorithm.
func isValidSBOMHash(hash sbom.Hash) bool {
	lengths := map[string]int{"SHA-1": 40, "SHA-256": 64, "SHA-384": 96, "SHA-512": 128}
	expected, ok := lengths[hash.Alg]
	if !ok || len(hash.Content) != expected {
		return false
	}

	_, err := hex.DecodeString(hash.Content)

	return err == nil
}

// linkRootDependencies sets the root component's direct dependencies. The
// edges between packages from go-composer/sbom stay as they are.
func linkRootDependencies(bom *ExtensionBOM, rootRef string, directRefs []string) {
	refs := slices.Clone(directRefs)
	slices.Sort(refs)
	refs = slices.Compact(refs)

	for i, dependency := range bom.Dependencies {
		if dependency.Ref == rootRef {
			bom.Dependencies[i].DependsOn = refs
			return
		}
	}

	bom.Dependencies = append([]sbom.Dependency{{Ref: rootRef, DependsOn: refs}}, bom.Dependencies...)
}

var (
	spdxLicensesOnce sync.Once
	spdxLicenses     *spdx.SpdxLicenses
)

// sbomLicense uses the CycloneDX license id only for an SPDX license list
// entry and writes it in its canonical spelling (mit becomes MIT), because
// license.id is an enumeration of SPDX identifiers. Expressions
// (MIT OR Apache-2.0), LicenseRef-*, NONE, NOASSERTION and values such as
// proprietary or UNLICENSED go into the name.
func sbomLicense(license string) sbom.LicenseChoice {
	spdxLicensesOnce.Do(func() {
		if licenses, err := spdx.NewSpdxLicenses(); err == nil {
			spdxLicenses = licenses
		}
	})

	if spdxLicenses != nil {
		if entry, ok := spdxLicenses.GetLicenseByIdentifier(license); ok {
			return sbom.LicenseChoice{License: sbom.License{ID: entry.Identifier}}
		}
	}

	return sbom.LicenseChoice{License: sbom.License{Name: license}}
}

func splitComposerPackageName(name string) (string, string) {
	if group, pkgName, ok := strings.Cut(name, "/"); ok {
		return group, pkgName
	}

	return "", name
}

var composerVersionPattern = regexp.MustCompile(`^v?(\d+)(?:\.(\d+|\*|x))?(?:\.(\d+|\*|x))?(?:\.(\d+|\*|x))?$`)

type versBound struct {
	op      string
	version string
}

// composerConstraintToVers converts a composer constraint into a CycloneDX
// versionRange (vers:composer/...). It returns an empty string for constraints
// it cannot express exactly (alternatives, hyphen ranges, stability flags); the
// raw constraint is always kept in a component property.
func composerConstraintToVers(constraint string) string {
	constraint = strings.TrimSpace(constraint)
	if constraint == "*" || constraint == "" {
		return "vers:composer/*"
	}

	if strings.Contains(constraint, "|") || strings.Contains(constraint, " - ") || strings.Contains(constraint, "@") {
		return ""
	}

	// Composer accepts whitespace between operator and version (">= 8.3").
	tokens := make([]string, 0, 2)
	for _, token := range strings.FieldsFunc(constraint, func(r rune) bool { return r == ',' || r == ' ' }) {
		if len(tokens) > 0 && strings.Trim(tokens[len(tokens)-1], "<>=!^~") == "" {
			tokens[len(tokens)-1] += token
			continue
		}
		tokens = append(tokens, token)
	}

	bounds := make([]versBound, 0, 2)
	for _, token := range tokens {
		tokenBounds, ok := composerTokenToVersBounds(token)
		if !ok {
			return ""
		}
		bounds = append(bounds, tokenBounds...)
	}

	rendered, ok := intersectVersBounds(bounds)
	if !ok {
		return ""
	}

	return "vers:composer/" + strings.Join(rendered, "|")
}

// intersectVersBounds reduces the bounds to the composer intersection: one
// lower and one upper bound at most, because vers rejects consecutive bounds of
// the same direction. Exact versions and != are only kept on their own.
func intersectVersBounds(bounds []versBound) ([]string, bool) {
	if len(bounds) == 1 {
		return []string{bounds[0].op + bounds[0].version}, true
	}

	var lower, upper *versBound
	var lowerVersion, upperVersion *version.Version

	for i := range bounds {
		bound := &bounds[i]
		if bound.op == "" || bound.op == "!=" {
			return nil, false
		}

		v, err := version.NewVersion(bound.version)
		if err != nil {
			return nil, false
		}

		if strings.HasPrefix(bound.op, ">") {
			if lower == nil || v.GreaterThan(lowerVersion) || (v.Equal(lowerVersion) && bound.op == ">") {
				lower, lowerVersion = bound, v
			}
			continue
		}

		if upper == nil || v.LessThan(upperVersion) || (v.Equal(upperVersion) && bound.op == "<") {
			upper, upperVersion = bound, v
		}
	}

	rendered := make([]string, 0, 2)
	if lower != nil {
		rendered = append(rendered, lower.op+lower.version)
	}
	if upper != nil {
		rendered = append(rendered, upper.op+upper.version)
	}

	// An empty or single-version intersection is left to the raw constraint.
	if lower != nil && upper != nil && !lowerVersion.LessThan(upperVersion) {
		return nil, false
	}

	return rendered, true
}

// composerTokenToVersBounds converts a single constraint such as ^6.6, ~6.6.0,
// 6.6.* or >=6.6.5 into vers bounds.
func composerTokenToVersBounds(token string) ([]versBound, bool) {
	op := ""
	for _, candidate := range []string{">=", "<=", "!=", "==", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(token, candidate) {
			op = candidate
			break
		}
	}

	match := composerVersionPattern.FindStringSubmatch(strings.TrimPrefix(token, op))
	if match == nil {
		return nil, false
	}

	parts := make([]int, 0, 4)
	wildcard := false
	for _, part := range match[1:] {
		if part == "" {
			break
		}
		if part == "*" || part == "x" {
			wildcard = true
			break
		}
		n, _ := strconv.Atoi(part)
		parts = append(parts, n)
	}

	exact := op == "" || op == "=" || op == "=="

	// Composer bumps a different segment for four-part ^ and ~ constraints;
	// leave those to the raw constraint.
	if (op == "^" || op == "~") && len(parts) > 3 {
		return nil, false
	}

	switch {
	case wildcard && !exact:
		return nil, false
	case wildcard:
		return []versBound{{">=", joinVersion(parts)}, {"<", joinVersion(bumpAt(parts, len(parts)-1))}}, true
	case op == "^":
		idx := 0
		for idx < len(parts)-1 && parts[idx] == 0 {
			idx++
		}
		return []versBound{{">=", joinVersion(parts)}, {"<", joinVersion(bumpAt(parts, idx))}}, true
	case op == "~":
		idx := max(len(parts)-2, 0)
		return []versBound{{">=", joinVersion(parts)}, {"<", joinVersion(bumpAt(parts, idx))}}, true
	case exact:
		return []versBound{{"", joinVersion(parts)}}, true
	default:
		return []versBound{{op, joinVersion(parts)}}, true
	}
}

// bumpAt increments the part at idx and drops everything after it, e.g.
// bumpAt([6 6 5], 1) -> [6 7].
func bumpAt(parts []int, idx int) []int {
	bumped := make([]int, idx+1)
	copy(bumped, parts[:idx+1])
	bumped[idx]++

	return bumped
}

func joinVersion(parts []int) string {
	strParts := make([]string, 0, 3)
	for _, part := range parts {
		strParts = append(strParts, strconv.Itoa(part))
	}
	for len(strParts) < 3 {
		strParts = append(strParts, "0")
	}

	return strings.Join(strParts, ".")
}
