package account_api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateCheckExtensionCompatibilityStatusClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      UpdateCheckExtensionCompatibilityStatus
		isBlocker   bool
		isUpdatable bool
	}{
		{
			name:   "compatible",
			status: UpdateCheckExtensionCompatibilityStatus{Name: CompatibilityCompatible, Type: "green"},
		},
		{
			name:        "updatable now is not a blocker",
			status:      UpdateCheckExtensionCompatibilityStatus{Name: CompatibilityUpdatableNow, Type: "yellow"},
			isUpdatable: true,
		},
		{
			name:        "updatable future is not a blocker",
			status:      UpdateCheckExtensionCompatibilityStatus{Name: CompatibilityUpdatableFuture, Type: "yellow"},
			isUpdatable: true,
		},
		{
			name:      "not compatible blocks",
			status:    UpdateCheckExtensionCompatibilityStatus{Name: CompatibilityNotCompatible, Type: "red"},
			isBlocker: true,
		},
		{
			name:   "not in store is informational",
			status: UpdateCheckExtensionCompatibilityStatus{Name: CompatibilityNotInStore},
		},
		{
			name:   "empty status is not a blocker",
			status: UpdateCheckExtensionCompatibilityStatus{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.isBlocker, tt.status.IsBlocker())
			assert.Equal(t, tt.isUpdatable, tt.status.IsUpdatable())
		})
	}
}
