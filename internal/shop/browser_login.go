package shop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

func (c *browserCredentials) login(ctx context.Context) (*oauth2.Token, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cannot listen for browser callback: %w", err)
	}

	conf := c.config
	conf.RedirectURL = "http://" + listener.Addr().String() + "/callback"
	state, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	codes := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		q := r.URL.Query()
		code, denied := q["code"], q["error"]
		hasCode := len(code) == 1 && code[0] != "" && denied == nil
		hasError := len(denied) == 1 && denied[0] != "" && code == nil
		if len(q["state"]) != 1 || q.Get("state") != state || (!hasCode && !hasError) {
			http.Error(w, "Invalid authorization callback.", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintln(w, "Return to the terminal to finish signing in.")
		select {
		case codes <- q.Get("code"):
		default:
		}
	})
	server := http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	defer func() { _ = server.Close(); _ = listener.Close() }()

	authorize := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	_, _ = fmt.Fprintf(os.Stderr, "Sign in to the shop in your browser. If it does not open, visit:\n%s\n", authorize)
	_ = c.open(ctx, authorize)

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("cannot complete browser login: %w", ctx.Err())
	case err := <-stopped:
		return nil, fmt.Errorf("cannot receive browser callback: %w", err)
	case code := <-codes:
		if code == "" {
			return nil, errors.New("cannot authorize the Admin API: browser authorization was denied or failed")
		}
		token, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, tokenRequestError("exchange browser authorization code", err)
		}
		return token, nil
	}
}
