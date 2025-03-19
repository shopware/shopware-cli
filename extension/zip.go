package extension

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/changelog"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shyim/go-version"
)

var (
	defaultNotAllowedPaths = []string{
		".editorconfig",
		".git",
		".github",
		".gitlab-ci.yml",
		".gitpod.Dockerfile",
		".gitpod.yml",
		".php-cs-fixer.cache",
		".php_cs.cache",
		".php_cs.dist",
		".php-cs-fixer.dist.php",
		".sw-zip-blacklist",
		".travis.yml",
		"phpstan.neon",
		"phpstan.neon.dist",
		"ISSUE_TEMPLATE.md",
		"Makefile",
		"Resources/store",
		"auth.json",
		"bitbucket-pipelines.yml",
		"build.sh",
		"grumphp.yml",
		"phpunit.xml.dist",
		"psalm.xml",
		"rector.php",
		"src/Resources/app/administration/node_modules",
		"src/Resources/app/node_modules",
		"src/Resources/app/storefront/node_modules",
		"src/Resources/store",
		"tests",
		"var",
	}

	defaultNotAllowedFiles = []string{
		".DS_Store",
		"Thumbs.db",
		"__MACOSX",
		".gitignore",
		"stylelint.config.js",
		".stylelintrc.js",
		".stylelintrc",
		"eslint.config.js",
		".eslintrc.js",
	}

	defaultNotAllowedExtensions = []string{
		".zip",
		".tar",
		".gz",
		".phar",
		".rar",
	}
)

func Unzip(r *zip.Reader, dest string) error {
	errorFormat := "unzip: %w"

	for _, f := range r.File {
		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name) //nolint:gosec

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("Unzip: %s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			// Make Folder
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		_, err = io.Copy(outFile, rc) //nolint:gosec

		// Close the file without defer to close before next iteration of loop
		_ = outFile.Close()
		_ = rc.Close()

		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		// Restore the modified time
		if err := os.Chtimes(fpath, f.Modified, f.Modified); err != nil {
			return fmt.Errorf(errorFormat, err)
		}
	}

	return nil
}

func CreateZip(baseFolder, zipFile string) error {
	// Get a Buffer to Write To
	outFile, err := os.Create(zipFile)
	if err != nil {
		return fmt.Errorf("create zipfile: %w", err)
	}

	defer func() {
		_ = outFile.Close()
	}()

	// Create a new zip archive.
	w := zip.NewWriter(outFile)

	defer func() {
		_ = w.Close()
	}()

	return AddZipFiles(w, baseFolder, "")
}

