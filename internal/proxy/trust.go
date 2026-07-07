package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/smallstep/truststore"
)

// TrustInstructions returns manual steps to trust the root CA on the current
// operating system.
func TrustInstructions(caPath string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %q", caPath)
	case "windows":
		return fmt.Sprintf("certutil -addstore -f ROOT %q", caPath)
	default:
		return fmt.Sprintf("sudo cp %q /usr/local/share/ca-certificates/mkcert-ca.crt && sudo update-ca-certificates (Debian/Ubuntu)\n"+
			"sudo cp %q /etc/pki/ca-trust/source/anchors/mkcert-ca.pem && sudo update-ca-trust (Fedora/RHEL)", caPath, caPath)
	}
}

// InstallTrust installs the mkcert root CA into the system and browser trust
// stores. When the mkcert binary is installed, "mkcert -install" is used (it
// shares the same CAROOT and additionally covers Java trust stores).
// Otherwise the CA is installed via the truststore library, the same code
// mkcert is built on. It returns a human readable summary.
func InstallTrust(ctx context.Context, caPath string) (string, error) {
	if _, err := exec.LookPath("mkcert"); err == nil {
		cmd := exec.CommandContext(ctx, "mkcert", "-install")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("mkcert -install failed: %w", err)
		}

		return "The mkcert root CA is installed, certificates issued by it are trusted.", nil
	}

	if err := truststore.InstallFile(caPath); err != nil {
		return "", fmt.Errorf("installing the CA into the system trust store failed: %w\n\nCA certificate: %s\nManual steps:\n%s", err, caPath, TrustInstructions(caPath))
	}

	messages := []string{"The CA was added to the system trust store."}

	// Firefox (and Chromium on Linux) use NSS databases instead of the system
	// store. This needs certutil (libnss3-tools / nss via brew) and is
	// best-effort.
	if err := truststore.InstallFile(caPath, truststore.WithFirefox(), truststore.WithNoSystem()); err != nil {
		messages = append(messages, fmt.Sprintf("The CA could not be added to NSS browser trust stores (Firefox/Chromium): %s", strings.TrimSpace(err.Error())))
		messages = append(messages, "Either install certutil (libnss3-tools) and re-run, or import the CA in the browser manually: "+caPath)
	} else {
		messages = append(messages, "The CA was added to the NSS trust stores used by Firefox/Chromium.")
	}

	return strings.Join(messages, "\n"), nil
}
