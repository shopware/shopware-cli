package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidationStringListDecode(t *testing.T) {
	cfg := `
validation:
  ignore:
    - metadata.setup
    - metadata.setup.path
`

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yaml"), []byte(cfg), 0o644))

	ext, err := readExtensionConfig(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ext.Validation.Ignore))
	assert.Equal(t, "metadata.setup", ext.Validation.Ignore[0].Identifier)
	assert.Equal(t, "metadata.setup.path", ext.Validation.Ignore[1].Identifier)
}

func TestConfigValidationStringObjectDecode(t *testing.T) {
	cfg := `
validation:
  ignore:
    - identifier: metadata.setup
    - identifier: foo
      path: bar
`

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yaml"), []byte(cfg), 0o644))

	ext, err := readExtensionConfig(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ext.Validation.Ignore))
	assert.Equal(t, "metadata.setup", ext.Validation.Ignore[0].Identifier)
	assert.Equal(t, "foo", ext.Validation.Ignore[1].Identifier)
	assert.Equal(t, "bar", ext.Validation.Ignore[1].Path)
}

func TestConfigExtraBundle_ResolvePath(t *testing.T) {
	t.Run("uses Path when set", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "custom/path/to/bundle",
			Name: "MyBundle",
		}

		result := bundle.ResolvePath("/root")
		assert.Equal(t, filepath.Join("/root", "custom/path/to/bundle"), result)
	})

	t.Run("uses Name when Path is empty", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "",
			Name: "MyBundle",
		}

		result := bundle.ResolvePath("/root")
		assert.Equal(t, filepath.Join("/root", "MyBundle"), result)
	})

	t.Run("prefers Path over Name", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "src/Bundle",
			Name: "DifferentName",
		}

		result := bundle.ResolvePath("/project")
		assert.Equal(t, filepath.Join("/project", "src/Bundle"), result)
	})
}

func TestConfigExtraBundle_ResolveName(t *testing.T) {
	t.Run("uses Name when set", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "some/path/to/bundle",
			Name: "MyBundle",
		}

		result := bundle.ResolveName()
		assert.Equal(t, "MyBundle", result)
	})

	t.Run("uses base of Path when Name is empty", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "src/Bundles/CustomBundle",
			Name: "",
		}

		result := bundle.ResolveName()
		assert.Equal(t, "CustomBundle", result)
	})

	t.Run("handles simple path", func(t *testing.T) {
		bundle := ConfigExtraBundle{
			Path: "MyBundle",
			Name: "",
		}

		result := bundle.ResolveName()
		assert.Equal(t, "MyBundle", result)
	})
}

func TestConfigValidationList_Identifiers(t *testing.T) {
	t.Run("returns list of identifiers", func(t *testing.T) {
		list := ConfigValidationList{
			{Identifier: "error.one", Path: "/path/one"},
			{Identifier: "error.two", Message: "some message"},
			{Identifier: "error.three"},
		}

		identifiers := list.Identifiers()
		assert.ElementsMatch(t, []string{"error.one", "error.two", "error.three"}, identifiers)
	})

	t.Run("returns empty slice for empty list", func(t *testing.T) {
		list := ConfigValidationList{}

		identifiers := list.Identifiers()
		assert.Empty(t, identifiers)
	})
}

func TestConfigStore_IsInGermanStore(t *testing.T) {
	t.Run("returns true when availabilities is nil", func(t *testing.T) {
		store := ConfigStore{
			Availabilities: nil,
		}

		assert.True(t, store.IsInGermanStore())
	})

	t.Run("returns true when German is in availabilities", func(t *testing.T) {
		availabilities := []string{"German", "International"}
		store := ConfigStore{
			Availabilities: &availabilities,
		}

		assert.True(t, store.IsInGermanStore())
	})

	t.Run("returns false when German is not in availabilities", func(t *testing.T) {
		availabilities := []string{"International"}
		store := ConfigStore{
			Availabilities: &availabilities,
		}

		assert.False(t, store.IsInGermanStore())
	})

	t.Run("returns false for empty availabilities", func(t *testing.T) {
		availabilities := []string{}
		store := ConfigStore{
			Availabilities: &availabilities,
		}

		assert.False(t, store.IsInGermanStore())
	})
}

