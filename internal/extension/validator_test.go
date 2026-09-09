package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
)

func getAppForValidation() App {
	return App{
		manifest: Manifest{
			Meta: Meta{
				Name: "Test",
				Label: TranslatableString{
					struct {
						Value string "xml:\",chardata\""
						Lang  string "xml:\"lang,attr,omitempty\""
					}{"BLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAA", "de-DE"},
					struct {
						Value string "xml:\",chardata\""
						Lang  string "xml:\"lang,attr,omitempty\""
					}{"BLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAA", "en-GB"},
				},
				Description: TranslatableString{
					struct {
						Value string "xml:\",chardata\""
						Lang  string "xml:\"lang,attr,omitempty\""
					}{"BLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAA", "de-DE"},
					struct {
						Value string "xml:\",chardata\""
						Lang  string "xml:\"lang,attr,omitempty\""
					}{"BLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAABLAAAAAA", "en-GB"},
				},
				License:   "MIT",
				Author:    "Test",
				Copyright: "Test",
				Version:   "1.0.0",
			},
		},
	}
}

func TestLicenseValidationNoLicense(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.License = ""

	check := &testCheck{}

	runDefaultValidate(app, check)

	assert.True(t, len(check.Results) > 0)
	assert.Equal(t, "Could not validate the license: empty license string", check.Results[0].Message)
}

func TestLicenseValidationInvalidLicense(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.License = "FUUUU"

	check := &testCheck{}

	runDefaultValidate(app, check)

	assert.True(t, len(check.Results) > 0)
	assert.Equal(t, "Could not validate the license: invalid license factor: \"FUUUU\"", check.Results[0].Message)
}

func TestLicenseValidate(t *testing.T) {
	app := getAppForValidation()

	check := &testCheck{}

	runDefaultValidate(app, check)

	assert.False(t, len(check.Results) > 0)
}

func TestDescriptionLengthCountedInCharacters(t *testing.T) {
	app := getAppForValidation()
	multibyte := strings.Repeat("ä", 160)
	app.manifest.Meta.Description = TranslatableString{
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{multibyte, "de-DE"},
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{multibyte, "en-GB"},
	}

	check := &testCheck{}

	runDefaultValidate(app, check)

	assert.Empty(t, check.Results)
}

func TestDescriptionTooLongReportsCharacterCount(t *testing.T) {
	app := getAppForValidation()
	multibyte := strings.Repeat("ä", 200)
	app.manifest.Meta.Description = TranslatableString{
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{multibyte, "de-DE"},
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{multibyte, "en-GB"},
	}

	check := &testCheck{}

	runDefaultValidate(app, check)

	assert.Len(t, check.Results, 2)
	identifiers := make([]string, 0, len(check.Results))
	for _, result := range check.Results {
		identifiers = append(identifiers, result.Identifier)
		assert.Contains(t, result.Message, "length of 200")
	}
	assert.ElementsMatch(t, []string{
		"metadata.description.length.de-DE",
		"metadata.description.length.en-GB",
	}, identifiers)
}

func TestDescriptionMissingUsesTranslationIdentifier(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.Description = TranslatableString{
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{"", "de-DE"},
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{"", "en-GB"},
	}

	check := &testCheck{}
	runDefaultValidate(app, check)

	identifiers := make([]string, 0, len(check.Results))
	for _, result := range check.Results {
		identifiers = append(identifiers, result.Identifier)
	}
	assert.Contains(t, identifiers, "metadata.description.translation.de-DE")
	assert.Contains(t, identifiers, "metadata.description.translation.en-GB")
	assert.Contains(t, identifiers, "metadata.description.length.de-DE")
	assert.Contains(t, identifiers, "metadata.description.length.en-GB")
}

func TestDescriptionIgnoreMatchesSpecificAndPrefixIdentifiers(t *testing.T) {
	check := &testCheck{}
	check.AddResult(validation.CheckResult{Identifier: "metadata.description.length.de-DE", Message: "too long"})
	check.AddResult(validation.CheckResult{Identifier: "metadata.description.translation.en-GB", Message: "missing"})
	check.AddResult(validation.CheckResult{Identifier: "metadata.name", Message: "required"})

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.description.length"},
	})

	assert.Len(t, check.Results, 2)
	assert.Equal(t, "metadata.description.translation.en-GB", check.Results[0].Identifier)
	assert.Equal(t, "metadata.name", check.Results[1].Identifier)

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.description"},
	})

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "metadata.name", check.Results[0].Identifier)
}

func TestLabelMissingUsesTranslationIdentifier(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.Label = TranslatableString{
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{"", "de-DE"},
		struct {
			Value string "xml:\",chardata\""
			Lang  string "xml:\"lang,attr,omitempty\""
		}{"", "en-GB"},
	}

	check := &testCheck{}
	runDefaultValidate(app, check)

	identifiers := make([]string, 0, len(check.Results))
	for _, result := range check.Results {
		identifiers = append(identifiers, result.Identifier)
	}
	assert.Contains(t, identifiers, "metadata.label.translation.de-DE")
	assert.Contains(t, identifiers, "metadata.label.translation.en-GB")
}

