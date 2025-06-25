package packagist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFillAuthStruct(t *testing.T) {
	// Test with empty struct
	auth := &ComposerAuth{}
	filledAuth := fillAuthStruct(auth)

	assert.NotNil(t, filledAuth.HTTPBasicAuth)
	assert.NotNil(t, filledAuth.BearerAuth)
	assert.NotNil(t, filledAuth.GitlabAuth)
	assert.NotNil(t, filledAuth.GithubOAuth)
	assert.NotNil(t, filledAuth.BitbucketOauth)

	// Test with partially filled struct
	auth = &ComposerAuth{
		HTTPBasicAuth: map[string]ComposerAuthHttpBasic{
			"example.org": {
				Username: "user",
				Password: "pass",
			},
		},
	}
	filledAuth = fillAuthStruct(auth)

	assert.NotNil(t, filledAuth.HTTPBasicAuth)
	assert.Equal(t, "user", filledAuth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, "pass", filledAuth.HTTPBasicAuth["example.org"].Password)
	assert.NotNil(t, filledAuth.BearerAuth)
	assert.NotNil(t, filledAuth.GitlabAuth)
	assert.NotNil(t, filledAuth.GithubOAuth)
	assert.NotNil(t, filledAuth.BitbucketOauth)

	t.Run("with SHOPWARE_PACKAGES_TOKEN", func(t *testing.T) {
		t.Setenv("SHOPWARE_PACKAGES_TOKEN", "my-token")
		auth := &ComposerAuth{}
		filledAuth := fillAuthStruct(auth)

		assert.Equal(t, "my-token", filledAuth.BearerAuth["packages.shopware.com"])
	})

	t.Run("with COMPOSER_AUTH", func(t *testing.T) {
		composerAuth := `{
			"http-basic": {
				"example.com": {
					"username": "user",
					"password": "password"
				}
			},
			"bearer": {
				"example.com": "bearer-token"
			}
		}`
		t.Setenv("COMPOSER_AUTH", composerAuth)
		auth := &ComposerAuth{}
		filledAuth := fillAuthStruct(auth)

		assert.Equal(t, "user", filledAuth.HTTPBasicAuth["example.com"].Username)
		assert.Equal(t, "password", filledAuth.HTTPBasicAuth["example.com"].Password)
		assert.Equal(t, "bearer-token", filledAuth.BearerAuth["example.com"])
	})

	t.Run("with invalid COMPOSER_AUTH", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", "invalid-json")
		auth := &ComposerAuth{}
		filledAuth := fillAuthStruct(auth)

		assert.Empty(t, filledAuth.HTTPBasicAuth)
		assert.Empty(t, filledAuth.BearerAuth)
	})

	t.Run("with both env variables", func(t *testing.T) {
		t.Setenv("SHOPWARE_PACKAGES_TOKEN", "my-token")
		composerAuth := `{
			"bearer": {
				"packages.shopware.com": "override-token"
			}
		}`
		t.Setenv("COMPOSER_AUTH", composerAuth)
		auth := &ComposerAuth{}
		filledAuth := fillAuthStruct(auth)

		assert.Equal(t, "override-token", filledAuth.BearerAuth["packages.shopware.com"])
	})
}

func TestComposerAuthSave(t *testing.T) {
	tempDir := t.TempDir()
	authFile := filepath.Join(tempDir, "auth.json")

	auth := &ComposerAuth{
		path: authFile,
		HTTPBasicAuth: map[string]ComposerAuthHttpBasic{
			"example.org": {
				Username: "user",
				Password: "pass",
			},
		},
		BearerAuth: map[string]string{
			"api.example.org": "token123",
		},
	}

	err := auth.Save()
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(authFile)
	assert.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(authFile)
	assert.NoError(t, err)

	var savedAuth ComposerAuth
	err = json.Unmarshal(content, &savedAuth)
	assert.NoError(t, err)

	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Username, savedAuth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Password, savedAuth.HTTPBasicAuth["example.org"].Password)
	assert.Equal(t, auth.BearerAuth["api.example.org"], savedAuth.BearerAuth["api.example.org"])
}

func TestComposerAuth_Json(t *testing.T) {
	auth := &ComposerAuth{
		HTTPBasicAuth: map[string]ComposerAuthHttpBasic{
			"example.org": {
				Username: "user",
				Password: "pass",
			},
		},
		BearerAuth: map[string]string{
			"api.example.org": "token123",
		},
	}

	jsonBytes, err := auth.Json(true)
	assert.NoError(t, err)

	var decodedAuth ComposerAuth
	err = json.Unmarshal(jsonBytes, &decodedAuth)
	assert.NoError(t, err)

	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Username, decodedAuth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Password, decodedAuth.HTTPBasicAuth["example.org"].Password)
	assert.Equal(t, auth.BearerAuth["api.example.org"], decodedAuth.BearerAuth["api.example.org"])
}

func TestReadComposerAuth(t *testing.T) {
	// Test with existing file
	t.Run("existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "auth.json")

		testAuth := ComposerAuth{
			HTTPBasicAuth: map[string]ComposerAuthHttpBasic{
				"example.org": {
					Username: "user",
					Password: "pass",
				},
			},
			BearerAuth: map[string]string{
				"api.example.org": "token123",
			},
		}

		content, err := json.MarshalIndent(testAuth, "", "  ")
		assert.NoError(t, err)
		err = os.WriteFile(authFile, content, 0o644)
		assert.NoError(t, err)

		auth, err := ReadComposerAuth(authFile)
		assert.NoError(t, err)
		assert.Equal(t, authFile, auth.path)
		assert.Equal(t, "user", auth.HTTPBasicAuth["example.org"].Username)
		assert.Equal(t, "pass", auth.HTTPBasicAuth["example.org"].Password)
		assert.Equal(t, "token123", auth.BearerAuth["api.example.org"])
	})

	// Test with non-existing file, with fallback
	t.Run("non-existing file with fallback", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "nonexistent.json")

		auth, err := ReadComposerAuth(authFile)
		assert.NoError(t, err)
		assert.Equal(t, authFile, auth.path)
		assert.NotNil(t, auth.HTTPBasicAuth)
		assert.NotNil(t, auth.BearerAuth)
		assert.NotNil(t, auth.GitlabAuth)
		assert.NotNil(t, auth.GithubOAuth)
		assert.NotNil(t, auth.BitbucketOauth)
	})

	// Test with invalid JSON
	t.Run("invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "invalid.json")

		err := os.WriteFile(authFile, []byte("{invalid json}"), 0o644)
		assert.NoError(t, err)

		auth, err := ReadComposerAuth(authFile)
		assert.Error(t, err)
		assert.Nil(t, auth)
	})
}
