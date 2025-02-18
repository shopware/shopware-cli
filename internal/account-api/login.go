package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shopware/shopware-cli/logging"
	"golang.org/x/oauth2"
)

const ApiUrl = "https://api.shopware.com"

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

	memberships, err := fetchMemberships(ctx, client.Token)
	if err != nil {
		return nil, err
	}

	var activeMemberShip Membership

	// for _, membership := range memberships {
	// 	if membership.Company.Id == token.UserID {
	// 		activeMemberShip = membership
	// 	}
	// }

	client.Memberships = memberships
	client.ActiveMembership = activeMemberShip

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot token cache: %v", err))
	}

	return client, nil
}

func fetchMemberships(ctx context.Context, token *oauth2.Token) ([]Membership, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/account/%d/memberships", ApiUrl, 0), http.NoBody)
	token.SetAuthHeader(r)

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

	r, err := c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/account/%d/memberships/change", ApiUrl, c.UserID), bytes.NewBuffer(s))
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
		c.UserID = selected.Company.Id

		if err := saveApiTokenToTokenCache(c); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("could not change active membership due http error %d", resp.StatusCode)
}
