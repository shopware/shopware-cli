package verifier

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOverrideShopwareVersionsForTestingNestedRestore asserts that the
// overrides stack: every restore func puts back the lookup installed before its
// own override, not the original one. The real lookup is never invoked, so the
// test stays offline.
func TestOverrideShopwareVersionsForTestingNestedRestore(t *testing.T) {
	original := getShopwareVersions
	t.Cleanup(func() { getShopwareVersions = original })

	fakeA := func(context.Context) ([]string, error) { return []string{"1.a"}, nil }
	fakeB := func(context.Context) ([]string, error) { return []string{"2.b"}, nil }

	restoreA := OverrideShopwareVersionsForTesting(fakeA)
	versions, err := getShopwareVersions(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"1.a"}, versions)

	restoreB := OverrideShopwareVersionsForTesting(fakeB)
	versions, err = getShopwareVersions(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"2.b"}, versions)

	restoreB()
	versions, err = getShopwareVersions(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"1.a"}, versions)

	restoreA()
	// The original lookup hits Packagist, so assert on the restored func value
	// instead of calling it.
	assert.Equal(t,
		reflect.ValueOf(original).Pointer(),
		reflect.ValueOf(getShopwareVersions).Pointer(),
	)
}
