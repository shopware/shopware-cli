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

// TrustInstructions returns the manual command that trusts the root CA on
// the current operating system, e.g. for handing over to an IT team.
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

// certutilInstallHint returns the package-install command for certutil on
// the current operating system, needed for Firefox's own certificate store.
func certutilInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install nss"
	default:
		return "sudo apt install libnss3-tools (Debian/Ubuntu) or sudo dnf install nss-tools (Fedora/RHEL)"
	}
}

// firefoxImportHint explains how to trust the CA in Firefox without
// administrator rights — the browser's certificate store is per-user.
func firefoxImportHint(caPath string) string {
	return "in Firefox open Settings > Privacy & Security > Certificates > View Certificates > Authorities > Import and select " + caPath
}

// TrustBlockedGuidance explains, in plain words, what to do when the trust
// store cannot be written (usually because sudo/administrator rights are
// blocked on the machine).
func TrustBlockedGuidance(caPath string) string {
	return strings.Join([]string{
		"This step needs administrator rights. You have three options:",
		"",
		"1. Run \"shopware-cli project proxy setup\" again from an account that is",
		"   allowed to use sudo/administrator commands.",
		"",
		"2. Ask your IT team to run this command for you:",
		"     " + TrustInstructions(caPath),
		"",
		"3. Continue without it: your shops still work over HTTPS, but browsers will",
		"   show a security warning that you can click through.",
		"   Tip: Firefox works without administrator rights — " + firefoxImportHint(caPath),
	}, "\n")
}

// InstallTrust installs the mkcert root CA into the system and browser trust
// stores. When the mkcert binary is installed, "mkcert -install" is tried
// first (it shares the same CAROOT and additionally covers Java trust
// stores); on failure the truststore library — the same code mkcert is built
// on — is used directly. It returns a human readable summary.
func InstallTrust(ctx context.Context, caPath string) (string, error) {
	if _, err := exec.LookPath("mkcert"); err == nil {
		cmd := exec.CommandContext(ctx, "mkcert", "-install")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err == nil {
			return "The mkcert root CA is installed, certificates issued by it are trusted.", nil
		}
		// mkcert failed (often: sudo blocked, or a broken mkcert install);
		// fall through to the library path, which explains itself on failure.
	}

	if err := truststore.InstallFile(caPath); err != nil {
		return "", fmt.Errorf("adding the certificate authority to the system trust store needs administrator rights: %w", err)
	}

	messages := []string{"The certificate authority was added to the system trust store."}

	// Firefox (and Chromium on Linux) keep their own certificate store (NSS)
	// and need certutil to update it; this is best-effort.
	if err := truststore.InstallFile(caPath, truststore.WithFirefox(), truststore.WithNoSystem()); err != nil {
		messages = append(messages,
			"Firefox (and Chromium on Linux) could not be updated because they keep their own certificate store: "+strings.TrimSpace(err.Error()),
			"To fix it, either:",
			"  - install certutil ("+certutilInstallHint()+") and run \"shopware-cli project proxy setup\" again, or",
			"  - "+firefoxImportHint(caPath),
			"Other browsers (Safari, Chrome on macOS, Edge) already work.")
	} else {
		messages = append(messages, "The certificate authority was added to the Firefox/Chromium trust stores.")
	}

	return strings.Join(messages, "\n"), nil
}
