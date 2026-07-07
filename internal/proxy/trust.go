package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// TrustInstructions returns manual steps to trust the local CA on the current
// operating system.
func TrustInstructions(caPath string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %q", caPath)
	case "windows":
		return fmt.Sprintf("certutil -addstore -f ROOT %q", caPath)
	default:
		return fmt.Sprintf("sudo cp %q /usr/local/share/ca-certificates/shopware-cli-ca.crt && sudo update-ca-certificates (Debian/Ubuntu)\n"+
			"sudo cp %q /etc/pki/ca-trust/source/anchors/shopware-cli-ca.pem && sudo update-ca-trust (Fedora/RHEL)", caPath, caPath)
	}
}

// InstallTrust tries to install the local CA into the system (and, where
// possible, browser) trust stores. It returns a human readable summary of
// what happened.
func InstallTrust(ctx context.Context, caPath string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return installTrustDarwin(ctx, caPath)
	case "windows":
		return installTrustWindows(ctx, caPath)
	case "linux":
		return installTrustLinux(ctx, caPath)
	default:
		return "", fmt.Errorf("automatic trust installation is not supported on %s, install manually:\n%s", runtime.GOOS, TrustInstructions(caPath))
	}
}

func installTrustDarwin(ctx context.Context, caPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", caPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("installing the CA into the system keychain failed: %w", err)
	}

	return "The CA was added to the system keychain. Firefox uses its own trust store, enable security.enterprise_roots.enabled in about:config or import the CA manually.", nil
}

func installTrustWindows(ctx context.Context, caPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "certutil", "-addstore", "-f", "ROOT", caPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("installing the CA into the Windows root store failed (run the command from an elevated terminal): %w", err)
	}

	return "The CA was added to the Windows root store. Firefox uses its own trust store, enable security.enterprise_roots.enabled in about:config or import the CA manually.", nil
}

func installTrustLinux(ctx context.Context, caPath string) (string, error) {
	var messages []string

	systemTarget := ""
	updateCommand := []string{}

	switch {
	case dirExists("/usr/local/share/ca-certificates"):
		systemTarget = "/usr/local/share/ca-certificates/shopware-cli-ca.crt"
		updateCommand = []string{"update-ca-certificates"}
	case dirExists("/etc/pki/ca-trust/source/anchors"):
		systemTarget = "/etc/pki/ca-trust/source/anchors/shopware-cli-ca.pem"
		updateCommand = []string{"update-ca-trust"}
	}

	if systemTarget != "" {
		copyCmd := exec.CommandContext(ctx, "sudo", "cp", caPath, systemTarget)
		copyCmd.Stdin = os.Stdin
		copyCmd.Stdout = os.Stdout
		copyCmd.Stderr = os.Stderr

		if err := copyCmd.Run(); err != nil {
			return "", fmt.Errorf("copying the CA into the system trust store failed: %w", err)
		}

		update := exec.CommandContext(ctx, "sudo", updateCommand[0])
		update.Stdin = os.Stdin
		update.Stdout = os.Stdout
		update.Stderr = os.Stderr

		if err := update.Run(); err != nil {
			return "", fmt.Errorf("updating the system trust store failed: %w", err)
		}

		messages = append(messages, "The CA was added to the system trust store.")
	} else {
		messages = append(messages, "No known system trust store found, install manually:\n"+TrustInstructions(caPath))
	}

	// Chrome/Chromium and Firefox on Linux use NSS databases instead of the system store.
	if _, err := exec.LookPath("certutil"); err == nil {
		if home, err := os.UserHomeDir(); err == nil {
			nssdb := filepath.Join(home, ".pki", "nssdb")
			if dirExists(nssdb) {
				cmd := exec.CommandContext(ctx, "certutil", "-A", "-d", "sql:"+nssdb, "-t", "C,,", "-n", "shopware-cli development CA", "-i", caPath)
				if output, err := cmd.CombinedOutput(); err != nil {
					messages = append(messages, fmt.Sprintf("Could not add the CA to the NSS database used by Chrome: %s", strings.TrimSpace(string(output))))
				} else {
					messages = append(messages, "The CA was added to the NSS database used by Chrome/Chromium.")
				}
			}
		}
	} else {
		messages = append(messages, "certutil (libnss3-tools) is not installed, browsers like Chrome and Firefox need the CA imported manually.")
	}

	return strings.Join(messages, "\n"), nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
