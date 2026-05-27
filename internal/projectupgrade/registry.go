package projectupgrade

import (
	"context"
	"errors"
	"strings"
	"sync"

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

// PackagistRegistry resolves package versions via repo.packagist.org.
type PackagistRegistry struct{}

func (PackagistRegistry) GetPackageVersions(ctx context.Context, name string) ([]packagist.ComposerPackageVersion, error) {
	return packagist.GetComposerPackageVersions(ctx, name)
}

// ShopwareStoreRegistry resolves store-managed plugins via
// packages.shopware.com. The full listing is fetched once and cached for the
// lifetime of the registry instance.
type ShopwareStoreRegistry struct {
	Token string

	once     sync.Once
	loadErr  error
	packages map[string][]packagist.ComposerPackageVersion
}

func (s *ShopwareStoreRegistry) load(ctx context.Context) error {
	s.once.Do(func() {
		if s.Token == "" {
			s.loadErr = ErrRegistryUnavailable
			return
		}

		response, err := packagist.GetAvailablePackagesFromShopwareStore(ctx, s.Token)
		if err != nil {
			s.loadErr = err
			return
		}

		s.packages = make(map[string][]packagist.ComposerPackageVersion, len(response.Packages))
		for name, versions := range response.Packages {
			list := make([]packagist.ComposerPackageVersion, 0, len(versions))
			for _, v := range versions {
				list = append(list, packagist.ComposerPackageVersion{
					Name:        name,
					Version:     v.Version,
					Description: v.Description,
					Replace:     v.Replace,
					Require:     v.Require,
				})
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

	return s.packages[name], nil
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
