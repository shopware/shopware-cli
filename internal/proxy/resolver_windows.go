//go:build windows

package proxy

import (
	"context"
	"errors"
)

// errNotSupportedOnWindows is returned by the resolver/verify entry points
// that have no Windows implementation (the shared proxy runs under WSL, not
// native Windows).
var errNotSupportedOnWindows = errors.New("the shared proxy DNS setup is not supported on Windows")

func SupportsWildcardDNS(ctx context.Context) bool {
	return false
}

func CheckResolverConfigured(baseDomain string) ResolverStatus {
	return ResolverStatus{Configured: false, Detail: "the shared proxy DNS setup is not supported on Windows"}
}

func ConfigureResolver(ctx context.Context, baseDomain string) error {
	return errNotSupportedOnWindows
}

func UnconfigureResolver(ctx context.Context, baseDomain string) error {
	return errNotSupportedOnWindows
}

func ResolverBlockedGuidance(baseDomain string) string {
	return "the shared proxy DNS setup is not supported on Windows"
}

func ResolverManualInstructions(baseDomain string) string {
	return "the shared proxy DNS setup is not supported on Windows"
}
