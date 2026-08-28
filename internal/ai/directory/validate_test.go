package directory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validDirectory returns a minimal directory that passes Validate. Tests mutate
// a fresh copy to exercise one rejection rule each.
func validDirectory() *Directory {
	return &Directory{
		Version: 1,
		Integrations: []Integration{
			{
				Name:          "example",
				DisplayName:   "Example",
				Type:          TypeSkill,
				Provider:      "shopware",
				Description:   "An example integration.",
				Status:        StatusActive,
				Documentation: "https://example.test/docs",
				Delivery:      Delivery{Kind: DeliveryBundled},
			},
		},
	}
}

func TestValidateValid(t *testing.T) {
	require.NoError(t, validDirectory().Validate())

	// The hardwired directory must also be valid.
	require.NoError(t, Load().Validate())
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Directory)
		want   string
	}{
		{"bad version", func(d *Directory) { d.Version = 2 }, "unsupported manifest version"},
		{"empty integrations", func(d *Directory) { d.Integrations = nil }, "integrations must not be empty"},
		{"missing name", func(d *Directory) { d.Integrations[0].Name = "" }, "name is required"},
		{"bad name", func(d *Directory) { d.Integrations[0].Name = "Bad Name" }, "must match"},
		{"unknown type", func(d *Directory) { d.Integrations[0].Type = "plugin" }, "unknown type"},
		{"missing display_name", func(d *Directory) { d.Integrations[0].DisplayName = "" }, "display_name is required"},
		{"missing provider", func(d *Directory) { d.Integrations[0].Provider = "" }, "provider is required"},
		{"missing description", func(d *Directory) { d.Integrations[0].Description = "" }, "description is required"},
		{"unknown status", func(d *Directory) { d.Integrations[0].Status = "beta" }, "unknown status"},
		{"missing documentation", func(d *Directory) { d.Integrations[0].Documentation = "" }, "documentation is required"},
		{"bad documentation url", func(d *Directory) { d.Integrations[0].Documentation = "not-a-url" }, "absolute http(s) URL"},
		{"unknown delivery kind", func(d *Directory) { d.Integrations[0].Delivery.Kind = "svn" }, "unknown delivery.kind"},
		{"git without repository", func(d *Directory) { d.Integrations[0].Delivery = Delivery{Kind: DeliveryGit} }, "repository is required"},
		{"git bad repository url", func(d *Directory) {
			d.Integrations[0].Delivery = Delivery{Kind: DeliveryGit, Repository: "nope"}
		}, "absolute http(s) URL"},
		{"repository on non-git", func(d *Directory) {
			d.Integrations[0].Delivery.Repository = "https://x.test/r"
		}, "only allowed when delivery.kind"},
		{"unknown compatibility source", func(d *Directory) {
			d.Integrations[0].Compatibility = &Compatibility{Source: "vendor"}
		}, "unknown compatibility.source"},
		{"empty maintainer", func(d *Directory) {
			d.Integrations[0].Internal = &Internal{Maintainer: ""}
		}, "internal.maintainer must not be empty"},
		{"duplicate name", func(d *Directory) {
			d.Integrations = append(d.Integrations, d.Integrations[0])
		}, "duplicate name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDirectory()
			tc.mutate(d)

			err := d.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