func AddZipFiles(w *zip.Writer, basePath, baseInZip string) error {
	files, err := os.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("could not zip dir, basePath: %q, baseInZip: %q, %w", basePath, baseInZip, err)
	}

	for _, file := range files {
		if file.IsDir() {
			// Add files of directory recursively
			if err = AddZipFiles(w, filepath.Join(basePath, file.Name()), filepath.Join(baseInZip, file.Name())); err != nil {
				return err
			}
		} else {
			if err = addFileToZip(w, filepath.Join(basePath, file.Name()), filepath.Join(baseInZip, file.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func CleanupExtensionFolder(path string, additionalPaths []string) error {
	defaultNotAllowedPaths = append(defaultNotAllowedPaths, additionalPaths...)

	for _, folder := range defaultNotAllowedPaths {
		if _, err := os.Stat(path + folder); !os.IsNotExist(err) {
			err := os.RemoveAll(path + folder)
			if err != nil {
				return err
			}
		}
	}

	return filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// When we delete a folder, this function will be called also the files in it
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}

		base := filepath.Base(path)

		for _, file := range defaultNotAllowedFiles {
			if file == base {
				return os.RemoveAll(path)
			}
		}

		for _, ext := range defaultNotAllowedExtensions {
			if strings.HasSuffix(base, ext) {
				return os.RemoveAll(path)
			}
		}

		return nil
	})
}

func PrepareFolderForZipping(ctx context.Context, path string, ext Extension, extCfg *Config) error {
	errorFormat := "PrepareFolderForZipping: %v"
	composerJSONPath := filepath.Join(path, "composer.json")
	composerLockPath := filepath.Join(path, "composer.lock")

	if _, err := os.Stat(composerJSONPath); os.IsNotExist(err) {
		return nil
	}

	content, err := os.ReadFile(composerJSONPath)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	var composer map[string]interface{}
	err = json.Unmarshal(content, &composer)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	minShopwareVersionConstraint, err := ext.GetShopwareVersionConstraint()
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	minVersion, err := lookupForMinMatchingVersion(ctx, minShopwareVersionConstraint)
	if err != nil {
		return fmt.Errorf("lookup for min matching version: %w", err)
	}

	shopware65Constraint, _ := version.NewConstraint(">=6.5.0")

	if shopware65Constraint.Check(version.Must(version.NewVersion(minVersion))) {
		logging.FromContext(ctx).Info("Shopware 6.5 detected, disabling composer replacements")
		return nil
	}

	// Add replacements
	composer, err = addComposerReplacements(composer, minVersion)
	if err != nil {
		return fmt.Errorf("add composer replacements: %w", err)
	}

	filtered := filterRequires(composer, extCfg)

	if len(filtered["require"].(map[string]interface{})) == 0 {
		return nil
	}

	// Remove the composer.lock
	if _, err := os.Stat(composerLockPath); !os.IsNotExist(err) {
		err := os.Remove(composerLockPath)
		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}
	}

	newContent, err := json.Marshal(&composer)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	err = os.WriteFile(composerJSONPath, newContent, 0o644) //nolint:gosec
	if err != nil {
		// Revert on failure
		_ = os.WriteFile(composerJSONPath, content, 0o644) //nolint:gosec
		return fmt.Errorf(errorFormat, err)
	}

	// Execute composer in this directory
	composerInstallCmd := exec.Command("composer", "install", "-d", path, "--no-dev", "-n", "-o")
	composerInstallCmd.Stdout = os.Stdout
	composerInstallCmd.Stderr = os.Stderr
	err = composerInstallCmd.Run()
	if err != nil {
		// Revert on failure
		_ = os.WriteFile(composerJSONPath, content, 0o644) //nolint:gosec
		return fmt.Errorf(errorFormat, err)
	}

	_ = os.WriteFile(composerJSONPath, content, 0o644) //nolint:gosec

	return nil
}

func addFileToZip(zipWriter *zip.Writer, sourcePath string, zipPath string) error {
	zipErrorFormat := "could not zip file, sourcePath: %q, zipPath: %q, %w"

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	f, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	if _, err := io.Copy(f, file); err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	return nil
}

func filterRequires(composer map[string]interface{}, extCfg *Config) map[string]interface{} {
	if _, ok := composer["provide"]; !ok {
		composer["provide"] = make(map[string]interface{})
	}
	if _, ok := composer["require"]; !ok {
		composer["require"] = make(map[string]interface{})
	}

	provide := composer["provide"]
	require := composer["require"]

	keys := []string{"shopware/platform", "shopware/core", "shopware/shopware", "shopware/storefront", "shopware/administration", "shopware/elasticsearch", "composer/installers"}
	if extCfg != nil {
		keys = append(keys, extCfg.Build.Zip.Composer.ExcludedPackages...)
	}

	for _, key := range keys {
		if _, ok := require.(map[string]interface{})[key]; ok {
			delete(require.(map[string]interface{}), key)
			provide.(map[string]interface{})[key] = "*"
		}
	}

	return composer
}

