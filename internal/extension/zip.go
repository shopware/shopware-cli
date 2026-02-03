package extension

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/changelog"
	"github.com/shopware/shopware-cli/logging"
)

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
	composerInstallCmd := exec.CommandContext(ctx, "composer", "install", "-d", path, "--no-dev", "-n", "-o")
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

			defer func() {
				if err := composerFile.Close(); err != nil {
					log.Printf("failed to close composer file: %v", err)
				}
			}()

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

// PrepareExtensionForRelease Remove secret from the manifest.
// sourceRoot is the original folder (contains also .git).
func PrepareExtensionForRelease(ctx context.Context, sourceRoot, extensionRoot string, ext Extension) error {
	if ext.GetExtensionConfig().Changelog.Enabled {
		v, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Generated changelog for version %s", v.String())

		content, err := changelog.GenerateChangelog(ctx, v.String(), sourceRoot, ext.GetExtensionConfig().Changelog)
		if err != nil {
			return err
		}

		changelogFile := fmt.Sprintf("# %s\n%s", v.String(), content)

		logging.FromContext(ctx).Debugf("Changelog:\n%s", changelogFile)

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
