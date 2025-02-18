package account_api

import (
	"context"
	"encoding/json"
	"fmt"
)

type MerchantEndpoint struct {
	c *Client
}

func (c *Client) Merchant() *MerchantEndpoint {
	return &MerchantEndpoint{c: c}
}

func (m MerchantEndpoint) Shops(ctx context.Context) (MerchantShopList, error) {
	r, err := m.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/shops?limit=100&userId=%d", ApiUrl, m.c.ComapnyID), nil)
	if err != nil {
		return nil, err
	}

	body, err := m.c.doRequest(r)
	if err != nil {
		return nil, err
	}

	var shops MerchantShopList
	if err := json.Unmarshal(body, &shops); err != nil {
		return nil, fmt.Errorf("shops: %v", err)
	}

	return shops, nil
}

type MerchantShopList []*MerchantShop

type MerchantShop struct {
	Id                  int         `json:"id"`
	Domain              string      `json:"domain"`
	Type                string      `json:"type"`
	CompanyId           int         `json:"companyId"`
	CompanyName         string      `json:"companyName"`
	Dispo               int         `json:"dispo"`
	Balance             float64     `json:"balance"`
	IsPartnerShop       bool        `json:"isPartnerShop"`
	Subaccount          *int        `json:"subaccount"`
	IsCommercial        bool        `json:"isCommercial"`
	DocumentComment     string      `json:"documentComment"`
	Activated           bool        `json:"activated"`
	AccountId           string      `json:"accountId"`
	ShopNumber          string      `json:"shopNumber"`
	CreationDate        string      `json:"creationDate"`
	Branch              interface{} `json:"branch"`
	SubscriptionModules []struct {
		Id     int `json:"id"`
		Module struct {
			Id                    int    `json:"id"`
			Name                  string `json:"name"`
			Description           string `json:"description"`
			Price                 int    `json:"price"`
			PriceMonthlyPayment   int    `json:"priceMonthlyPayment"`
			Price24               int    `json:"price24"`
			Price24MonthlyPayment int    `json:"price24MonthlyPayment"`
			UpgradeOrder          int    `json:"upgradeOrder"`
			DurationInMonths      int    `json:"durationInMonths"`
			BookingKey            string `json:"bookingKey"`
		} `json:"module"`
		Status struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"status"`
		ExpirationDate   string      `json:"expirationDate"`
		CreationDate     string      `json:"creationDate"`
		UpgradeDate      interface{} `json:"upgradeDate"`
		MonthlyPayment   bool        `json:"monthlyPayment"`
		DurationInMonths int         `json:"durationInMonths"`
		DurationOptions  []struct {
			Name             string `json:"name"`
			DurationInMonths int    `json:"durationInMonths"`
		} `json:"durationOptions"`
		AutomaticExtension bool `json:"automaticExtension"`
		Charging           struct {
			Id            int `json:"id"`
			ShopId        int `json:"shopId"`
			BookingShopId int `json:"bookingShopId"`
			Price         int `json:"price"`
		} `json:"charging"`
	} `json:"subscriptionModules"`
	Environment struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"environment"`
	Staging                      bool        `json:"staging"`
	Instance                     bool        `json:"instance"`
	Mandant                      bool        `json:"mandant"`
	PlatformMigrationInformation interface{} `json:"platformMigrationInformation"`
	ShopwareVersion              struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Parent      int    `json:"parent"`
		Selectable  bool   `json:"selectable"`
		Major       string `json:"major"`
		ReleaseDate string `json:"releaseDate"`
		Public      bool   `json:"public"`
	} `json:"shopwareVersion"`
	ShopwareEdition                string `json:"shopwareEdition"`
	DomainIdn                      string `json:"domain_idn"`
	LatestVerificationStatusChange struct {
		Id                   int    `json:"id"`
		ShopId               int    `json:"shopId"`
		StatusCreationDate   string `json:"statusCreationDate"`
		PreviousStatusChange struct {
			Id                           int         `json:"id"`
			ShopId                       int         `json:"shopId"`
			StatusCreationDate           string      `json:"statusCreationDate"`
			PreviousStatusChange         interface{} `json:"previousStatusChange"`
			ShopDomainVerificationStatus struct {
				Id          int    `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"shopDomainVerificationStatus"`
		} `json:"previousStatusChange"`
		ShopDomainVerificationStatus struct {
			Id          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"shopDomainVerificationStatus"`
	} `json:"latestVerificationStatusChange"`
}

func (m MerchantShopList) GetByDomain(domain string) *MerchantShop {
	for _, shop := range m {
		if shop.Domain == domain {
			return shop
		}
	}

	return nil
}

func (m MerchantEndpoint) GetComposerToken(ctx context.Context, shopId int) (string, error) {
	r, err := m.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/companies/%d/shops/%d/packagestoken", ApiUrl, m.c.ComapnyID, shopId), nil)
	if err != nil {
		return "", err
	}

	body, err := m.c.doRequest(r)
	if err != nil {
		return "", err
	}

	// We don't have generated a token
	if string(body) == "" {
		return "", nil
	}

	var token composerToken

	err = json.Unmarshal(body, &token)
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

func (m MerchantEndpoint) GenerateComposerToken(ctx context.Context, shopId int) (string, error) {
	r, err := m.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/companies/%d/shops/%d/packagestoken", ApiUrl, m.c.ComapnyID, shopId), nil)
	if err != nil {
		return "", err
	}

	body, err := m.c.doRequest(r)
	if err != nil {
		return "", err
	}

	var token composerToken

	err = json.Unmarshal(body, &token)
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

func (m MerchantEndpoint) SaveComposerToken(ctx context.Context, shopId int, token string) error {
	r, err := m.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/companies/%d/shops/%d/packagestoken/%s", ApiUrl, m.c.ComapnyID, shopId, token), nil)
	if err != nil {
		return err
	}

	_, err = m.c.doRequest(r)

	return err
}

type composerToken struct {
	Token string `json:"token"`
}
