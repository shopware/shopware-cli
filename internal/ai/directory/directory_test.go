package directory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	d, err := Load()
	require.NoError(t, err)
	require.NotNil(t, d)

	assert.Equal(t, 1, d.Version)
	assert.Len(t, d.Integrations, 3)

	// Load caches: a second call returns the same pointer.
	d2, err := Load()
	require.NoError(t, err)
	assert.Same(t, d, d2)
}

func TestGet(t *testing.T) {
	d, err := Load()
	require.NoError(t, err)

	e, ok := d.Get("shopware-cli")
	require.True(t, ok)
	assert.Equal(t, "shopware-cli", e.Name)

	_, ok = d.Get("does-not-exist")
	assert.False(t, ok)
}

// TestIntegrationJSONNeverIncludesInternal asserts maintainer metadata never
// leaks into any user-facing output: the Integration struct that `ai info`
// marshals keeps the Internal field out of JSON via json:"-".
func TestIntegrationJSONNeverIncludesInternal(t *testing.T) {
	e := Integration{
		Name:     "example",
		Internal: &Internal{Maintainer: "@shopware/team"},
	}

	b, err := json.Marshal(e)
	require.NoError(t, err)

	out := strings.ToLower(string(b))
	assert.NotContains(t, out, "maintainer")
	assert.NotContains(t, out, "internal")
}
