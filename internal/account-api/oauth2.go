package account_api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/logging"
)

func InteractiveLogin(ctx context.Context) (*oauth2.Token, error) {
	client := &oauth2.Config{
		ClientID: OIDCClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:   fmt.Sprintf("%s/oauth2/auth", OIDCEndpoint),
			TokenURL:  fmt.Sprintf("%s/oauth2/token", OIDCEndpoint),
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	var (
		state        = generateRandomState()
		pkceVerifier = oauth2.GenerateVerifier()
		serverErr    = make(chan error)
		serverToken  = make(chan *oauth2.Token)
	)

	l, err := net.Listen("tcp", "localhost:61472")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate port for OAuth2 callback handler, try again later: %w", err)
	}

	client.RedirectURL = strings.ReplaceAll(fmt.Sprintf("http://%s/callback", l.Addr().String()), "127.0.0.1", "localhost")

	srv := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(serverErr)
			defer close(serverToken)

			ctx := r.Context()
			if err := r.ParseForm(); err != nil {
				serverErr <- fmt.Errorf("failed to parse form: %w", err)
				return
			}
			if s := r.Form.Get("state"); s != state {
				serverErr <- fmt.Errorf("state mismatch: expected %q, got %q", state, s)
				return
			}
			if r.Form.Has("error") {
				e, d := r.Form.Get("error"), r.Form.Get("error_description")
				serverErr <- fmt.Errorf("upstream error: %s: %s", e, d)
				return
			}
			code := r.Form.Get("code")
			if code == "" {
				serverErr <- fmt.Errorf("missing code")
				return
			}
			t, err := client.Exchange(
				ctx,
				code,
				oauth2.VerifierOption(pkceVerifier),
			)
			if err != nil {
				serverErr <- fmt.Errorf("failed OAuth2 token exchange: %w", err)
				return
			}
			serverToken <- t
		}),
	}
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()

	u := client.AuthCodeURL(state,
		oauth2.S256ChallengeOption(pkceVerifier),
		oauth2.SetAuthURLParam("scope", OIDCScopes),
		oauth2.SetAuthURLParam("response_type", "code"),
	)

	logging.FromContext(ctx).Infof("Please open the following URL in your browser: %s", u)

	select {
	case err := <-serverErr:
		return nil, fmt.Errorf("failed to handle OAuth2 callback: %w", err)
	case t := <-serverToken:
		return t, nil
	}
}

func generateRandomState() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
