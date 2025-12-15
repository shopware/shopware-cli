package packagist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposerJsonRepositoriesHasRepository(t *testing.T) {
	repos := ComposerJsonRepositories{
		{
			Type: "vcs",
			URL:  "https://github.com/shopware/platform",
		},
		{
			Type: "composer",
			URL:  "https://packages.example.org",
		},
	}

	assert.True(t, repos.HasRepository("https://github.com/shopware/platform"))
	assert.True(t, repos.HasRepository("https://packages.example.org"))
	assert.False(t, repos.HasRepository("https://github.com/shopware/core"))
	assert.False(t, repos.HasRepository(""))
}

func TestComposerJsonHasPackage(t *testing.T) {
	composer := &ComposerJson{
		Require: ComposerPackageLink{
			"symfony/console": "^5.0",
			"php":             "^7.4 || ^8.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
		},
	}

	assert.True(t, composer.HasPackage("symfony/console"))
	assert.True(t, composer.HasPackage("php"))
	assert.False(t, composer.HasPackage("phpunit/phpunit"))
	assert.False(t, composer.HasPackage("not-exists"))
}

func TestComposerJsonHasPackageDev(t *testing.T) {
	composer := &ComposerJson{
		Require: ComposerPackageLink{
			"symfony/console": "^5.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
			"mockery/mockery": "^1.4",
		},
	}

	assert.True(t, composer.HasPackageDev("phpunit/phpunit"))
	assert.True(t, composer.HasPackageDev("mockery/mockery"))
	assert.False(t, composer.HasPackageDev("symfony/console"))
	assert.False(t, composer.HasPackageDev("not-exists"))
}

func TestComposerJsonSave(t *testing.T) {
	tempDir := t.TempDir()
	composerFile := filepath.Join(tempDir, "composer.json")

	composer := &ComposerJson{
		path:        composerFile,
		Name:        "shopware/cli",
		Description: "Shopware CLI tool",
		Version:     "1.0.0",
		Type:        "library",
		License:     "MIT",
		Authors: []ComposerJsonAuthor{
			{
				Name:  "Shopware AG",
				Email: "info@shopware.com",
			},
		},
		Require: ComposerPackageLink{
			"php":             "^7.4 || ^8.0",
			"symfony/console": "^5.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
		},
		Repositories: ComposerJsonRepositories{
			{
				Type: "vcs",
				URL:  "https://github.com/shopware/platform",
			},
		},
	}

	err := composer.Save()
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(composerFile)
	assert.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(composerFile)
	assert.NoError(t, err)

	var savedComposer ComposerJson
	err = json.Unmarshal(content, &savedComposer)
	assert.NoError(t, err)

	assert.Equal(t, composer.Name, savedComposer.Name)
	assert.Equal(t, composer.Description, savedComposer.Description)
	assert.Equal(t, composer.Version, savedComposer.Version)
	assert.Equal(t, composer.Type, savedComposer.Type)
	assert.Equal(t, composer.License, savedComposer.License)
	assert.Equal(t, composer.Authors[0].Name, savedComposer.Authors[0].Name)
	assert.Equal(t, composer.Authors[0].Email, savedComposer.Authors[0].Email)
	assert.Equal(t, composer.Require["php"], savedComposer.Require["php"])
	assert.Equal(t, composer.Require["symfony/console"], savedComposer.Require["symfony/console"])
	assert.Equal(t, composer.RequireDev["phpunit/phpunit"], savedComposer.RequireDev["phpunit/phpunit"])
	assert.Equal(t, composer.Repositories[0].Type, savedComposer.Repositories[0].Type)
	assert.Equal(t, composer.Repositories[0].URL, savedComposer.Repositories[0].URL)
}

func TestReadComposerJson(t *testing.T) {
	// Test with valid file
	t.Run("valid file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		testComposer := ComposerJson{
			Name:        "shopware/cli",
			Description: "Shopware CLI tool",
			Version:     "1.0.0",
			Require: ComposerPackageLink{
				"php": "^7.4 || ^8.0",
			},
			Repositories: ComposerJsonRepositories{
				{
					Type: "vcs",
					URL:  "https://github.com/shopware/platform",
				},
			},
		}

		content, err := json.MarshalIndent(testComposer, "", "  ")
		assert.NoError(t, err)
		err = os.WriteFile(composerFile, content, 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)
		assert.Equal(t, "shopware/cli", composer.Name)
		assert.Equal(t, "Shopware CLI tool", composer.Description)
		assert.Equal(t, "1.0.0", composer.Version)
		assert.Equal(t, "^7.4 || ^8.0", composer.Require["php"])
		assert.Equal(t, "vcs", composer.Repositories[0].Type)
		assert.Equal(t, "https://github.com/shopware/platform", composer.Repositories[0].URL)
	})

	// Test with non-existing file
	t.Run("non-existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "nonexistent.json")

		composer, err := ReadComposerJson(composerFile)
		assert.Error(t, err)
		assert.Nil(t, composer)
	})

	// Test with invalid JSON
	t.Run("invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "invalid.json")

		err := os.WriteFile(composerFile, []byte("{invalid json}"), 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.Error(t, err)
		assert.Nil(t, composer)
	})
}

func TestReadComposerJsonDifferentRepositoryWritings(t *testing.T) {
	t.Run("repository list", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		content := `
{
	"repositories": [
		{
			"type": "vcs",
			"url": "https://github.com/shopware/platform"
		},
		{
			"type": "path",
			"url": "custom/plugins"
		}
	]
}
`
		err := os.WriteFile(composerFile, []byte(content), 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)

		expectedRepos := []ComposerJsonRepository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		assert.ElementsMatch(t, expectedRepos, composer.Repositories)
	})

	t.Run("repository map", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		content := `
{
	"repositories": {
		"remote": {
			"type": "vcs",
			"url": "https://github.com/shopware/platform"
		},
		"local": {
			"type": "path",
			"url": "custom/plugins"
		}
	}
}
`
		err := os.WriteFile(composerFile, []byte(content), 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)

		expectedRepos := []ComposerJsonRepository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		assert.ElementsMatch(t, expectedRepos, composer.Repositories)
	})
}