func addComposerReplacements(composer map[string]interface{}, minVersion string) (map[string]interface{}, error) {
	if _, ok := composer["replace"]; !ok {
		composer["replace"] = make(map[string]interface{})
	}

	if _, ok := composer["require"]; !ok {
		composer["require"] = make(map[string]interface{})
	}

	replace := composer["replace"]
	require := composer["require"]

	components := []string{"core", "administration", "storefront", "administration"}

	composerInfo, err := getComposerInfoFS()
	if err != nil {
		return nil, fmt.Errorf("get composer info fs: %w", err)
	}

	for _, component := range components {
		packageName := fmt.Sprintf("shopware/%s", component)

		if _, ok := require.(map[string]interface{})[packageName]; ok {
			composerFile, err := composerInfo.Open(fmt.Sprintf("%s/%s.json", minVersion, component))
			if err != nil {
				return nil, fmt.Errorf("open composer file: %w", err)
			}

			defer composerFile.Close()

			composerPartByte, err := io.ReadAll(composerFile)
			if err != nil {
				return nil, fmt.Errorf("read component version body: %w", err)
			}

			var composerPart map[string]string
			err = json.Unmarshal(composerPartByte, &composerPart)
			if err != nil {
				return nil, fmt.Errorf("unmarshal component version: %w", err)
			}

			for k, v := range composerPart {
				if _, userReplaced := replace.(map[string]interface{})[k]; userReplaced {
					continue
				}

				replace.(map[string]interface{})[k] = v

				delete(require.(map[string]interface{}), k)
			}
		}
	}

	return composer, nil
}

type packagistResponse struct {
	Packages struct {
		Core []struct {
			Version string `json:"version_normalized"`
		} `json:"shopware/core"`
	} `json:"packages"`
}

func GetShopwareVersions(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://repo.packagist.org/p2/shopware/core.json", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create composer version request: %w", err)
	}

	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch composer versions: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("lookupForMinMatchingVersion: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch composer versions: %s", resp.Status)
	}

	var pckResponse packagistResponse

	var versions []string

	if err := json.NewDecoder(resp.Body).Decode(&pckResponse); err != nil {
		return nil, fmt.Errorf("decode composer versions: %w", err)
	}

	for _, v := range pckResponse.Packages.Core {
		versions = append(versions, v.Version)
	}

	return versions, nil
}

func lookupForMinMatchingVersion(ctx context.Context, versionConstraint *version.Constraints) (string, error) {
	versions, err := GetShopwareVersions(ctx)
	if err != nil {
		return "", fmt.Errorf("get shopware versions: %w", err)
	}

	return getMinMatchingVersion(versionConstraint, versions), nil
}

const DevVersionNumber = "6.9.9.9"

func getMinMatchingVersion(constraint *version.Constraints, versions []string) string {
	vs := make([]*version.Version, 0)

	for _, r := range versions {
		v, err := version.NewVersion(r)
		if err != nil {
			continue
		}

		vs = append(vs, v)
	}

	sort.Sort(version.Collection(vs))

	matchingVersions := make([]*version.Version, 0)

	for _, v := range vs {
		if constraint.Check(v) {
			matchingVersions = append(matchingVersions, v)
		}
	}

	// If there are matching versions, return the first non-prerelease version
	for _, matchingVersion := range matchingVersions {
		if matchingVersion.IsPrerelease() {
			continue
		}

		return matchingVersion.String()
	}

	// If there are no non-prerelease versions, return the first matching version
	if len(matchingVersions) > 0 {
		return matchingVersions[0].String()
	}

	return DevVersionNumber
}

// PrepareExtensionForRelease Remove secret from the manifest.
// sourceRoot is the original folder (contains also .git).
func PrepareExtensionForRelease(ctx context.Context, sourceRoot, extensionRoot string, ext Extension) error {
	if ext.GetExtensionConfig().Changelog.Enabled {
		v, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Generated changelog for version %s", v.String())

		content, err := changelog.GenerateChangelog(ctx, sourceRoot, ext.GetExtensionConfig().Changelog)
		if err != nil {
			return err
		}

		changelogFile := fmt.Sprintf("# %s\n%s", v.String(), content)

		if err := os.WriteFile(path.Join(extensionRoot, "CHANGELOG_en-GB.md"), []byte(changelogFile), os.ModePerm); err != nil {
			return err
		}
	}

	if ext.GetType() == "plugin" {
		return nil
	}

	manifestPath := filepath.Join(extensionRoot, "manifest.xml")

	bytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var manifest Manifest

	if err := xml.Unmarshal(bytes, &manifest); err != nil {
		return fmt.Errorf("unmarshal manifest failed: %w", err)
	}

	if manifest.Setup != nil {
		manifest.Setup.Secret = ""
	}

	newManifest, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal manifest failed: %w", err)
	}

	if err := os.WriteFile(manifestPath, newManifest, os.ModePerm); err != nil {
		return err
	}

	return nil
}
