package extension

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/stretchr/testify/assert"
)

type fakeShopwareCore struct{}

func (fakeShopwareCore) Package(_ context.Context, name string) (*repository.Package, error) {
	if name != "shopware/core" {
		return nil, repository.ErrPackageNotFound
	}

	return &repository.Package{Name: name, Versions: []repository.Version{
		{Name: name, Version: "v6.6.0.0", VersionNormalized: "6.6.0.0"},
		{Name: name, Version: "v6.7.0.0", VersionNormalized: "6.7.0.0"},
	}}, nil
}

func TestGetShopwareVersions(t *testing.T) {
	server := httptest.NewServer(repository.NewHandler(fakeShopwareCore{}))
	t.Cleanup(server.Close)

	original := PackagistURL
	PackagistURL = server.URL
	t.Cleanup(func() { PackagistURL = original })

	versions, err := GetShopwareVersions(t.Context())
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"6.6.0.0", "6.7.0.0"}, versions)
}
