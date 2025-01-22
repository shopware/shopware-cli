package account_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shopware/shopware-cli/logging"
)

func (c *Client) GetMyProfile(ctx context.Context) (*MyProfile, error) {
	errorFormat := "GetMyProfile: %v"

	request, err := c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/account/%d", ApiUrl, c.Token.UserAccountID), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("GetMyProfile: %v", err)
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var profile MyProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return &profile, nil
}

type MyProfile struct {
	Id           int    `json:"id"`
	Email        string `json:"email"`
	CreationDate string `json:"creationDate"`
	Banned       bool   `json:"banned"`
	Verified     bool   `json:"verified"`
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
	PartnerMarketingOptIn bool `json:"partnerMarketingOptIn"`
	SelectedMembership    struct {
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
	} `json:"selectedMembership"`
}
