package deployment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInitializationConfig(t *testing.T) {
	config, err := ReadInitializationConfig(strings.NewReader(`{"runtime_values":{"APP_URL":"https://shop.example.com","DATABASE_URL":"mysql://user:secret@localhost/shop"},"install_values":{"INSTALL_ADMIN_PASSWORD":"secret"}}`))
	require.NoError(t, err)
	assert.Equal(t, "https://shop.example.com", config.RuntimeValues["APP_URL"])
	assert.Equal(t, "secret", config.InstallValues["INSTALL_ADMIN_PASSWORD"])
	for _, input := range []string{"", "null", "[]", "{} {}", `{"runtime_value":{}}`, `{"runtime_values":{"DATABASE_URL":123456789}}`} {
		t.Run(input, func(t *testing.T) {
			_, err := ReadInitializationConfig(strings.NewReader(input))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "123456789")
		})
	}
}
