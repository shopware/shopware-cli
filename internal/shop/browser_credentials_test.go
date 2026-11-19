package shop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/internal/system"
)

func isolateBrowserSessions(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "AppData"} {
		t.Setenv(key, dir)
	}
	for _, key := range []string{"URL", "CLIENT_ID", "CLIENT_SECRET", "USERNAME", "PASSWORD"} {
		t.Setenv("SHOPWARE_CLI_API_"+key, "")
	}
}

func TestImplicitBrowserSession(t *testing.T) {
	isolateBrowserSessions(t)
	var challenge, redirect string
	var exchanges, refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/oauth/token":
			assert.Equal(t, "shopware-cli", r.FormValue("client_id"))
			switch r.FormValue("grant_type") {
			case "authorization_code":
				exchanges.Add(1)
				assert.Equal(t, challenge, oauth2.S256ChallengeFromVerifier(r.FormValue("code_verifier")))
				assert.Equal(t, redirect, r.FormValue("redirect_uri"))
				assert.Equal(t, "approved", r.FormValue("code"))
				_, _ = fmt.Fprint(w, `{"access_token":"first","refresh_token":"refresh","expires_in":3600}`)
			case "refresh_token":
				refreshes.Add(1)
				assert.Equal(t, "refresh", r.FormValue("refresh_token"))
				_, _ = fmt.Fprint(w, `{"access_token":"second","refresh_token":"rotated","expires_in":3600}`)
			default:
				t.Errorf("unexpected grant type %s", r.FormValue("grant_type"))
			}
		case "/api/oauth/authorize":
			w.WriteHeader(http.StatusOK)
		case "/api/_info/config":
			assert.Contains(t, []string{"Bearer first", "Bearer second"}, r.Header.Get("Authorization"))
			_, _ = fmt.Fprint(w, `{"version":"6.7.15.0"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := &Config{URL: server.URL}
	credentials, err := newShopCredentials(cfg)
	require.NoError(t, err)
	browser := credentials.(*browserCredentials)
	browser.open = func(ctx context.Context, raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.Equal(t, "/api/oauth/authorize", u.Path)
		q := u.Query()
		assert.Equal(t, "S256", q.Get("code_challenge_method"))
		assert.Equal(t, "write", q.Get("scope"))
		assert.Equal(t, "code", q.Get("response_type"))
		challenge, redirect = q.Get("code_challenge"), q.Get("redirect_uri")
		assert.NotEmpty(t, challenge)
		assert.NotEmpty(t, q.Get("state"))
		for _, state := range []string{"incorrect", q.Get("state")} {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect+"?code=approved&state="+state, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			if state == "incorrect" {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	token, err := browser.token(ctx)
	require.NoError(t, err)
	assert.Equal(t, "first", token.AccessToken)
	assert.True(t, HasAdminAPICredentials(cfg))
	info, err := os.Stat(browser.path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	headless := system.WithInteraction(ctx, false)
	_, err = NewShopClient(headless, cfg)
	require.NoError(t, err)
	token.Expiry = time.Now().Add(-time.Hour)
	_, err = browser.save(token)
	require.NoError(t, err)
	var group sync.WaitGroup
	for range 3 {
		group.Go(func() { _, err := NewShopClient(headless, cfg); assert.NoError(t, err) })
	}
	group.Wait()
	assert.EqualValues(t, 1, exchanges.Load())
	assert.EqualValues(t, 1, refreshes.Load())
	saved, err := os.ReadFile(browser.path)
	require.NoError(t, err)
	var rotated oauth2.Token
	require.NoError(t, json.Unmarshal(saved, &rotated))
	assert.Equal(t, "rotated", rotated.RefreshToken)

	cfg.Environments = map[string]*EnvironmentConfig{"staging": {URL: server.URL + "/"}}
	staging, err := cfg.WithEnvironment("staging")
	require.NoError(t, err)
	assert.True(t, HasAdminAPICredentials(staging))
	_, err = NewShopClient(headless, staging)
	require.NoError(t, err)

	staging.URL = server.URL + "/another-shop"
	assert.False(t, HasAdminAPICredentials(staging))
	_, err = NewShopClient(headless, staging)
	require.Error(t, err)
}

func TestBrowserSessionDenial(t *testing.T) {
	isolateBrowserSessions(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/authorize" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	browser, err := newBrowserCredentials(&Config{URL: server.URL})
	require.NoError(t, err)
	browser.open = func(ctx context.Context, raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		q := u.Query()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.Get("redirect_uri")+"?error=access_denied&state="+q.Get("state"), nil)
		require.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return res.Body.Close()
	}
	_, err = browser.token(t.Context())
	require.Error(t, err)
	assert.NoFileExists(t, browser.path)
}

func TestBrowserRefreshFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantLogin  bool
	}{
		{"shopware rejected token", `{"errors":[{"code":"8","detail":"expired"}]}`, 400, true},
		{"OAuth rejected token", `{"error":"invalid_grant"}`, 400, true},
		{"server down", `{"errors":[]}`, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateBrowserSessions(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, err := newBrowserCredentials(&Config{URL: server.URL})
			require.NoError(t, err)
			c.path = t.TempDir() + "/session.json"
			_, err = c.save(&oauth2.Token{AccessToken: "expired", RefreshToken: "old", Expiry: time.Now().Add(-time.Hour)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			opened := false
			c.open = func(context.Context, string) error { opened = true; cancel(); return nil }
			_, err = c.token(ctx)
			require.Error(t, err)
			assert.Equal(t, tc.wantLogin, opened)
		})
	}
}

func TestBrowserLoginUnsupportedShop(t *testing.T) {
	isolateBrowserSessions(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	browser, err := newBrowserCredentials(&Config{URL: server.URL})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	browser.open = func(context.Context, string) error {
		t.Error("browser must not open when the shop does not support OAuth login")
		cancel()
		return nil
	}
	_, err = browser.token(ctx)
	assert.ErrorIs(t, err, ErrBrowserLoginUnsupported)
	assert.NoFileExists(t, browser.path)
}

func TestBrowserExchangeUnsupportedShop(t *testing.T) {
	isolateBrowserSessions(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/authorize" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	browser, err := newBrowserCredentials(&Config{URL: server.URL})
	require.NoError(t, err)
	browser.open = func(ctx context.Context, raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		q := u.Query()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.Get("redirect_uri")+"?code=approved&state="+q.Get("state"), nil)
		require.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return res.Body.Close()
	}
	_, err = browser.token(t.Context())
	assert.ErrorIs(t, err, ErrBrowserLoginUnsupported)
	assert.NoFileExists(t, browser.path)
}

func TestBrowserRefreshUnsupportedShop(t *testing.T) {
	isolateBrowserSessions(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	browser, err := newBrowserCredentials(&Config{URL: server.URL})
	require.NoError(t, err)
	browser.path = t.TempDir() + "/session.json"
	_, err = browser.save(&oauth2.Token{AccessToken: "expired", RefreshToken: "old", Expiry: time.Now().Add(-time.Hour)})
	require.NoError(t, err)
	browser.open = func(context.Context, string) error {
		t.Error("browser must not open when the shop does not support OAuth login")
		return nil
	}
	_, err = browser.token(t.Context())
	assert.ErrorIs(t, err, ErrBrowserLoginUnsupported)
}
