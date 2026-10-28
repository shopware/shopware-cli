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
	"sort"
	"strconv"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/sbom"
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

	// Keep version ranges such as >=6.6.5 readable instead of \u003e=6.6.5.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bom); err != nil {
		return fmt.Errorf("cannot marshal SBOM: %w", err)
	}
	data := buf.Bytes()

	for _, outputPath := range outputPaths {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("cannot create SBOM output directory: %w", err)
		}

		if err := os.WriteFile(outputPath, data, 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("cannot write SBOM %s: %w", outputPath, err)
		}

		logging.FromContext(ctx).Infof("Wrote SBOM with %d components to %s", len(bom.Components), outputPath)
	}

	return nil
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
	// Apps have no composer.json; their identity comes from manifest.xml.
	var composerJSON *composer.Json
	composerPath := filepath.Join(root, "composer.json")
	if _, err := os.Stat(composerPath); err == nil {
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
	}

	rootComponent := ExtensionComponent{Component: *base.Metadata.Component}
	bom.Metadata.Component = &rootComponent

	devSet := make(map[string]bool, len(installed.devPackages))
	for _, name := range installed.devPackages {
		devSet[name] = true
	}

	bundledComposer := make(map[string]bool, len(base.Components))
	for _, component := range base.Components {
		extComponent := ExtensionComponent{Component: component}
		name := strings.TrimPrefix(component.Group+"/"+component.Name, "/")
		bundledComposer[name] = true
		if devSet[name] {
			extComponent.Properties = append(extComponent.Properties, sbomProperty{Name: "shopware-cli:composer:dev", Value: "true"})
		}
		bom.Components = append(bom.Components, extComponent)
	}

	if composerJSON != nil {
		enrichRootComponent(&rootComponent, composerJSON)
		bom.Metadata.Manufacturer = manufacturerFromComposer(composerJSON)

		bom.Components = append(bom.Components, externalComposerComponents(composerJSON, bundledComposer)...)
	}

	npmComponents, err := bundledNpmComponents(ext, root)
	if err != nil {
		return nil, err
	}
	bom.Components = append(bom.Components, npmComponents...)

	linkRootDependencies(bom, base.Metadata.Component.BOMRef)

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
	group, name := splitComposerPackageName(composerJSON.Name)
	component.Group = group
	component.Name = name
	component.Description = composerJSON.Description

	purl := "pkg:composer/" + composerJSON.Name
	if component.Version != "" {
		purl += "@" + url.PathEscape(component.Version)
	}
	component.PURL = purl

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
			if link, ok := links[key].(string); ok && link != "" && !containsString(org.URL, link) {
				org.URL = append(org.URL, link)
			}
		}
	}

	if org.Name == "" {
		return nil
	}

	return org
}

func externalComposerComponents(composerJSON *composer.Json, bundled map[string]bool) []ExtensionComponent {
	names := make([]string, 0, len(composerJSON.Require))
	for name := range composerJSON.Require {
		names = append(names, name)
	}
	sort.Strings(names)

	components := make([]ExtensionComponent, 0, len(names))
	for _, name := range names {
		if bundled[name] || isComposerPlatformRequirement(name) {
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

func isComposerPlatformRequirement(name string) bool {
	if name == "php" {
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
	Version     string `json:"version"`
	Resolved    string `json:"resolved"`
	Integrity   string `json:"integrity"`
	License     any    `json:"license"`
	Dev         bool   `json:"dev"`
	DevOptional bool   `json:"devOptional"`
	Link        bool   `json:"link"`
}

// bundledNpmComponents lists the production npm dependencies of the
// Administration and Storefront sources. They end up in the compiled
// JavaScript, so they are part of the shipped extension.
func bundledNpmComponents(ext Extension, root string) ([]ExtensionComponent, error) {
	indexByPURL := map[string]int{}
	components := make([]ExtensionComponent, 0)

	for _, lockPath := range npmLockFiles(ext, root) {
		content, err := os.ReadFile(lockPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", lockPath, err)
		}

		var lock struct {
			Packages map[string]npmLockPackage `json:"packages"`
		}
		if err := json.Unmarshal(content, &lock); err != nil {
			return nil, fmt.Errorf("cannot parse %s: %w", lockPath, err)
		}

		keys := make([]string, 0, len(lock.Packages))
		for key := range lock.Packages {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			pkg := lock.Packages[key]
			idx := strings.LastIndex(key, "node_modules/")
			if key == "" || idx == -1 || pkg.Dev || pkg.DevOptional || pkg.Link || pkg.Version == "" {
				continue
			}

			name := key[idx+len("node_modules/"):]
			purl := npmPURL(name, pkg.Version)

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
	}

	return components, nil
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

	return sbom.Hash{Alg: alg, Content: hex.EncodeToString(digest)}, true
}

// linkRootDependencies makes the root component depend on every component,
// including the external and npm ones go-composer/sbom does not know about.
func linkRootDependencies(bom *ExtensionBOM, rootRef string) {
	refs := make([]string, 0, len(bom.Components))
	for _, component := range bom.Components {
		refs = append(refs, component.BOMRef)
	}
	sort.Strings(refs)

	for i, dependency := range bom.Dependencies {
		if dependency.Ref == rootRef {
			bom.Dependencies[i].DependsOn = refs
			return
		}
	}

	bom.Dependencies = append([]sbom.Dependency{{Ref: rootRef, DependsOn: refs}}, bom.Dependencies...)
}

var spdxLikeLicense = regexp.MustCompile(`^[A-Za-z0-9.+-]+$`)

func sbomLicense(license string) sbom.LicenseChoice {
	if spdxLikeLicense.MatchString(license) && !strings.EqualFold(license, "proprietary") {
		return sbom.LicenseChoice{License: sbom.License{ID: license}}
	}

	return sbom.LicenseChoice{License: sbom.License{Name: license}}
}

func splitComposerPackageName(name string) (string, string) {
	if group, pkgName, ok := strings.Cut(name, "/"); ok {
		return group, pkgName
	}

	return "", name
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
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

	sort.SliceStable(bounds, func(i, j int) bool {
		vi, errI := version.NewVersion(bounds[i].version)
		vj, errJ := version.NewVersion(bounds[j].version)
		if errI != nil || errJ != nil {
			return bounds[i].version < bounds[j].version
		}
		return vi.LessThan(vj)
	})

	rendered := make([]string, 0, len(bounds))
	for _, b := range bounds {
		rendered = append(rendered, b.op+b.version)
	}

	return "vers:composer/" + strings.Join(rendered, "|")
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
