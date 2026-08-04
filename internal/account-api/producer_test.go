package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	return &Client{
		Token: &oauth2.Token{AccessToken: "test-token", Expiry: time.Now().Add(time.Hour)},
	}
}

func TestProducer(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/integrations/shopwarecli/producers", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode([]Producer{{Id: 10, Name: "Acme"}})
	})

	endpoint, err := client.Producer(t.Context())
	require.NoError(t, err)
	require.Len(t, endpoint.producers, 1)
	assert.Equal(t, 10, endpoint.producers[0].Id)
	assert.Equal(t, "Acme", endpoint.producers[0].Name)
}

func TestProducerNoProducers(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]Producer{})
	})

	_, err := client.Producer(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no producer found")
}

func TestProducerEndpointExtensions(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins":
			assert.Equal(t, "1", r.URL.Query().Get("producerId"))
			assert.Equal(t, "Pay", r.URL.Query().Get("search"))
			_ = json.NewEncoder(w).Encode([]Extension{{Id: 5, Name: "PayPal"}})
		default:
			http.NotFound(w, r)
		}
	})

	endpoint := &ProducerEndpoint{
		c:         client,
		producers: []Producer{{Id: 1, Name: "Acme"}},
	}

	extensions, err := endpoint.Extensions(t.Context(), &ListExtensionCriteria{Search: "Pay"})
	require.NoError(t, err)
	require.Len(t, extensions, 1)
	assert.Equal(t, "PayPal", extensions[0].Name)
	assert.Equal(t, 1, extensions[0].Producer.Id)
	assert.Equal(t, "Acme", extensions[0].Producer.Name)
}

func TestGetExtensionByNameAndId(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/plugins":
			assert.Equal(t, "paypal", r.URL.Query().Get("search"))
			_ = json.NewEncoder(w).Encode([]Extension{{Id: 5, Name: "PayPal"}})
		case r.URL.Path == "/plugins/5":
			_ = json.NewEncoder(w).Encode(Extension{Id: 5, Name: "PayPal", Code: "pp"})
		default:
			http.NotFound(w, r)
		}
	})

	endpoint := &ProducerEndpoint{
		c:         client,
		producers: []Producer{{Id: 1, Name: "Acme"}},
	}

	ext, err := endpoint.GetExtensionByName(t.Context(), "paypal")
	require.NoError(t, err)
	assert.Equal(t, 5, ext.Id)
	assert.Equal(t, "PayPal", ext.Name)
	assert.Equal(t, "pp", ext.Code)
}

func TestGetExtensionByNameNotFound(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/plugins" {
			_ = json.NewEncoder(w).Encode([]Extension{{Id: 1, Name: "Other"}})
			return
		}
		http.NotFound(w, r)
	})

	endpoint := &ProducerEndpoint{
		c:         client,
		producers: []Producer{{Id: 1, Name: "Acme"}},
	}

	_, err := endpoint.GetExtensionByName(t.Context(), "PayPal")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find Extension by name PayPal")
}

func TestUpdateExtension(t *testing.T) {
	var gotMethod string
	var gotPath string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.UpdateExtension(t.Context(), &Extension{Id: 9, Name: "X"}))
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/plugins/9", gotPath)
}

func TestGetSoftwareVersions(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/pluginstatics/softwareVersions", r.URL.Path)
		assert.Contains(t, r.URL.RawQuery, "platform")
		_ = json.NewEncoder(w).Encode(SoftwareVersionList{
			{Id: 1, Name: "6.5.0.0", Selectable: true},
		})
	})

	endpoint := &ProducerEndpoint{c: client}
	versions, err := endpoint.GetSoftwareVersions(t.Context(), "platform")
	require.NoError(t, err)
	require.Len(t, *versions, 1)
	assert.Equal(t, "6.5.0.0", (*versions)[0].Name)
}

func TestGetExtensionGeneralInfo(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/pluginstatics/all", r.URL.Path)
		_ = json.NewEncoder(w).Encode(ExtensionGeneralInformation{
			Locales: []Locale{{Id: 1, Name: "de_DE"}},
		})
	})

	endpoint := &ProducerEndpoint{c: client}
	info, err := endpoint.GetExtensionGeneralInfo(t.Context())
	require.NoError(t, err)
	require.Len(t, info.Locales, 1)
	assert.Equal(t, "de_DE", info.Locales[0].Name)
}
