package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shopware/shopware-cli/logging"
)

const ApiUrl = "https://api.shopware.com"

type AccountConfig interface {
	GetAccountEmail() string
	GetAccountPassword() string
}

func NewApi(ctx context.Context, config AccountConfig) (*Client, error) {
	errorFormat := "login: %v"

	request := LoginRequest{
		Email:    config.GetAccountEmail(),
		Password: config.GetAccountPassword(),
	}
	client, err := createApiFromTokenCache(ctx)

	if err == nil {
		return client, nil
	}

	s, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ApiUrl+"/accesstokens", bytes.NewBuffer(s))
	if err != nil {
		return nil, fmt.Errorf("create access token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	if resp.StatusCode != 200 {
		logging.FromContext(ctx).Debugf("Login failed with response: %s", string(data))

		var apiErr struct {
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Detail != "" {
			return nil, fmt.Errorf("login failed: %s", apiErr.Detail)
		}

		return nil, fmt.Errorf("login failed. Check your credentials")
	}

	var token token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	client = &Client{
		Token: token,
	}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot token cache: %v", err))
	}

	return client, nil
}

type token struct {
	Token         string      `json:"token"`
	Expire        tokenExpire `json:"expire"`
	UserAccountID int         `json:"userAccountId"`
	UserID        int         `json:"userId"`
	LegacyLogin   bool        `json:"legacyLogin"`
}

type tokenExpire struct {
	Date         string `json:"date"`
	TimezoneType int    `json:"timezone_type"`
	Timezone     string `json:"timezone"`
}

type LoginRequest struct {
	Email    string `json:"shopwareId"`
	Password string `json:"password"`
}

func (l LoginRequest) GetAccountEmail() string {
	return l.Email
}

func (l LoginRequest) GetAccountPassword() string {
	return l.Password
}
