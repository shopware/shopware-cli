package admin_sdk

import (
	"fmt"
	"net/http"
)

type CacheManagerService ClientService

func (m CacheManagerService) Clear(ctx ApiContext) (*http.Response, error) {
	r, err := m.Client.NewRequest(ctx, http.MethodDelete, "/api/_action/cache", nil)

	if err != nil {
		return nil, fmt.Errorf("cannot clear cache %w", err)
	}

	return m.Client.BareDo(ctx.Context, r)
}
