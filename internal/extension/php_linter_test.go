package extension

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func setupMockPHPVersionServer(t *testing.T) {
	t.Helper()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"6.4.0.0": "7.4", "6.5.0.0": "8.1", "6.6.0.0": "8.2", "invalid": "9.9"}`))
	}))
	t.Cleanup(server.Close)

	original := phpVersionURL
	phpVersionURL = server.URL
	t.Cleanup(func() {
		phpVersionURL = original
	})
}

func TestNormalizePhpVersion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"8.4", "8.4"},
		{"8.4.1", "8.4"},
		{" 8.2 ", "8.2"},
		{"8", "8"},
		{"7.4.33-1", "7.4"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizePhpVersion(tc.input))
		})
	}
}

func TestGetPhpVersion(t *testing.T) {
	setupMockPHPVersionServer(t)

	for _, tc := range []struct {
		constraint string
		expected   string
	}{
		{">=6.4", "7.4"},
		{">=6.5", "8.1"},
		{">=6.6", "8.2"},
		{">=6.7", ""},
	} {
		t.Run(tc.constraint, func(t *testing.T) {
			constraint, err := version.NewConstraint(tc.constraint)
			require.NoError(t, err)

			for range 20 {
				phpVersion, err := GetPhpVersion(t.Context(), &constraint)
				if tc.expected == "" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, tc.expected, phpVersion)
			}
		})
	}
}

func TestRunValidationPHPVersion(t *testing.T) {
	setupMockPHPVersionServer(t)

	for _, tc := range []struct {
		name             string
		extensionVersion string
		projectVersion   string
		wantSyntaxError  bool
	}{
		{name: "Shopware minimum", wantSyntaxError: true},
		{name: "project target", projectVersion: "8.2"},
		{name: "extension target", extensionVersion: "8.2"},
		{name: "lower extension override", extensionVersion: "7.4", projectVersion: "8.2", wantSyntaxError: true},
		{name: "higher extension override", extensionVersion: "8.2", projectVersion: "7.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plugin := getTestPlugin(dir)
			plugin.config.Validation.PhpVersion = tc.extensionVersion
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Status.php"), []byte("<?php\nenum Status { case Active; }\n"), 0o644))

			check := &testCheck{}
			RunValidation(t.Context(), plugin, check, tc.projectVersion)

			var lintResults []validation.CheckResult
			for _, result := range check.Results {
				if result.Identifier == "php.linter" {
					lintResults = append(lintResults, result)
					assert.Equal(t, validation.SeverityError, result.Severity)
					assert.Equal(t, "src/Status.php", result.Path)
				}
			}
			assert.Equal(t, tc.wantSyntaxError, len(lintResults) > 0)
			assert.Equal(t, tc.extensionVersion, plugin.config.Validation.PhpVersion)
		})
	}
}

func TestValidatePHPFilesSkipsAppsAndBundles(t *testing.T) {
	for _, ext := range []Extension{App{}, ShopwareBundle{}} {
		t.Run(ext.GetType(), func(t *testing.T) {
			check := &testCheck{}
			validatePHPFiles(t.Context(), ext, check, "8.2")
			assert.Empty(t, check.Results)
		})
	}
}
