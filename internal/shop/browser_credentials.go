package shop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/internal/system"
)

type browserCredentials struct {
	path   string
	config oauth2.Config
	open   func(context.Context, string) error
}

func newBrowserCredentials(cfg *Config) (*browserCredentials, error) {
	base := strings.TrimRight(AdminAPIURL(cfg), "/")
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("cannot start browser login: configure the canonical shop URL without credentials, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	local := net.ParseIP(host).IsLoopback() || host == "localhost" || strings.HasSuffix(host, ".localhost")
	allowLocalHTTP := u.Scheme == "http" && local
	if u.Scheme != "https" && !allowLocalHTTP {
		return nil, errors.New("cannot start browser login: HTTPS is required outside localhost")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("cannot locate browser session directory: %w", err)
	}
	key := sha256.Sum256([]byte(base))
	return &browserCredentials{
		path: filepath.Join(dir, "shopware-cli", "admin-sessions", fmt.Sprintf("%x.json", key)),
		config: oauth2.Config{
			ClientID: "shopware-cli",
			Scopes:   []string{"write"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   base + "/api/oauth/authorize",
				TokenURL:  base + "/api/oauth/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		open: system.OpenURL,
	}, nil
}

type tokenSourceFunc func() (*oauth2.Token, error)

func (fn tokenSourceFunc) Token() (*oauth2.Token, error) { return fn() }

func (c *browserCredentials) GetTokenSource(ctx context.Context, _ string) (oauth2.TokenSource, error) {
	return oauth2.ReuseTokenSource(nil, tokenSourceFunc(func() (*oauth2.Token, error) { return c.token(ctx) })), nil
}

func (c *browserCredentials) token(ctx context.Context) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return nil, fmt.Errorf("cannot create browser session directory: %w", err)
	}
	// Shopware rotates refresh tokens. Read, refresh and replace under one lock shared by CLI processes.
	lock := flock.New(c.path + ".lock")
	if _, err := lock.TryLockContext(ctx, 100*time.Millisecond); err != nil {
		return nil, fmt.Errorf("cannot lock browser session: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	var cached oauth2.Token
	data, err := os.ReadFile(c.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cached); err != nil {
			return nil, fmt.Errorf("cannot read browser session: remove %q and rerun interactively", c.path)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("cannot read browser session: %w", err)
	}
	if cached.Valid() {
		return &cached, nil
	}
	if cached.RefreshToken != "" {
		token, err := c.config.TokenSource(ctx, &cached).Token()
		if err == nil {
			return c.save(token)
		}
		if !invalidRefreshToken(err) {
			return nil, tokenRequestError("refresh browser session", err)
		}
	}
	if !system.IsInteractionEnabled(ctx) {
		return nil, errors.New("cannot authorize the Admin API: rerun interactively to sign in or configure integration credentials")
	}
	token, err := c.login(ctx)
	if err != nil {
		return nil, err
	}
	return c.save(token)
}

func invalidRefreshToken(err error) bool {
	var response *oauth2.RetrieveError
	if !errors.As(err, &response) {
		return false
	}
	if response.ErrorCode == "invalid_grant" {
		return true
	}
	// Shopware wraps League OAuth exceptions in its Admin API error envelope.
	// League uses code 8 for an expired, revoked or otherwise invalid refresh token.
	if response.Response.StatusCode != http.StatusBadRequest && response.Response.StatusCode != http.StatusUnauthorized {
		return false
	}
	var body struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(response.Body, &body) != nil {
		return false
	}
	for _, item := range body.Errors {
		if item.Code == "8" {
			return true
		}
	}
	return false
}

// OAuth error bodies can contain credentials or authorization request details.
func tokenRequestError(action string, err error) error {
	var e *oauth2.RetrieveError
	if errors.As(err, &e) {
		return fmt.Errorf("cannot %s: token endpoint returned HTTP %d", action, e.Response.StatusCode)
	}
	return fmt.Errorf("cannot %s: %w", action, err)
}

func (c *browserCredentials) save(token *oauth2.Token) (*oauth2.Token, error) {
	f, err := os.CreateTemp(filepath.Dir(c.path), ".session-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create browser session: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	err = json.NewEncoder(f).Encode(token)
	closeErr := f.Close()
	if err = errors.Join(err, closeErr); err == nil {
		err = os.Rename(f.Name(), c.path)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot save browser session: %w", err)
	}
	return token, nil
}
