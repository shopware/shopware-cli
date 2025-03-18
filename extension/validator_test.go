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
	assert.Equal(t, "Could not validate the license: empty license string", ctx.errors[0].Message)
}

func TestLicenseValidationInvalidLicense(t *testing.T) {
	app := getAppForValidation()
	app.manifest.Meta.License = "FUUUU"

	ctx := newValidationContext(app)

	runDefaultValidate(ctx)

	assert.True(t, ctx.HasErrors())
	assert.Equal(t, "Could not validate the license: invalid license factor: \"FUUUU\"", ctx.errors[0].Message)
}

func TestLicenseValidate(t *testing.T) {
	app := getAppForValidation()

	ctx := newValidationContext(app)

	runDefaultValidate(ctx)

	assert.False(t, ctx.HasErrors())
}

func TestIgnores(t *testing.T) {
	app := getAppForValidation()

	ctx := newValidationContext(app)

	ctx.AddError("metadata.name", "Key `name` is required")
	assert.True(t, ctx.HasErrors())

	ctx.ApplyIgnores([]ConfigValidationIgnoreItem{
		{Identifier: "metadata.name"},
	})
	assert.False(t, ctx.HasErrors())
}

func TestIgnoresWithMessage(t *testing.T) {
	app := getAppForValidation()

	ctx := newValidationContext(app)

	ctx.AddError("metadata.name", "Key `name` is required")
	assert.True(t, ctx.HasErrors())

	ctx.ApplyIgnores([]ConfigValidationIgnoreItem{
		{Identifier: "metadata.name", Message: "Key `name` is required"},
	})
	assert.False(t, ctx.HasErrors())
}
