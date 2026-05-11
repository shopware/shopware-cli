package admin_sdk

import (
	"errors"
	"fmt"
	"net/http"
)

var ErrNotFound = errors.New("not found")

type SalesChannelService ClientService

const SalesChannelTypeStorefront = "8a243080f92e4c719546314b577cf82b"

type SalesChannel struct {
	Id      string               `json:"id"`
	Name    string               `json:"name"`
	TypeId  string               `json:"typeId"`
	Active  bool                 `json:"active"`
	Domains []SalesChannelDomain `json:"domains"`
}

type SalesChannelDomain struct {
	Id  string `json:"id"`
	Url string `json:"url"`
}

type Theme struct {
	Id            string `json:"id"`
	Name          string `json:"name"`
	TechnicalName string `json:"technicalName"`
	ParentThemeId string `json:"parentThemeId"`
}

type searchResponse[T any] struct {
	Data []T `json:"data"`
}

func (s SalesChannelService) ListStorefront(ctx ApiContext) ([]SalesChannel, error) {
	body := map[string]any{
		"filter": []map[string]any{
			{"type": "equals", "field": "typeId", "value": SalesChannelTypeStorefront},
			{"type": "equals", "field": "active", "value": true},
		},
		"associations": map[string]any{
			"domains": map[string]any{},
		},
		"limit": 100,
	}

	r, err := s.Client.NewRequest(ctx, http.MethodPost, "/api/search/sales-channel", body)
	if err != nil {
		return nil, fmt.Errorf("cannot search sales channels %w", err)
	}

	var out searchResponse[SalesChannel]
	resp, err := s.Client.Do(ctx.Context, r, &out)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	return out.Data, nil
}

// FindThemeForSalesChannel returns the theme assigned to the given sales channel,
// resolving the parent theme chain so the returned theme has a non-empty TechnicalName.
func (s SalesChannelService) FindThemeForSalesChannel(ctx ApiContext, salesChannelId string) (*Theme, error) {
	body := map[string]any{
		"filter": []map[string]any{
			{"type": "equals", "field": "salesChannels.id", "value": salesChannelId},
		},
		"limit": 1,
	}

	r, err := s.Client.NewRequest(ctx, http.MethodPost, "/api/search/theme", body)
	if err != nil {
		return nil, fmt.Errorf("cannot search theme %w", err)
	}

	var out searchResponse[Theme]
	resp, err := s.Client.Do(ctx.Context, r, &out)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if len(out.Data) == 0 {
		return nil, ErrNotFound
	}

	return &out.Data[0], nil
}