func TestReadExtensionConfig(t *testing.T) {
	t.Run("returns default config when no file exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		config, err := readExtensionConfig(tmpDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.True(t, config.Build.Zip.Assets.Enabled)
		assert.True(t, config.Build.Zip.Composer.Enabled)
		assert.Equal(t, ".shopware-extension.yml", config.FileName)
	})

	t.Run("reads .shopware-extension.yml", func(t *testing.T) {
		tmpDir := t.TempDir()

		configContent := `
store:
  default_locale: en_GB
build:
  shopwareVersionConstraint: "~6.5.0"
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yml"), []byte(configContent), 0644))

		config, err := readExtensionConfig(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, "~6.5.0", config.Build.ShopwareVersionConstraint)
		assert.Equal(t, ".shopware-extension.yml", config.FileName)
	})

	t.Run("prefers .yml over .yaml", func(t *testing.T) {
		tmpDir := t.TempDir()

		ymlContent := `
build:
  shopwareVersionConstraint: "from-yml"
`
		yamlContent := `
build:
  shopwareVersionConstraint: "from-yaml"
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yml"), []byte(ymlContent), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yaml"), []byte(yamlContent), 0644))

		config, err := readExtensionConfig(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, "from-yml", config.Build.ShopwareVersionConstraint)
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()

		invalidContent := `
store:
  - invalid: [structure
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yml"), []byte(invalidContent), 0644))

		_, err := readExtensionConfig(tmpDir)
		assert.Error(t, err)
	})
}

func TestValidateExtensionConfig(t *testing.T) {
	t.Run("passes for valid config", func(t *testing.T) {
		config := &Config{}
		err := validateExtensionConfig(config)
		assert.NoError(t, err)
	})

	t.Run("fails when English tags exceed 5", func(t *testing.T) {
		tags := []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6"}
		config := &Config{
			Store: ConfigStore{
				Tags: ConfigTranslated[[]string]{
					English: &tags,
				},
			},
		}
		err := validateExtensionConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tags.en")
	})

	t.Run("fails when German tags exceed 5", func(t *testing.T) {
		tags := []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6"}
		config := &Config{
			Store: ConfigStore{
				Tags: ConfigTranslated[[]string]{
					German: &tags,
				},
			},
		}
		err := validateExtensionConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tags.de")
	})

	t.Run("fails when English videos exceed 2", func(t *testing.T) {
		videos := []string{"vid1", "vid2", "vid3"}
		config := &Config{
			Store: ConfigStore{
				Videos: ConfigTranslated[[]string]{
					English: &videos,
				},
			},
		}
		err := validateExtensionConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "videos.en")
	})

	t.Run("fails when German videos exceed 2", func(t *testing.T) {
		videos := []string{"vid1", "vid2", "vid3"}
		config := &Config{
			Store: ConfigStore{
				Videos: ConfigTranslated[[]string]{
					German: &videos,
				},
			},
		}
		err := validateExtensionConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "videos.de")
	})

	t.Run("passes when tags and videos are within limits", func(t *testing.T) {
		tags := []string{"tag1", "tag2", "tag3"}
		videos := []string{"vid1", "vid2"}
		config := &Config{
			Store: ConfigStore{
				Tags: ConfigTranslated[[]string]{
					English: &tags,
					German:  &tags,
				},
				Videos: ConfigTranslated[[]string]{
					English: &videos,
					German:  &videos,
				},
			},
		}
		err := validateExtensionConfig(config)
		assert.NoError(t, err)
	})
}
