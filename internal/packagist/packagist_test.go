package packagist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageResponseHasPackage(t *testing.T) {
	testCases := []struct {
		name           string
		packageName    string
		responseData   map[string]map[string]PackageVersion
		expectedResult bool
	}{
		{
			name:        "package exists",
			packageName: "SwagExtensionStore",
			responseData: map[string]map[string]PackageVersion{
				"store.shopware.com/swagextensionstore": {
					"1.0.0": {
						Version: "1.0.0",
					},
				},
			},
			expectedResult: true,
		},
		{
			name:        "package exists with different case",
			packageName: "SWAGEXTENSIONSTORE",
			responseData: map[string]map[string]PackageVersion{
				"store.shopware.com/swagextensionstore": {
					"1.0.0": {
						Version: "1.0.0",
					},
				},
			},
			expectedResult: true,
		},
		{
			name:        "package does not exist",
			packageName: "NonExistentPackage",
			responseData: map[string]map[string]PackageVersion{
				"store.shopware.com/swagextensionstore": {
					"1.0.0": {
						Version: "1.0.0",
					},
				},
			},
			expectedResult: false,
		},
		{
			name:           "empty response",
			packageName:    "SwagExtensionStore",
			responseData:   map[string]map[string]PackageVersion{},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := &PackageResponse{
				Packages: tc.responseData,
			}
			result := response.HasPackage(tc.packageName)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestGetPackages(t *testing.T) {
	// Save the original HTTP client to restore it after tests
	originalClient := http.DefaultClient
	defer func() {
		http.DefaultClient = originalClient
	}()

	t.Run("successful request", func(t *testing.T) {
		// Setup mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check request
			assert.Equal(t, "Shopware CLI", r.Header.Get("User-Agent"))
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

			// Return successful response
			response := PackageResponse{
				Packages: map[string]map[string]PackageVersion{
					"store.shopware.com/swagextensionstore": {
						"1.0.0": {
							Version: "1.0.0",
							Replace: map[string]string{
								"some/package": "^1.0",
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			require.NoError(t, err)
		}))
		defer server.Close()

		// Create a custom client that redirects requests to the test server
		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		// Call the function
		packages, err := GetAvailablePackagesFromShopwareStore(t.Context(), "test-token")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, packages)
		assert.True(t, packages.HasPackage("SwagExtensionStore"))
		assert.Equal(t, "1.0.0", packages.Packages["store.shopware.com/swagextensionstore"]["1.0.0"].Version)
	})

	t.Run("unauthorized request", func(t *testing.T) {
		// Setup mock server that returns 401
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		// Create a custom client that redirects requests to the test server
		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		// Call the function
		packages, err := GetAvailablePackagesFromShopwareStore(t.Context(), "invalid-token")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, packages)
		assert.Contains(t, err.Error(), "failed to get packages")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		// Setup mock server that returns invalid JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("invalid json"))
			require.NoError(t, err)
		}))
		defer server.Close()

		// Create a custom client that redirects requests to the test server
		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		// Call the function
		packages, err := GetAvailablePackagesFromShopwareStore(t.Context(), "test-token")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, packages)
	})

	t.Run("server error", func(t *testing.T) {
		// Setup mock server that returns 500
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		// Create a custom client that redirects requests to the test server
		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		// Call the function
		packages, err := GetAvailablePackagesFromShopwareStore(t.Context(), "test-token")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, packages)
		assert.Contains(t, err.Error(), "failed to get packages")
	})

	t.Run("context canceled", func(t *testing.T) {
		// Use a canceled context
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // Cancel the context immediately

		// Call the function with canceled context
		packages, err := GetAvailablePackagesFromShopwareStore(ctx, "test-token")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, packages)
	})
}

func TestGetPackageVersions(t *testing.T) {
	originalClient := http.DefaultClient
	defer func() {
		http.DefaultClient = originalClient
	}()

	t.Run("successful request with composer unminify", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/p2/shopware/core.json", r.URL.Path)
			assert.Equal(t, "Shopware CLI", r.Header.Get("User-Agent"))

			response := map[string]any{
				"minified": "composer/2.0",
				"packages": map[string]any{
					"shopware/core": []map[string]any{
						{
							"name":               "shopware/core",
							"version":            "v1.0.0",
							"version_normalized": "1.0.0.0",
							"description":        "Base description",
							"replace": map[string]string{
								"shopware/core": "*",
							},
						},
						{
							"version":            "v1.0.1",
							"version_normalized": "1.0.1.0",
						},
						{
							"version":            "v1.0.2",
							"version_normalized": "1.0.2.0",
							"description":        "__unset",
							"replace":            "__unset",
						},
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			require.NoError(t, err)
		}))
		defer server.Close()

		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		versions, err := GetShopwarePackageVersions(t.Context())

		require.NoError(t, err)
		require.Len(t, versions, 3)
		assert.Equal(t, "shopware/core", versions[0].Name)
		assert.Equal(t, "Base description", versions[1].Description)
		assert.Equal(t, map[string]string{"shopware/core": "*"}, versions[1].Replace)
		assert.Empty(t, versions[2].Description)
		assert.Nil(t, versions[2].Replace)
	})

	t.Run("successful request without minified payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"packages": map[string]any{
					"shopware/core": []map[string]any{
						{
							"name":               "shopware/core",
							"version":            "v2.0.0",
							"version_normalized": "2.0.0.0",
						},
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			require.NoError(t, err)
		}))
		defer server.Close()

		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		versions, err := GetShopwarePackageVersions(t.Context())

		require.NoError(t, err)
		require.Len(t, versions, 1)
		assert.Equal(t, "2.0.0.0", versions[0].VersionNormalized)
	})

	t.Run("package missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"packages": map[string]any{
					"some/other-package": []map[string]any{},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			require.NoError(t, err)
		}))
		defer server.Close()

		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		versions, err := GetShopwarePackageVersions(t.Context())

		assert.Error(t, err)
		assert.Nil(t, versions)
		assert.Contains(t, err.Error(), "package shopware/core not found")
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		http.DefaultClient = &http.Client{
			Transport: &mockTransport{
				server: server,
			},
		}

		versions, err := GetShopwarePackageVersions(t.Context())

		assert.Error(t, err)
		assert.Nil(t, versions)
		assert.Contains(t, err.Error(), "fetch package versions")
	})

	t.Run("context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		versions, err := GetShopwarePackageVersions(ctx)

		assert.Error(t, err)
		assert.Nil(t, versions)
	})
}

// mockTransport is a custom RoundTripper that redirects all requests to a test server.
type mockTransport struct {
	server *httptest.Server
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the request URL with the test server URL, but keep the same path
	url := m.server.URL + req.URL.Path

	// Create a new request to the test server
	newReq, err := http.NewRequestWithContext(
		req.Context(),
		req.Method,
		url,
		req.Body,
	)
	if err != nil {
		return nil, err
	}

	// Copy all headers
	newReq.Header = req.Header

	// Send request to the test server
	return m.server.Client().Transport.RoundTrip(newReq)
}
