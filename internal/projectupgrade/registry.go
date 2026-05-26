package projectupgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// Registry resolves a composer package name to its available versions.
// Implementations are expected to be safe for use from multiple goroutines.
type Registry interface {
	GetPackageVersions(ctx context.Context, name string) ([]packagist.ComposerPackageVersion, error)
}

// ErrRegistryUnavailable is returned when no backend can resolve the package
// (e.g. a store.shopware.com package when no token is configured).
var ErrRegistryUnavailable = errors.New("registry unavailable for this package")

// CombinedRegistry routes lookups to the appropriate backend based on the
// package name prefix.
type CombinedRegistry struct {
	// Store handles store.shopware.com/* packages. May be nil.
	Store Registry
	// Packagist handles every other vendor/name combination. Required.
	Packagist Registry
}

func (c *CombinedRegistry) GetPackageVersions(ctx context.Context, name string) ([]packagist.ComposerPackageVersion, error) {
	if strings.HasPrefix(name, "store.shopware.com/") {
		if c.Store == nil {
			return nil, ErrRegistryUnavailable
		}
		return c.Store.GetPackageVersions(ctx, name)
	}

	if c.Packagist == nil {
		return nil, ErrRegistryUnavailable
	}
	return c.Packagist.GetPackageVersions(ctx, name)
}

var registryHTTPClient = &http.Client{Timeout: 30 * time.Second}

// PackagistRegistry queries https://repo.packagist.org for any composer
// package's available versions. The package metadata is returned with full
// require/replace info so we can pick a Shopware-compatible release.
type PackagistRegistry struct{}

type packagistResponse struct {
	Minified string                                  `json:"minified"`
	Packages map[string][]map[string]json.RawMessage `json:"packages"`
}

func (p PackagistRegistry) GetPackageVersions(ctx context.Context, name string) ([]packagist.ComposerPackageVersion, error) {
	url := fmt.Sprintf("https://repo.packagist.org/p2/%s.json", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("packagist returned %s for %s", resp.Status, name)
	}

	var body packagistResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode packagist response: %w", err)
	}

	raw, ok := body.Packages[name]
	if !ok || len(raw) == 0 {
		return nil, nil
	}

	if body.Minified != "" {
		raw = unminify(raw)
	}

	versions := make([]packagist.ComposerPackageVersion, 0, len(raw))
	for _, m := range raw {
		payload, err := json.Marshal(m)
		if err != nil {
			continue
		}
		var v packagist.ComposerPackageVersion
		if err := json.Unmarshal(payload, &v); err != nil {
			continue
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// unminify expands the composer v2 minified packages format ("__unset"
// markers and inheritance from the previous entry) into independent records.
func unminify(versions []map[string]json.RawMessage) []map[string]json.RawMessage {
	if len(versions) == 0 {
		return nil
	}
	expanded := make([]map[string]json.RawMessage, 0, len(versions))
	var current map[string]json.RawMessage
	for _, v := range versions {
		if current == nil {
			current = cloneRaw(v)
			expanded = append(expanded, cloneRaw(current))
			continue
		}
		for k, val := range v {
			if bytes.Equal(val, []byte(`"__unset"`)) {
				delete(current, k)
			} else {
				current[k] = val
			}
		}
		expanded = append(expanded, cloneRaw(current))
	}
	return expanded
}

func cloneRaw(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ShopwareStoreRegistry queries https://packages.shopware.com/packages.json
// for store-managed plugins. The full listing is fetched once and cached for
// the lifetime of the registry instance.
type ShopwareStoreRegistry struct {
	Token string

	once     sync.Once
	loadErr  error
	packages map[string][]packagist.ComposerPackageVersion
}

type shopwareStoreResponse struct {
	Packages map[string]map[string]packagist.ComposerPackageVersion `json:"packages"`
}

func (s *ShopwareStoreRegistry) load(ctx context.Context) error {
	s.once.Do(func() {
		if s.Token == "" {
			s.loadErr = ErrRegistryUnavailable
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://packages.shopware.com/packages.json", http.NoBody)
		if err != nil {
			s.loadErr = err
			return
		}
		req.Header.Set("User-Agent", "Shopware CLI")
		req.Header.Set("Authorization", "Bearer "+s.Token)

		resp, err := registryHTTPClient.Do(req)
		if err != nil {
			s.loadErr = err
			return
		}
		defer closeBody(resp)

		if resp.StatusCode != http.StatusOK {
			s.loadErr = fmt.Errorf("shopware packages returned %s", resp.Status)
			return
		}

		var body shopwareStoreResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			s.loadErr = fmt.Errorf("decode shopware packages: %w", err)
			return
		}

		s.packages = make(map[string][]packagist.ComposerPackageVersion, len(body.Packages))
		for name, versions := range body.Packages {
			list := make([]packagist.ComposerPackageVersion, 0, len(versions))
			for _, v := range versions {
				list = append(list, v)
			}
			s.packages[name] = list
		}
	})
	return s.loadErr
}

func (s *ShopwareStoreRegistry) GetPackageVersions(ctx context.Context, name string) ([]packagist.ComposerPackageVersion, error) {
	if err := s.load(ctx); err != nil {
		return nil, err
	}

	versions, ok := s.packages[name]
	if !ok {
		return nil, nil
	}
	return versions, nil
}

// closeBody drains and closes an HTTP response body, swallowing any close
// error since callers can't act on it once they've already read the payload.
func closeBody(resp *http.Response) {
	_ = resp.Body.Close()
}

// DefaultRegistry builds a CombinedRegistry that uses packages.shopware.com
// when a store token is provided and falls back to repo.packagist.org for
// everything else. token may be empty; in that case store lookups return
// ErrRegistryUnavailable.
func DefaultRegistry(token string) Registry {
	combined := &CombinedRegistry{
		Packagist: PackagistRegistry{},
	}
	if token != "" {
		combined.Store = &ShopwareStoreRegistry{Token: token}
	}
	return combined
}
