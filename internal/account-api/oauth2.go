package account_api

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

func InteractiveLogin(ctx context.Context) (*oauth2.Token, error) {
	client := &oauth2.Config{
		ClientID: getOIDCClientID(),
		Endpoint: oauth2.Endpoint{
			AuthURL:   fmt.Sprintf("%s/oauth2/auth", getOIDCEndpoint()),
			TokenURL:  fmt.Sprintf("%s/oauth2/token", getOIDCEndpoint()),
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	state, err := generateRandomState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate OAuth2 state: %w", err)
	}

	var (
		pkceVerifier = oauth2.GenerateVerifier()
		result       = make(chan callbackResult, 1)
		once         sync.Once
	)

	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "localhost:61472")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate port for OAuth2 callback handler, try again later: %w", err)
	}

	client.RedirectURL = strings.ReplaceAll(fmt.Sprintf("http://%s/callback", l.Addr().String()), "127.0.0.1", "localhost")

	srv := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			once.Do(func() {
				defer close(result)

				ctx := r.Context()
				if err := r.ParseForm(); err != nil {
					result <- callbackResult{err: fmt.Errorf("failed to parse form: %w", err)}
					return
				}
				if s := r.Form.Get("state"); s != state {
					result <- callbackResult{err: fmt.Errorf("state mismatch: expected %q, got %q", state, s)}
					return
				}
				if r.Form.Has("error") {
					e, d := r.Form.Get("error"), r.Form.Get("error_description")
					result <- callbackResult{err: fmt.Errorf("upstream error: %s: %s", e, d)}
					return
				}
				code := r.Form.Get("code")
				if code == "" {
					result <- callbackResult{err: fmt.Errorf("missing code")}
					return
				}
				t, err := client.Exchange(
					ctx,
					code,
					oauth2.VerifierOption(pkceVerifier),
				)
				if err != nil {
					result <- callbackResult{err: fmt.Errorf("failed OAuth2 token exchange: %w", err)}
					return
				}
				result <- callbackResult{token: t}
			})

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><h1>Login successful</h1><p>You can close this window now.</p></body></html>`))
		}),
	}
	go func() { _ = srv.Serve(l) }()
	defer func() { _ = srv.Close() }()

	u := client.AuthCodeURL(state,
		oauth2.S256ChallengeOption(pkceVerifier),
		oauth2.SetAuthURLParam("scope", OIDCScopes),
		oauth2.SetAuthURLParam("response_type", "code"),
	)

	fmt.Println(tui.BoldText.Render("  Press Enter to open the login page in your browser..."))
	fmt.Println(tui.DimText.Render(fmt.Sprintf("  URL: %s", u)))
	fmt.Println()

	enterPressed := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
		close(enterPressed)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-enterPressed:
			enterPressed = nil
			if err := system.OpenURL(ctx, u); err != nil {
				logging.FromContext(ctx).Infof("Could not open browser automatically. Please open the URL above manually.")
			}
			fmt.Println(tui.DimText.Render("  Waiting for login to complete..."))
		case r := <-result:
			if r.err != nil {
				return nil, fmt.Errorf("failed to handle OAuth2 callback: %w", r.err)
			}
			return r.token, nil
		}
	}
}

type callbackResult struct {
	token *oauth2.Token
	err   error
}

func generateRandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
