package extension

import (
	"context"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/spdx"
	"github.com/shopware/shopware-cli/internal/validation"
)

func validateExtensionIcon(ext Extension, check validation.Check) {
	fullIconPath := ext.GetIconPath()
	relPath, err := filepath.Rel(ext.GetRootDir(), fullIconPath)
	if err != nil {
		relPath = fullIconPath
	}

	info, err := os.Stat(fullIconPath)

	if os.IsNotExist(err) {
		check.AddResult(validation.CheckResult{
			Path:       relPath,
			Identifier: "metadata.icon",
			Message:    fmt.Sprintf("The extension icon %s does not exist", relPath),
			Severity:   validation.SeverityError,
		})
	} else if err == nil {
		if info.Size() > 30*1024 {
			check.AddResult(validation.CheckResult{
				Path:       relPath,
				Identifier: "metadata.icon.size",
				Message:    fmt.Sprintf("The extension icon %s is bigger than 30kb. Please think of loading times and shrink your PNG file with tools like tinypng.com to less than 30kb", relPath),
				Severity:   validation.SeverityError,
			})
		}

		file, err := os.Open(fullIconPath)
		if err != nil {
			check.AddResult(validation.CheckResult{
				Path:       relPath,
				Identifier: "metadata.icon",
				Message:    fmt.Sprintf("Could not open icon file %s: %s", relPath, err.Error()),
				Severity:   validation.SeverityError,
			})
		} else {
			config, _, err := image.DecodeConfig(file)
			if err != nil {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Identifier: "metadata.icon",
					Message:    fmt.Sprintf("Could not decode icon image %s: %s", relPath, err.Error()),
					Severity:   validation.SeverityError,
				})
			} else {
				if config.Width < 112 || config.Height < 112 {
					check.AddResult(validation.CheckResult{
						Path:       relPath,
						Identifier: "metadata.icon.size",
						Message:    fmt.Sprintf("The extension icon %s dimensions (%dx%d) are smaller than required 112x112 and maximum 256x256 pixels with max file size 30kb and 72dpi", relPath, config.Width, config.Height),
						Severity:   validation.SeverityError,
					})
				} else if config.Width > 256 || config.Height > 256 {
					check.AddResult(validation.CheckResult{
						Path:       relPath,
						Identifier: "metadata.icon.size",
						Message:    fmt.Sprintf("The extension icon %s dimensions (%dx%d) are larger than maximum 256x256 pixels with max file size 30kb and 72dpi", relPath, config.Width, config.Height),
						Severity:   validation.SeverityError,
					})
				}
			}

			if err := file.Close(); err != nil {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Identifier: "metadata.icon",
					Message:    fmt.Sprintf("Failed to close icon file %s: %s", relPath, err.Error()),
					Severity:   validation.SeverityError,
				})
			}
		}
	}
}

func RunValidation(ctx context.Context, ext Extension, check validation.Check) {
	runDefaultValidate(ext, check)
	ext.Validate(ctx, check)
	validateAdministrationSnippets(ext, check)
	validateStorefrontSnippets(ext, check)
	validateAssets(ext, check)
	validateExtensionIcon(ext, check)
	// Note: ignores are now applied in the verifier layer
}

