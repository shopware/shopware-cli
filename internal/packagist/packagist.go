package packagist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

type PackageResponse struct {
	Packages map[string]map[string]PackageVersion `json:"packages"`
}

func (p *PackageResponse) HasPackage(name string) bool {
	expectedName := fmt.Sprintf("store.shopware.com/%s", strings.ToLower(name))

	_, ok := p.Packages[expectedName]

	return ok
}

type PackageVersion struct {
	Version string            `json:"version"`
	Replace map[string]string `json:"replace"`
}

func GetPackages(ctx context.Context, token string) (*PackageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://packages.shopware.com/packages.json", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Shopware CLI")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get packages: %s", resp.Status)
	}

	var packages PackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, err
	}

	return &packages, nil
}
