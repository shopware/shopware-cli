package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	ctx := newValidationContext(app)

	runDefaultValidate(ctx)

	assert.True(t, ctx.HasErrors())
	assert.Equal(t, ctx.errors[0], "Could not validate the license: empty license string")
}

func TestLicenseValidationInvalidLicense(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.License = "FUUUU"

	ctx := newValidationContext(app)

	runDefaultValidate(ctx)

	assert.True(t, ctx.HasErrors())
	assert.Equal(t, ctx.errors[0], "Could not validate the license: invalid license factor: \"FUUUU\"")
}

func TestLicenseValidate(t *testing.T) {
	app := getAppForValidation()

	ctx := newValidationContext(app)

	runDefaultValidate(ctx)

	assert.False(t, ctx.HasErrors())
}