func runDefaultValidate(ext Extension, check validation.Check) {
	_, versionErr := ext.GetVersion()
	name, nameErr := ext.GetName()
	_, shopwareVersionErr := ext.GetShopwareVersionConstraint()

	rootFile := "composer.json"

	if ext.GetType() == TypePlatformApp {
		rootFile = "manifest.xml"
	}

	// Skip version validation for ShopwareBundle
	if versionErr != nil && ext.GetType() != TypeShopwareBundle {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.version",
			Message:    versionErr.Error(),
			Severity:   validation.SeverityError,
		})
	}

	if nameErr != nil {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.name",
			Message:    nameErr.Error(),
			Severity:   validation.SeverityError,
		})
	}

	if shopwareVersionErr != nil {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.shopware_version",
			Message:    shopwareVersionErr.Error(),
			Severity:   validation.SeverityError,
		})
	}

	if len(name) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.name",
			Message:    "Extension name cannot be empty",
			Severity:   validation.SeverityError,
		})
	}

	notAllowedErrorFormat := "file %s is not allowed in the zip file"
	_ = filepath.Walk(ext.GetPath(), func(p string, info fs.FileInfo, _ error) error {
		base := filepath.Base(p)

		if base == ".." {
			check.AddResult(validation.CheckResult{
				Path:       p,
				Identifier: "zip.path_travel",
				Message:    "Path travel detected in zip file",
				Severity:   validation.SeverityError,
			})
		}

		for _, file := range defaultNotAllowedPaths {
			if strings.HasPrefix(p, file) {
				check.AddResult(validation.CheckResult{
					Path:       p,
					Identifier: "zip.disallowed_file",
					Message:    fmt.Sprintf(notAllowedErrorFormat, p),
					Severity:   validation.SeverityError,
				})
			}
		}

		for _, file := range defaultNotAllowedFiles {
			if file == base {
				check.AddResult(validation.CheckResult{
					Path:       p,
					Identifier: "zip.disallowed_file",
					Message:    fmt.Sprintf(notAllowedErrorFormat, p),
					Severity:   validation.SeverityError,
				})
			}
		}

		for _, extFile := range defaultNotAllowedExtensions {
			if strings.HasSuffix(base, extFile) {
				check.AddResult(validation.CheckResult{
					Path:       p,
					Identifier: "zip.disallowed_file",
					Message:    fmt.Sprintf(notAllowedErrorFormat, p),
					Severity:   validation.SeverityError,
				})
			}
		}

		license, err := ext.GetLicense()

		if err != nil {
			check.AddResult(validation.CheckResult{
				Path:       rootFile,
				Identifier: "metadata.license",
				Message:    fmt.Sprintf("Could not read the license of the extension: %s", err.Error()),
				Severity:   validation.SeverityError,
			})
		} else if strings.TrimSpace(strings.ToLower(license)) != "proprietary" {
			spdxList, err := spdx.NewSpdxLicenses()
			if err != nil {
				check.AddResult(validation.CheckResult{
					Path:       rootFile,
					Identifier: "metadata.license",
					Message:    fmt.Sprintf("Could not load the SPDX license list: %s", err.Error()),
					Severity:   validation.SeverityWarning,
				})
			} else {
				valid, err := spdxList.Validate(license)
				if err != nil {
					check.AddResult(validation.CheckResult{
						Path:       rootFile,
						Identifier: "metadata.license",
						Message:    fmt.Sprintf("Could not validate the license: %s", err.Error()),
						Severity:   validation.SeverityError,
					})
				} else if !valid {
					check.AddResult(validation.CheckResult{
						Path:       rootFile,
						Identifier: "metadata.license",
						Message:    fmt.Sprintf("The license %s is not a valid SPDX license", license),
						Severity:   validation.SeverityError,
					})
				}
			}
		}

		return nil
	})

	metaData := ext.GetMetaData()
	if len([]rune(metaData.Label.German)) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.label",
			Message:    "in composer.json, label is not translated in german",
			Severity:   validation.SeverityError,
		})
	}

	if len(metaData.Label.English) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       rootFile,
			Identifier: "metadata.label",
			Message:    "in composer.json, label is not translated in english",
			Severity:   validation.SeverityError,
		})
	}

	// Skip description validation for ShopwareBundle
	if ext.GetType() != TypeShopwareBundle {
		if len([]rune(metaData.Description.German)) == 0 {
			check.AddResult(validation.CheckResult{
				Path:       rootFile,
				Identifier: "metadata.description",
				Message:    "in composer.json, description is not translated in german",
				Severity:   validation.SeverityError,
			})
		}

		if len(metaData.Description.English) == 0 {
			check.AddResult(validation.CheckResult{
				Path:       rootFile,
				Identifier: "metadata.description",
				Message:    "in composer.json, description is not translated in english",
				Severity:   validation.SeverityError,
			})
		}

		if len([]rune(metaData.Description.German)) < 150 || len([]rune(metaData.Description.German)) > 185 {
			check.AddResult(validation.CheckResult{
				Path:       rootFile,
				Identifier: "metadata.description",
				Message:    fmt.Sprintf("in composer.json, the german description with length of %d should have a length from 150 up to 185 characters.", len([]rune(metaData.Description.German))),
				Severity:   validation.SeverityError,
			})
		}

		if len(metaData.Description.English) < 150 || len(metaData.Description.English) > 185 {
			check.AddResult(validation.CheckResult{
				Path:       rootFile,
				Identifier: "metadata.description",
				Message:    fmt.Sprintf("in composer.json, the english description with length of %d should have a length from 150 up to 185 characters.", len(metaData.Description.English)),
				Severity:   validation.SeverityError,
			})
		}
	}
}
