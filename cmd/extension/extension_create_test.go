package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFlagRelations(t *testing.T) {
	t.Run("non-interactive mode with missing flag", func(t *testing.T) {
		err := validateFlagRelations(map[string]bool{
			NameFlagName: false,
			TypeFlagName: true,
		}, false, false)

		require.Error(t, err)
		assert.ErrorContains(t, err, "--name")
	})

	t.Run("non-interactive mode with store enabled and vendor missing", func(t *testing.T) {
		err := validateFlagRelations(map[string]bool{
			NameFlagName:  true,
			TypeFlagName:  true,
			StoreFlagName: true,
		}, true, false)

		require.Error(t, err)
		assert.ErrorContains(t, err, "--vendor")
	})

	t.Run("non-interactive mode with all required flags", func(t *testing.T) {
		require.NoError(t, validateFlagRelations(map[string]bool{
			NameFlagName:   true,
			TypeFlagName:   true,
			StoreFlagName:  true,
			VendorFlagName: true,
		}, true, false))
	})

	t.Run("interactive mode does not require any flag", func(t *testing.T) {
		require.NoError(t, validateFlagRelations(map[string]bool{}, true, true))
	})
}
