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

	var (
		state        = generateRandomState()
		pkceVerifier = oauth2.GenerateVerifier()
		serverErr    = make(chan error)
		serverToken  = make(chan *oauth2.Token)
	)

	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "localhost:61472")
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
			enterPressed = nil // prevent re-triggering
			if err := system.OpenURL(ctx, u); err != nil {
				logging.FromContext(ctx).Infof("Could not open browser automatically. Please open the URL above manually.")
			}
			fmt.Println(tui.DimText.Render("  Waiting for login to complete..."))
		case err := <-serverErr:
			return nil, fmt.Errorf("failed to handle OAuth2 callback: %w", err)
		case t := <-serverToken:
			return t, nil
		}
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
