package extension

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/net/context"

	"github.com/shopware/shopware-cli/internal/spdx"
)

type ValidationMessage struct {
	Identifier string
	Message    string
}

type ValidationContext struct {
	Extension Extension
	errors    []ValidationMessage
	warnings  []ValidationMessage
}

func newValidationContext(ext Extension) *ValidationContext {
	return &ValidationContext{Extension: ext}
}

func (c *ValidationContext) AddError(id, message string) {
	c.errors = append(c.errors, ValidationMessage{Identifier: id, Message: message})
}

func (c *ValidationContext) HasErrors() bool {
	return len(c.errors) > 0
}

func (c *ValidationContext) Errors() []ValidationMessage {
	return c.errors
}

func (c *ValidationContext) AddWarning(id, message string) {
	c.warnings = append(c.warnings, ValidationMessage{Identifier: id, Message: message})
}

func (c *ValidationContext) HasWarnings() bool {
	return len(c.warnings) > 0
}

func (c *ValidationContext) Warnings() []ValidationMessage {
	return c.warnings
}

func (c *ValidationContext) ApplyIgnores(ignores []ConfigValidationIgnoreItem) {
	for _, ignore := range ignores {
		for i := 0; i < len(c.errors); i++ {
			if c.errors[i].Identifier == ignore.Identifier && ignore.Message == "" {
				c.errors = append(c.errors[:i], c.errors[i+1:]...)
				i--
				continue
			}

			if c.errors[i].Identifier == ignore.Identifier && ignore.Message != "" && strings.Contains(c.errors[i].Message, ignore.Message) {
				c.errors = append(c.errors[:i], c.errors[i+1:]...)
				i--
				continue
			}
		}
	}

	// Apply the same logic to warnings
	for _, ignore := range ignores {
		for i := 0; i < len(c.warnings); i++ {
			if c.warnings[i].Identifier == ignore.Identifier && ignore.Message == "" {
				c.warnings = append(c.warnings[:i], c.warnings[i+1:]...)
				i--
				continue
			}

			if c.warnings[i].Identifier == ignore.Identifier && ignore.Message != "" && strings.Contains(c.warnings[i].Message, ignore.Message) {
				c.warnings = append(c.warnings[:i], c.warnings[i+1:]...)
				i--
				continue
			}
		}
	}
}

func RunValidation(ctx context.Context, ext Extension) *ValidationContext {
	vc := newValidationContext(ext)

	runDefaultValidate(vc)
	ext.Validate(ctx, vc)
	validateAdministrationSnippets(vc)
	validateStorefrontSnippets(vc)
	vc.ApplyIgnores(ext.GetExtensionConfig().Validation.Ignore)

	return vc
}

func runDefaultValidate(vc *ValidationContext) {
	_, versionErr := vc.Extension.GetVersion()
	name, nameErr := vc.Extension.GetName()
	_, shopwareVersionErr := vc.Extension.GetShopwareVersionConstraint()

	if versionErr != nil {
		vc.AddError("metadata.version", versionErr.Error())
	}

	if nameErr != nil {
		vc.AddError("metadata.name", nameErr.Error())
	}

	if shopwareVersionErr != nil {
		vc.AddError("metadata.shopware_version", shopwareVersionErr.Error())
	}

	if len(name) == 0 {
		vc.AddError("metadata.name", "Extension name cannot be empty")
	}

	notAllowedErrorFormat := "file %s is not allowed in the zip file"
	_ = filepath.Walk(vc.Extension.GetPath(), func(p string, info fs.FileInfo, _ error) error {
		base := filepath.Base(p)

		if base == ".." {
			vc.AddError("zip.path_travel", "Path travel detected in zip file")
		}

		for _, file := range defaultNotAllowedPaths {
			if strings.HasPrefix(p, file) {
				vc.AddError("zip.disallowed_file", fmt.Sprintf(notAllowedErrorFormat, p))
			}
		}

		for _, file := range defaultNotAllowedFiles {
			if file == base {
				vc.AddError("zip.disallowed_file", fmt.Sprintf(notAllowedErrorFormat, p))
			}
		}

		for _, ext := range defaultNotAllowedExtensions {
			if strings.HasSuffix(base, ext) {
				vc.AddError("zip.disallowed_file", fmt.Sprintf(notAllowedErrorFormat, p))
			}
		}

		license, err := vc.Extension.GetLicense()

		if err != nil {
			vc.AddError("metadata.license", fmt.Sprintf("Could not read the license of the extension: %s", err.Error()))
		} else if strings.TrimSpace(strings.ToLower(license)) != "proprietary" {
			spdxList, err := spdx.NewSpdxLicenses()
			if err != nil {
				vc.AddWarning("metadata.license", fmt.Sprintf("Could not load the SPDX license list: %s", err.Error()))
			} else {
				valid, err := spdxList.Validate(license)
				if err != nil {
					vc.AddError("metadata.license", fmt.Sprintf("Could not validate the license: %s", err.Error()))
				} else if !valid {
					vc.AddError("metadata.license", fmt.Sprintf("The license %s is not a valid SPDX license", license))
				}
			}
		}

		return nil
	})

	metaData := vc.Extension.GetMetaData()
	if len([]rune(metaData.Label.German)) == 0 {
		vc.AddError("metadata.label", "label is not translated in german")
	}

	if len(metaData.Label.English) == 0 {
		vc.AddError("metadata.label", "label is not translated in english")
	}

	if len([]rune(metaData.Description.German)) == 0 {
		vc.AddError("metadata.description", "description is not translated in german")
	}

	if len(metaData.Description.English) == 0 {
		vc.AddError("metadata.description", "description is not translated in english")
	}

	if len([]rune(metaData.Description.German)) < 150 || len([]rune(metaData.Description.German)) > 185 {
		vc.AddError("metadata.description", fmt.Sprintf("the german description with length of %d should have a length from 150 up to 185 characters.", len([]rune(metaData.Description.German))))
	}

	if len(metaData.Description.English) < 150 || len(metaData.Description.English) > 185 {
		vc.AddError("metadata.description", fmt.Sprintf("the english description with length of %d should have a length from 150 up to 185 characters.", len(metaData.Description.English)))
	}
}
