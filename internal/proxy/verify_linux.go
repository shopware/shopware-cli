//go:build linux

package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// resolveViaOS resolves hostname through the system's NSS stack (which
// honors systemd-resolved split-DNS and /etc/hosts, unlike Go's resolver)
// and verifies it answers 127.0.0.1.
func resolveViaOS(ctx context.Context, hostname string) error {
	out, err := exec.CommandContext(ctx, "getent", "hosts", hostname).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s does not resolve via the system resolver (getent hosts): %w", hostname, err)
	}

	if !strings.HasPrefix(strings.TrimSpace(string(out)), "127.0.0.1") {
		return fmt.Errorf("%s resolves to %q instead of 127.0.0.1", hostname, strings.TrimSpace(string(out)))
	}

	return nil
}
