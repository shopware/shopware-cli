package acceptance

import (
	"context"

	"github.com/shyim/go-composer/repository"
)

// shopwareCoreVersions is a Composer repository with a fixed set of shopware/core releases.
// Kept out of the test files so a coverage run instruments this package; otherwise every CLI process warns about -cover.
type shopwareCoreVersions struct{}

func (shopwareCoreVersions) Package(_ context.Context, name string) (*repository.Package, error) {
	if name != "shopware/core" {
		return nil, repository.ErrPackageNotFound
	}

	pkg := &repository.Package{Name: name}
	for _, v := range []string{"6.5.8.0", "6.6.0.0", "6.6.10.0", "6.7.0.0"} {
		pkg.Versions = append(pkg.Versions, repository.Version{Name: name, Version: "v" + v, VersionNormalized: v})
	}

	return pkg, nil
}
