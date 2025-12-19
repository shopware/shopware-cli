package admin_sdk

import (
	"fmt"
	"net/http"
)

type InfoService ClientService

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

func (r InfoResponse) IsCloudShop() bool {
	_, ok := r.Bundles["SaasRufus"]

	return ok
}

func (s InfoService) Info(ctx ApiContext) (*InfoResponse, *http.Response, error) {
	r, err := s.Client.NewRequest(ctx, http.MethodGet, "/api/_info/config", nil)

	if err != nil {
		return nil, nil, fmt.Errorf("cannot get info %w", err)
	}

	var info *InfoResponse
	resp, err := s.Client.Do(ctx.Context, r, &info)
	if err != nil {
		return nil, nil, err
	}

	return info, resp, err
}
