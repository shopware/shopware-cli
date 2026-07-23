//go:build windows

package proxy

import "context"

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
