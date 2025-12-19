package admin_sdk

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type OAuthCredentials interface {
	GetTokenSource(ctx context.Context, tokenURL string) (oauth2.TokenSource, error)
}

type PasswordCredentials struct {
	Username string
	Password string
	Scopes   []string
}

func NewPasswordCredentials(username, password string, scopes []string) PasswordCredentials {
	return PasswordCredentials{
		Username: username,
		Password: password,
		Scopes:   scopes,
	}
}

func (c PasswordCredentials) GetTokenSource(ctx context.Context, tokenURL string) (oauth2.TokenSource, error) {
	oauthConf := &oauth2.Config{
		ClientID: "administration",
		Scopes:   c.Scopes,
		Endpoint: oauth2.Endpoint{
			TokenURL: tokenURL,
		},
	}

	token, err := oauthConf.PasswordCredentialsToken(ctx, c.Username, c.Password)
	if err != nil {
		return nil, err
	}
	return oauth2.StaticTokenSource(token), nil
}

type IntegrationCredentials struct {
	ClientId     string
	ClientSecret string
	Scopes       []string
}

func NewIntegrationCredentials(clientId, clientSecret string, scopes []string) IntegrationCredentials {
	return IntegrationCredentials{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Scopes:       scopes,
	}
}

func (c IntegrationCredentials) GetTokenSource(ctx context.Context, tokenURL string) (oauth2.TokenSource, error) {
	oauthConf := &clientcredentials.Config{
		ClientID:     c.ClientId,
		ClientSecret: c.ClientSecret,
		Scopes:       c.Scopes,
		TokenURL:     tokenURL,
	}

	return oauthConf.TokenSource(ctx), nil
}
