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
		return nil, fmt.Errorf("login failed. Check your credentials")
	}

	var token token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	memberships, err := fetchMemberships(ctx, token)
	if err != nil {
		return nil, err
	}

	var activeMemberShip Membership

	for _, membership := range memberships {
		if membership.Company.Id == token.UserID {
			activeMemberShip = membership
		}
	}

	client = &Client{
		Token:            token,
		Memberships:      memberships,
		ActiveMembership: activeMemberShip,
	}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot token cache: %v", err))
	}

	return client, nil
}

func fetchMemberships(ctx context.Context, token token) ([]Membership, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/account/%d/memberships", ApiUrl, token.UserAccountID), http.NoBody)
	r.Header.Set("x-shopware-token", token.Token)

	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetchMemberships: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(string(data)+" but got status code %d", resp.StatusCode)
	}

	var companies []Membership
	if err := json.Unmarshal(data, &companies); err != nil {
		return nil, fmt.Errorf("fetchMemberships: %v", err)
	}

	return companies, nil
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

type Membership struct {
	Id           int    `json:"id"`
	CreationDate string `json:"creationDate"`
	Active       bool   `json:"active"`
	Member       struct {
		Id           int         `json:"id"`
		Email        string      `json:"email"`
		AvatarUrl    interface{} `json:"avatarUrl"`
		PersonalData struct {
			Id         int `json:"id"`
			Salutation struct {
				Id          int    `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"salutation"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
			Locale    struct {
				Id          int    `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"locale"`
		} `json:"personalData"`
	} `json:"member"`
	Company struct {
		Id             int    `json:"id"`
		Name           string `json:"name"`
		CustomerNumber string `json:"customerNumber"`
	} `json:"company"`
	Roles []struct {
		Id           int         `json:"id"`
		Name         string      `json:"name"`
		CreationDate string      `json:"creationDate"`
		Company      interface{} `json:"company"`
		Permissions  []struct {
			Id      int    `json:"id"`
			Context string `json:"context"`
			Name    string `json:"name"`
		} `json:"permissions"`
	} `json:"roles"`
}

func (m Membership) GetRoles() []string {
	roles := make([]string, 0)

	for _, role := range m.Roles {
		roles = append(roles, role.Name)
	}

	return roles
}

type changeMembershipRequest struct {
	SelectedMembership struct {
		Id int `json:"id"`
	} `json:"membership"`
}

func (c *Client) ChangeActiveMembership(ctx context.Context, selected Membership) error {
	s, err := json.Marshal(changeMembershipRequest{SelectedMembership: struct {
		Id int `json:"id"`
	}(struct{ Id int }{Id: selected.Id})})
	if err != nil {
		return fmt.Errorf("ChangeActiveMembership: %v", err)
	}

	r, err := c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/account/%d/memberships/change", ApiUrl, c.GetUserID()), bytes.NewBuffer(s))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("ChangeActiveMembership: %v", err)
		}
	}()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == 200 {
		c.ActiveMembership = selected
		c.Token.UserID = selected.Company.Id

		if err := saveApiTokenToTokenCache(c); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("could not change active membership due http error %d", resp.StatusCode)
}
