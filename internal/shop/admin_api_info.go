package shop

import (
	"context"
	"fmt"
)

// InfoResponse is the payload of GET /api/_info/config.
type InfoResponse struct {
	Version         string `json:"version"`
	VersionRevision string `json:"versionRevision"`
	AdminWorker     struct {
		EnableAdminWorker bool     `json:"enableAdminWorker"`
		Transports        []string `json:"transports"`
	} `json:"adminWorker"`
	Bundles  map[string]infoResponseBundle `json:"bundles"`
	Settings struct {
		EnableURLFeature bool `json:"enableUrlFeature"`
	} `json:"settings"`
}

type infoResponseBundle struct {
	CSS []string `json:"css"`
	Js  []string `json:"js"`
}

// IsCloudShop reports whether the shop exposes the SaaS admin bundle.
func (r InfoResponse) IsCloudShop() bool {
	_, ok := r.Bundles["SaasRufus"]
	return ok
}

// Info fetches GET /api/_info/config.
func (c *Client) Info(ctx context.Context) (*InfoResponse, error) {
	resp, err := c.Get(ctx, "/_info/config")
	if err != nil {
		return nil, fmt.Errorf("cannot get info: %w", err)
	}

	var info InfoResponse
	if err := resp.JSON(&info); err != nil {
		return nil, err
	}

	return &info, nil
}
