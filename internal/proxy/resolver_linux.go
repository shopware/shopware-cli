//go:build linux

package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const resolvedDropInPath = "/etc/systemd/resolved.conf.d/90-shopware-cli.conf"

// SupportsWildcardDNS reports whether this system can resolve the whole
// proxy domain via the embedded DNS server, which requires systemd-resolved
// on Linux. Without it, per-project /etc/hosts entries are the fallback.
func SupportsWildcardDNS(ctx context.Context) bool {
	return hasSystemdResolved(ctx)
}

// hasSystemdResolved reports whether systemd-resolved is active on this
// system.
func hasSystemdResolved(ctx context.Context) bool {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return false
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "systemd-resolved")
	return cmd.Run() == nil
}

// CheckResolverConfigured reports whether systemd-resolved routes the proxy
// domain to the embedded DNS server.
func CheckResolverConfigured(baseDomain string) ResolverStatus {
	content, err := os.ReadFile(resolvedDropInPath)
	if err != nil {
		return ResolverStatus{Configured: false, Detail: resolvedDropInPath + " does not exist"}
	}

	if !strings.Contains(string(content), fmt.Sprintf("127.0.0.1:%d", DNSPort)) || !strings.Contains(string(content), "~"+baseDomain) {
		return ResolverStatus{Configured: false, Detail: resolvedDropInPath + " exists but does not match the expected configuration"}
	}

	return ResolverStatus{Configured: true, Detail: resolvedDropInPath + " is configured"}
}

// ConfigureResolver wires systemd-resolved split-DNS routing for the proxy
// domain to the embedded DNS server via sudo. On systems without
// systemd-resolved it returns ErrNoSystemdResolved; callers fall back to
// per-project /etc/hosts entries.
func ConfigureResolver(ctx context.Context, baseDomain string) error {
	if !hasSystemdResolved(ctx) {
		return ErrNoSystemdResolved
	}

	content := fmt.Sprintf("[Resolve]\nDNS=127.0.0.1:%d\nDomains=~%s\n", DNSPort, baseDomain)

	mkdir := exec.CommandContext(ctx, "sudo", "mkdir", "-p", "/etc/systemd/resolved.conf.d")
	mkdir.Stdin = os.Stdin
	mkdir.Stdout = os.Stdout
	mkdir.Stderr = os.Stderr
	if err := mkdir.Run(); err != nil {
		return fmt.Errorf("creating /etc/systemd/resolved.conf.d needs administrator rights: %w", err)
	}

	tee := exec.CommandContext(ctx, "sudo", "tee", resolvedDropInPath)
	tee.Stdin = strings.NewReader(content)
	tee.Stderr = os.Stderr
	if err := tee.Run(); err != nil {
		return fmt.Errorf("writing %s needs administrator rights: %w", resolvedDropInPath, err)
	}

	restart := exec.CommandContext(ctx, "sudo", "systemctl", "reload-or-restart", "systemd-resolved")
	restart.Stdin = os.Stdin
	restart.Stdout = os.Stdout
	restart.Stderr = os.Stderr
	if err := restart.Run(); err != nil {
		return fmt.Errorf("reloading systemd-resolved needs administrator rights: %w", err)
	}

	return nil
}

// ResolverBlockedGuidance explains, in plain words, what to do when the
// systemd-resolved configuration cannot be written (usually because sudo is
// blocked).
func ResolverBlockedGuidance(baseDomain string) string {
	content := fmt.Sprintf("[Resolve]\nDNS=127.0.0.1:%d\nDomains=~%s\n", DNSPort, baseDomain)

	return strings.Join([]string{
		"This step needs administrator rights (sudo). You have two options:",
		"",
		"1. Run \"shopware-cli project proxy setup\" again from an account that is",
		"   allowed to use sudo.",
		"",
		"2. Ask your IT team to create the file " + resolvedDropInPath,
		"   with exactly this content:",
		"",
		indentLines(content, "     "),
		"",
		"   and then run: systemctl reload-or-restart systemd-resolved",
		"",
		"Afterwards run \"shopware-cli project proxy verify\" to confirm everything works.",
	}, "\n")
}

// indentLines prefixes every non-empty line, for embedding file content in
// help texts.
func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}

	return strings.Join(lines, "\n")
}

// UnconfigureResolver removes the systemd-resolved drop-in for the proxy
// domain.
func UnconfigureResolver(ctx context.Context, baseDomain string) error {
	if _, err := os.Stat(resolvedDropInPath); os.IsNotExist(err) {
		return nil
	}

	rm := exec.CommandContext(ctx, "sudo", "rm", "-f", resolvedDropInPath)
	rm.Stdin = os.Stdin
	rm.Stdout = os.Stdout
	rm.Stderr = os.Stderr
	if err := rm.Run(); err != nil {
		return fmt.Errorf("removing %s: %w", resolvedDropInPath, err)
	}

	restart := exec.CommandContext(ctx, "sudo", "systemctl", "reload-or-restart", "systemd-resolved")
	restart.Stdin = os.Stdin
	restart.Stdout = os.Stdout
	restart.Stderr = os.Stderr
	if err := restart.Run(); err != nil {
		return fmt.Errorf("reloading systemd-resolved: %w", err)
	}

	return nil
}
