package account_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

const (
	OIDCEndpoint = "https://laughing-wiles-0b5m1rau2n.projects.oryapis.com"
	OIDCClientID = "fd3c9ce4-259e-4f6a-9ab0-7d8bab4de907"
)

func fetchUserInfo(ctx context.Context, token *oauth2.Token) (interface{}, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/userinfo", OIDCEndpoint), nil)
	if err != nil {
		return nil, err
	}

	token.SetAuthHeader(r)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, nil
	}

	defer resp.Body.Close()

	var userInfo interface{}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return userInfo, nil
}
