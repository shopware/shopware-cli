package proxy

import (
	"errors"
	"strings"
)

// ResolverStatus describes whether the OS is configured to resolve the proxy
// domain via the embedded DNS server.
type ResolverStatus struct {
	Configured bool
	// Detail is a short human-readable explanation of the current state.
	Detail string
}

// ErrNoSystemdResolved is returned by ConfigureResolver on Linux systems
// without systemd-resolved, where wildcard DNS routing is not available.
var ErrNoSystemdResolved = errors.New("systemd-resolved is not available")

// NoSystemdResolvedGuidance explains, in plain words, why automatic DNS
// cannot work without systemd-resolved, how to enable it, and the manual
// /etc/hosts entry as the last resort.
func NoSystemdResolvedGuidance(baseDomain string) string {
	return strings.Join([]string{
		"Automatic DNS is not possible on this system.",
		"Why: shopware-cli needs systemd-resolved to send *." + baseDomain + " lookups to its",
		"local DNS server, and this Linux system does not run it.",
		"",
		"To fix it, enable systemd-resolved (note: this changes how your whole system resolves DNS):",
		"  1. sudo apt install systemd-resolved     (Debian/Ubuntu; most other distros ship it already)",
		"  2. sudo systemctl enable --now systemd-resolved",
		"  3. sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf",
		"  4. run \"shopware-cli project proxy setup\" again",
		"",
		"If you prefer not to change your system — or it has no systemd at all —",
		"add one line per shop to /etc/hosts instead (needs sudo):",
		"  127.0.0.1 <shop-name>." + baseDomain,
		"Shops added this way work normally.",
	}, "\n")
}
