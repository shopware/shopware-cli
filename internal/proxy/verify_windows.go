//go:build windows

package proxy

import "context"

func resolveViaOS(ctx context.Context, hostname string) error {
	return errNotSupportedOnWindows
}
