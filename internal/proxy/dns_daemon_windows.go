//go:build windows

package proxy

import "errors"

var errNotSupportedOnWindows = errors.New("the shared proxy DNS server is not supported on Windows")

func EnsureDNSServerRunning(baseDomain string) error {
	return errNotSupportedOnWindows
}

func DNSServerStatus() (bool, int, error) {
	return false, 0, nil
}

func StopDNSServer() error {
	return nil
}