func TestLabelIgnoreMatchesSpecificAndPrefixIdentifiers(t *testing.T) {
	check := &testCheck{}
	check.AddResult(validation.CheckResult{Identifier: "metadata.label.translation.de-DE", Message: "missing de"})
	check.AddResult(validation.CheckResult{Identifier: "metadata.label.translation.en-GB", Message: "missing en"})
	check.AddResult(validation.CheckResult{Identifier: "metadata.name", Message: "required"})

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.label.translation.de-DE"},
	})

	assert.Len(t, check.Results, 2)
	assert.Equal(t, "metadata.label.translation.en-GB", check.Results[0].Identifier)
	assert.Equal(t, "metadata.name", check.Results[1].Identifier)

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.label"},
	})

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "metadata.name", check.Results[0].Identifier)
}

func TestIgnores(t *testing.T) {
	check := &testCheck{}

	check.AddResult(validation.CheckResult{
		Identifier: "metadata.name",
		Message:    "Key `name` is required",
		Severity:   validation.SeverityError,
	})
	assert.True(t, len(check.Results) > 0)

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.name"},
	})
	assert.False(t, len(check.Results) > 0)
}

func TestValidateServicesXmlWarnsWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	plugin := getTestPlugin(tmpDir)

	configDir := filepath.Join(tmpDir, "src", "Resources", "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))
	servicesXml := filepath.Join(configDir, "services.xml")
	assert.NoError(t, os.WriteFile(servicesXml, []byte("<container/>"), 0o644))

	check := &testCheck{}
	validateSymfonyXml(plugin, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "config.services_xml.deprecated", check.Results[0].Identifier)
	assert.Equal(t, validation.SeverityWarning, check.Results[0].Severity)
	assert.Equal(t, "src/Resources/config/services.xml", check.Results[0].Path)
}

func TestValidateRoutesXmlWarnsWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	plugin := getTestPlugin(tmpDir)

	configDir := filepath.Join(tmpDir, "src", "Resources", "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))
	routesXml := filepath.Join(configDir, "routes.xml")
	assert.NoError(t, os.WriteFile(routesXml, []byte("<routes/>"), 0o644))

	check := &testCheck{}
	validateSymfonyXml(plugin, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "config.routes_xml.deprecated", check.Results[0].Identifier)
	assert.Equal(t, validation.SeverityWarning, check.Results[0].Severity)
	assert.Equal(t, "src/Resources/config/routes.xml", check.Results[0].Path)
}

func TestValidateServicesXmlSilentWhenAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	plugin := getTestPlugin(tmpDir)

	check := &testCheck{}
	validateSymfonyXml(plugin, check)

	assert.Len(t, check.Results, 0)
}

func TestValidateServicesXmlSilentWhenYaml(t *testing.T) {
	tmpDir := t.TempDir()
	plugin := getTestPlugin(tmpDir)

	configDir := filepath.Join(tmpDir, "src", "Resources", "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(configDir, "services.yaml"), []byte("services:"), 0o644))

	check := &testCheck{}
	validateSymfonyXml(plugin, check)

	assert.Len(t, check.Results, 0)
}

func TestValidateServicesXmlSkippedForApp(t *testing.T) {
	app := getAppForValidation()

	check := &testCheck{}
	validateSymfonyXml(app, check)

	assert.Len(t, check.Results, 0)
}

func TestIgnoresWithMessage(t *testing.T) {
	check := &testCheck{}

	check.AddResult(validation.CheckResult{
		Identifier: "metadata.name",
		Message:    "Key `name` is required",
		Severity:   validation.SeverityError,
	})
	assert.True(t, len(check.Results) > 0)

	check.RemoveByIdentifier([]validation.ToolConfigIgnore{
		{Identifier: "metadata.name", Message: "Key `name` is required"},
	})
	assert.False(t, len(check.Results) > 0)
}

func countResultsWithMessage(results []validation.CheckResult, message string) int {
	count := 0
	for _, r := range results {
		if r.Message == message {
			count++
		}
	}
	return count
}

func TestLicenseValidationReportedOncePerExtension(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "Sub"), 0o755))
	for _, name := range []string{"a.php", "b.php", "src/c.php", "src/Sub/d.php"} {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("<?php"), 0o644))
	}

	plugin := getTestPlugin(dir)
	plugin.Composer.License = ""

	check := &testCheck{}
	runDefaultValidate(plugin, check)

	assert.Equal(t, 1, countResultsWithMessage(check.Results, "Could not validate the license: empty license string"))
}

func TestIconValidationReportedOncePerExtension(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)

	check := &testCheck{}
	RunValidation(getTestContext(), plugin, check)

	assert.Equal(t, 1, countResultsWithMessage(check.Results, "The extension icon src/Resources/config/plugin.png does not exist"))
}
