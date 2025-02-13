package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/logging"
)

func generateRandomState() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login into your Shopware Account",
	Long:  "",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client := &oauth2.Config{
			ClientID: "fd3c9ce4-259e-4f6a-9ab0-7d8bab4de907",
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://laughing-wiles-0b5m1rau2n.projects.oryapis.com/oauth2/auth",
				TokenURL:  "https://laughing-wiles-0b5m1rau2n.projects.oryapis.com/oauth2/token",
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
			return fmt.Errorf("failed to allocate port for OAuth2 callback handler, try again later: %w", err)
		}
		client.RedirectURL = strings.ReplaceAll(fmt.Sprintf("http://%s/callback", l.Addr().String()), "127.0.0.1", "localhost")
		fmt.Printf("Redirect URL: %s\n", client.RedirectURL)

		srv := http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// for retries the user has to start from the beginning
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
					serverErr <- fmt.Errorf("upsteam error: %s: %s", e, d)
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
			oauth2.SetAuthURLParam("scope", "offline_access openid"),
			oauth2.SetAuthURLParam("response_type", "code"),
			oauth2.SetAuthURLParam("prompt", "login consent"),
		)

		fmt.Println("Please open the following URL in your browser:")
		fmt.Println(u)

		select {
		case err := <-serverErr:
			return fmt.Errorf("failed to handle OAuth2 callback: %w", err)
		case t := <-serverToken:
			fmt.Println("Successfully authenticated")
			fmt.Println("Access Token:", t.AccessToken)
			fmt.Println("Refresh Token:", t.RefreshToken)
			fmt.Println("Expiry:", t.Expiry)
		}

		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(loginCmd)
}

func askUserForEmailAndPassword() (string, string, error) {
	emailPrompt := promptui.Prompt{
		Label:    "Email",
		Validate: emptyValidator,
	}

	email, err := emailPrompt.Run()
	if err != nil {
		return "", "", fmt.Errorf("prompt failed %w", err)
	}

	passwordPrompt := promptui.Prompt{
		Label:    "Password",
		Validate: emptyValidator,
		Mask:     '*',
	}

	password, err := passwordPrompt.Run()
	if err != nil {
		return "", "", fmt.Errorf("prompt failed %w", err)
	}

	return email, password, nil
}

func emptyValidator(s string) error {
	if len(s) == 0 {
		return fmt.Errorf("this cannot be empty")
	}

	return nil
}

func changeAPIMembership(ctx context.Context, client *accountApi.Client, companyID int) error {
	if companyID == 0 || client.GetActiveCompanyID() == companyID {
		logging.FromContext(ctx).Debugf("Client is on correct membership skip")
		return nil
	}

	for _, membership := range client.GetMemberships() {
		if membership.Company.Id == companyID {
			logging.FromContext(ctx).Debugf("Changing member ship from %s (%d) to %s (%d)", client.ActiveMembership.Company.Name, client.ActiveMembership.Company.Id, membership.Company.Name, membership.Company.Id)
			return client.ChangeActiveMembership(ctx, membership)
		}
	}

	return fmt.Errorf("could not find configured company with id %d", companyID)
}
