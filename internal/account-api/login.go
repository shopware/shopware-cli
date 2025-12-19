package account_api

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/logging"
)

func NewApi(ctx context.Context, token *oauth2.Token) (*Client, error) {
	errorFormat := "login: %v"

	client, _ := createApiFromTokenCache(ctx)

	if client == nil {
		client = &Client{}
	}

	if token != nil {
		client.Token = token
	}

	if !client.isTokenValid() {
		newToken, err := InteractiveLogin(ctx)
		if err != nil {
			return nil, fmt.Errorf(errorFormat, err)
		}

		client.Token = newToken
	}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot token cache: %v", err))
	}

	return client, nil
}
