package extension

import (
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/stretchr/testify/assert"
)

func TestPackagistURL(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_PACKAGIST_URL", "")
	assert.Equal(t, repository.PackagistURL, PackagistURL())

	t.Setenv("SHOPWARE_CLI_PACKAGIST_URL", "http://127.0.0.1:9/p2")
	assert.Equal(t, "http://127.0.0.1:9/p2", PackagistURL())
}
