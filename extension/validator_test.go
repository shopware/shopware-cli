package extension

import (
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
