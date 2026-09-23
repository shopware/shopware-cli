package shop

import (
	"context"
	"errors"
	"fmt"

	shopware "github.com/shopwareLabs/go-shopware-http-client"
)

// ErrNotFound is returned when a DAL search yields no matching entity.
var ErrNotFound = errors.New("not found")

// SalesChannel is a storefront sales channel with its domains.
type SalesChannel struct {
	Id      string               `json:"id"`
	Name    string               `json:"name"`
	TypeId  string               `json:"typeId"`
	Active  bool                 `json:"active"`
	Domains []SalesChannelDomain `json:"domains"`
}

// SalesChannelDomain is a domain assigned to a sales channel.
type SalesChannelDomain struct {
	Id  string `json:"id"`
	Url string `json:"url"`
}

// Theme is a storefront theme assigned to a sales channel.
type Theme struct {
	Id            string `json:"id"`
	Name          string `json:"name"`
	TechnicalName string `json:"technicalName"`
	ParentThemeId string `json:"parentThemeId"`
}

// ListStorefrontSalesChannels returns active storefront sales channels and
// their domains.
func (c *Client) ListStorefrontSalesChannels(ctx context.Context) ([]SalesChannel, error) {
	repo := shopware.NewRepository[SalesChannel](c.Client, "sales_channel")
	result, err := repo.Search(ctx, shopware.NewCriteria().
		SetLimit(100).
		AddFilter(shopware.Equals("typeId", shopware.DefaultSalesChannelTypeStorefront)).
		AddFilter(shopware.Equals("active", true)).
		AddAssociation("domains"),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot search sales channels: %w", err)
	}

	return result.Data, nil
}

// FindThemeForSalesChannel returns the theme assigned to the given sales channel.
func (c *Client) FindThemeForSalesChannel(ctx context.Context, salesChannelID string) (*Theme, error) {
	repo := shopware.NewRepository[Theme](c.Client, "theme")
	result, err := repo.Search(ctx, shopware.NewCriteria().
		SetLimit(1).
		AddFilter(shopware.Equals("salesChannels.id", salesChannelID)),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot search theme: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, ErrNotFound
	}

	return &result.Data[0], nil
}
