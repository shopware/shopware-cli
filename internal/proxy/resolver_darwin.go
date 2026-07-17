//go:build darwin

package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SupportsWildcardDNS reports whether this system can resolve the whole
// proxy domain via the embedded DNS server. Always true on macOS thanks to
// /etc/resolver.
func SupportsWildcardDNS(ctx context.Context) bool {
	return true
}

// resolverFilePath returns the macOS resolver configuration file for the
// proxy domain, e.g. /etc/resolver/shopware.local.
func resolverFilePath(baseDomain string) string {
	return "/etc/resolver/" + baseDomain
}

// CheckResolverConfigured reports whether the macOS resolver file for the
// proxy domain exists and points at the embedded DNS server's port.
func CheckResolverConfigured(baseDomain string) ResolverStatus {
	path := resolverFilePath(baseDomain)

	content, err := os.ReadFile(path)
	if err != nil {
		return ResolverStatus{Configured: false, Detail: path + " does not exist"}
	}

	if !strings.Contains(string(content), fmt.Sprintf("port %d", DNSPort)) {
		return ResolverStatus{Configured: false, Detail: path + " exists but points at a different DNS server"}
	}

	return ResolverStatus{Configured: true, Detail: path + " is configured"}
}

// ConfigureResolver writes the macOS resolver file for the proxy domain via
// sudo, inheriting the terminal so the password prompt is shown normally.
func ConfigureResolver(ctx context.Context, baseDomain string) error {
	content := fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", DNSPort)
	path := resolverFilePath(baseDomain)

	mkdir := exec.CommandContext(ctx, "sudo", "mkdir", "-p", "/etc/resolver")
	mkdir.Stdin = os.Stdin
	mkdir.Stdout = os.Stdout
	mkdir.Stderr = os.Stderr
	if err := mkdir.Run(); err != nil {
		return fmt.Errorf("creating /etc/resolver needs administrator rights: %w", err)
	}

	tee := exec.CommandContext(ctx, "sudo", "tee", path)
	tee.Stdin = strings.NewReader(content)
	tee.Stderr = os.Stderr
	if err := tee.Run(); err != nil {
		return fmt.Errorf("writing %s needs administrator rights: %w", path, err)
	}

	return nil
}

// ResolverBlockedGuidance explains, in plain words, what to do when the
// resolver file cannot be written (usually because sudo is blocked).
func ResolverBlockedGuidance(baseDomain string) string {
	return strings.Join([]string{
		"This step needs administrator rights (sudo). You have two options:",
		"",
		"1. Run \"shopware-cli project proxy setup\" again from an account that is",
		"   allowed to use sudo.",
		"",
		"2. Ask your IT team to create the file " + resolverFilePath(baseDomain),
		"   with exactly this content:",
		"",
		"     nameserver 127.0.0.1",
		fmt.Sprintf("     port %d", DNSPort),
		"",
		"Afterwards run \"shopware-cli project proxy verify\" to confirm everything works.",
	}, "\n")
}

// UnconfigureResolver removes the macOS resolver file for the proxy domain.
func UnconfigureResolver(ctx context.Context, baseDomain string) error {
	path := resolverFilePath(baseDomain)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	rm := exec.CommandContext(ctx, "sudo", "rm", "-f", path)
	rm.Stdin = os.Stdin
	rm.Stdout = os.Stdout
	rm.Stderr = os.Stderr
	if err := rm.Run(); err != nil {
		return fmt.Errorf("removing %s: %w", path, err)
	}

	return nil
}
