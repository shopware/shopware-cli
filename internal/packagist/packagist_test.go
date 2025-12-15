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
		packages, err := GetPackages(t.Context(), "test-token")

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
		packages, err := GetPackages(t.Context(), "invalid-token")

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
		packages, err := GetPackages(t.Context(), "test-token")

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
		packages, err := GetPackages(t.Context(), "test-token")

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
		packages, err := GetPackages(ctx, "test-token")

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, packages)
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
