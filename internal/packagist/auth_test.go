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
				"packages.shopware.com": "composer-token"
			}
		}`
		t.Setenv("COMPOSER_AUTH", composerAuth)
		auth := &ComposerAuth{}
		filledAuth := fillAuthStruct(auth)

		assert.Equal(t, "my-token", filledAuth.BearerAuth["packages.shopware.com"])
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

func TestGitlabTokenUnmarshalling(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": "my-token"}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabAuth["gitlab.com"].Username)
	})

	t.Run("unmarshal object", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": {"username": "my-user", "token": "my-token"}}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Equal(t, "my-user", auth.GitlabAuth["gitlab.com"].Username)
	})

	t.Run("unmarshal mixed", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": "my-token", "example.com": {"username": "my-user", "token": "my-token2"}}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabAuth["gitlab.com"].Username)
		assert.Equal(t, "my-token2", auth.GitlabAuth["example.com"].Token)
		assert.Equal(t, "my-user", auth.GitlabAuth["example.com"].Username)
	})

	t.Run("marshal string", func(t *testing.T) {
		auth := ComposerAuth{
			GitlabAuth: map[string]GitlabToken{
				"gitlab.com": {Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-token": {"gitlab.com": "my-token"}}`, string(jsonData))
	})

	t.Run("marshal object", func(t *testing.T) {
		auth := ComposerAuth{
			GitlabAuth: map[string]GitlabToken{
				"gitlab.com": {Username: "my-user", Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-token": {"gitlab.com": {"username": "my-user", "token": "my-token"}}}`, string(jsonData))
	})
}

func TestGitlabOAuthTokenUnmarshalling(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		jsonData := `{"gitlab-oauth": {"gitlab.com": "my-token"}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabOAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabOAuth["gitlab.com"].RefreshToken)
		assert.Zero(t, auth.GitlabOAuth["gitlab.com"].ExpiresAt)
	})

	t.Run("unmarshal object", func(t *testing.T) {
		jsonData := `{"gitlab-oauth": {"gitlab.com": {"token": "my-token", "refresh-token": "my-refresh", "expires-at": 123}}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabOAuth["gitlab.com"].Token)
		assert.Equal(t, "my-refresh", auth.GitlabOAuth["gitlab.com"].RefreshToken)
		assert.Equal(t, int64(123), auth.GitlabOAuth["gitlab.com"].ExpiresAt)
	})

	t.Run("marshal string", func(t *testing.T) {
		auth := ComposerAuth{
			GitlabOAuth: map[string]GitlabOAuthToken{
				"gitlab.com": {Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-oauth": {"gitlab.com": "my-token"}}`, string(jsonData))
	})

	t.Run("marshal object", func(t *testing.T) {
		auth := ComposerAuth{
			GitlabOAuth: map[string]GitlabOAuthToken{
				"gitlab.com": {Token: "my-token", RefreshToken: "my-refresh", ExpiresAt: 123},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-oauth": {"gitlab.com": {"token": "my-token", "refresh-token": "my-refresh", "expires-at": 123}}}`, string(jsonData))
	})
}

func TestCustomHeadersUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"custom-headers": {"example.com": ["Header-Name: Header-Value"]}}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Header-Name: Header-Value"}, auth.CustomHeaders["example.com"])
	})

	t.Run("marshal", func(t *testing.T) {
		auth := ComposerAuth{
			CustomHeaders: map[string][]string{
				"example.com": {"Header-Name: Header-Value"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"custom-headers": {"example.com": ["Header-Name: Header-Value"]}}`, string(jsonData))
	})
}

func TestGitlabDomainsUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"gitlab-domains": ["gitlab.com", "example.com"]}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"gitlab.com", "example.com"}, auth.GitlabDomains)
	})

	t.Run("marshal", func(t *testing.T) {
		auth := ComposerAuth{
			GitlabDomains: []string{"gitlab.com", "example.com"},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-domains": ["gitlab.com", "example.com"]}`, string(jsonData))
	})
}

func TestGithubDomainsUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"github-domains": ["github.com", "example.com"]}`
		var auth ComposerAuth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"github.com", "example.com"}, auth.GithubDomains)
	})

	t.Run("marshal", func(t *testing.T) {
		auth := ComposerAuth{
			GithubDomains: []string{"github.com", "example.com"},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"github-domains": ["github.com", "example.com"]}`, string(jsonData))
	})
}
