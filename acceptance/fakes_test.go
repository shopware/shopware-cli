package acceptance

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"

	"github.com/shyim/go-composer/repository"
)

// phpVersionPath is where staticDataHost serves the Shopware-to-PHP version map.
const phpVersionPath = "/FriendsOfShopware/shopware-static-data/main/data/php-version.json"

// shopwareReleases is the fixed set of shopware/core releases both fakes serve, each with its PHP minimum.
var shopwareReleases = map[string]string{
	"6.5.8.0":  "8.1",
	"6.6.0.0":  "8.2",
	"6.6.10.0": "8.2",
	"6.7.0.0":  "8.2",
}

// fakeServices serves Packagist and the static data on one loopback server.
func fakeServices() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(phpVersionPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(shopwareReleases)
	})
	mux.Handle("/", repository.NewHandler(shopwareCoreVersions{}))

	return mux
}

// shopwareCoreVersions is a Composer repository with the releases in shopwareReleases.
type shopwareCoreVersions struct{}

func (shopwareCoreVersions) Package(_ context.Context, name string) (*repository.Package, error) {
	if name != "shopware/core" {
		return nil, repository.ErrPackageNotFound
	}

	pkg := &repository.Package{Name: name}
	for _, v := range slices.Sorted(maps.Keys(shopwareReleases)) {
		pkg.Versions = append(pkg.Versions, repository.Version{Name: name, Version: "v" + v, VersionNormalized: v})
	}

	return pkg, nil
}
