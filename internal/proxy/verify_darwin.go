//go:build darwin

package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// resolveViaOS resolves hostname through macOS's system resolver stack
// (which honors /etc/resolver files, unlike Go's resolver or dig) and
// verifies it answers 127.0.0.1.
func resolveViaOS(ctx context.Context, hostname string) error {
	out, err := exec.CommandContext(ctx, "dscacheutil", "-q", "host", "-a", "name", hostname).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscacheutil failed: %w\n%s", err, out)
	}

	if !strings.Contains(string(out), "ip_address: 127.0.0.1") {
		return fmt.Errorf("%s does not resolve to 127.0.0.1 via the system resolver", hostname)
	}

	return nil
}
