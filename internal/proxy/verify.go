package proxy

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// CheckResult is the outcome of one verification step.
type CheckResult struct {
	// Name describes what was checked, e.g. "OS resolves *.shopware.local".
	Name string
	// Err is nil when the check passed.
	Err error
	// Hint tells the user how to fix a failed check.
	Hint string
}

// Verify runs the proxy health checks bottom-up and stops at the first
// failure, since later layers depend on earlier ones. It never mutates any
// state, so it is safe to run at any time.
func Verify(ctx context.Context, baseDomain string) []CheckResult {
	probe := randomProbeHostname(baseDomain)

	checks := []struct {
		name string
		run  func(context.Context) error
		hint string
	}{
		{
			name: "Docker is running",
			run:  checkDocker,
			hint: "Start Docker and run \"shopware-cli project proxy verify\" again",
		},
		{
			name: fmt.Sprintf("DNS server answers *.%s", baseDomain),
			run: func(ctx context.Context) error {
				return checkDNSDaemon(ctx, probe)
			},
			hint: "Run \"shopware-cli project proxy setup\" (or \"proxy up\" in a shop) to start it",
		},
		{
			name: fmt.Sprintf("OS resolves *.%s to 127.0.0.1", baseDomain),
			run: func(ctx context.Context) error {
				return checkOSResolution(ctx, probe)
			},
			hint: osResolutionHint(baseDomain, SupportsWildcardDNS(ctx)),
		},
		{
			name: "Shared proxy is running on port 443",
			run:  checkTraefik,
			hint: "Run \"shopware-cli project proxy up\" in a shop to start it; another local web server may also occupy port 80/443",
		},
		{
			name: fmt.Sprintf("HTTPS works and is trusted (https://%s/ping)", PingHostname(baseDomain)),
			run: func(ctx context.Context) error {
				return checkHTTPS(ctx, PingHostname(baseDomain))
			},
			hint: "Run \"shopware-cli project proxy setup\" so browsers trust the local HTTPS certificates (if sudo is blocked, it prints what to ask your IT team; Firefox needs certutil: " + certutilInstallHint() + ")",
		},
	}

	var results []CheckResult
	for _, check := range checks {
		err := check.run(ctx)

		result := CheckResult{Name: check.name, Err: err}
		if err != nil {
			result.Hint = check.hint
		}

		results = append(results, result)

		if err != nil {
			break
		}
	}

	return results
}

// osResolutionHint lists the likely causes when the DNS server answers but
// the operating system does not route queries to it. On macOS with a .local
// domain it additionally points at the mDNS/Bonjour conflict, since Apple
// treats that TLD specially. On systems without wildcard support it explains
// why and shows the manual /etc/hosts last resort instead.
func osResolutionHint(baseDomain string, wildcardSupported bool) string {
	if !wildcardSupported {
		return "This check can never pass here. " + NoSystemdResolvedGuidance(baseDomain)
	}

	lines := []string{
		"The DNS server works, but your system is not asking it. Likely causes:",
		"1. The one-time setup was never done on this machine",
		"   → run \"shopware-cli project proxy setup\" (needs sudo once)",
		"2. A VPN or a corporate security tool is taking over DNS on your machine",
		"   → disconnect the VPN and run \"shopware-cli project proxy verify\" again;",
		"     if it works then, ask your IT team to allow local DNS rules",
	}

	if runtime.GOOS == "darwin" && strings.HasSuffix(baseDomain, ".local") {
		alternative := strings.TrimSuffix(baseDomain, ".local") + ".internal"
		lines = append(lines,
			"3. macOS treats \".local\" names specially (Bonjour), which can conflict",
			"   → try a domain outside .local: \"shopware-cli project proxy setup --domain "+alternative+"\"",
		)
	}

	return strings.Join(lines, "\n")
}

// randomProbeHostname returns a hostname under baseDomain that no cache has
// seen yet, so resolution checks prove live behavior.
func randomProbeHostname(baseDomain string) string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)

	return fmt.Sprintf("verify-%s.%s", hex.EncodeToString(buf), baseDomain)
}

func checkDocker(ctx context.Context) error {
	_, err := runDocker(ctx, "version", "--format", "{{.Server.Version}}")
	return err
}

// checkDNSDaemon queries the embedded DNS server directly, bypassing the OS
// resolver, to isolate daemon problems from resolver-configuration problems.
func checkDNSDaemon(ctx context.Context, probe string) error {
	resp, err := queryDNS(ctx, fmt.Sprintf("127.0.0.1:%d", DNSPort), probe, dnsmessage.TypeA, 2*time.Second)
	if err != nil {
		return fmt.Errorf("the DNS server on 127.0.0.1:%d did not answer: %w", DNSPort, err)
	}

	for _, answer := range resp.Answers {
		if a, ok := answer.Body.(*dnsmessage.AResource); ok && a.A == [4]byte{127, 0, 0, 1} {
			return nil
		}
	}

	return fmt.Errorf("the DNS server answered %s without an A record for 127.0.0.1", probe)
}

// checkOSResolution resolves the probe through the operating system's own
// resolution path (not Go's resolver, which bypasses /etc/resolver and
// split-DNS configuration). It retries briefly because freshly written
// resolver configuration can take a moment to become effective.
func checkOSResolution(ctx context.Context, probe string) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = resolveViaOS(ctx, probe); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	return err
}

func checkTraefik(ctx context.Context) error {
	if !ContainerIsRunning(ctx) {
		return errors.New("the shared proxy container is not running")
	}

	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", "127.0.0.1:443")
	if err != nil {
		return fmt.Errorf("port 443 is not reachable: %w", err)
	}
	_ = conn.Close()

	return nil
}

// checkHTTPS proves the last mile: a TLS request to Traefik's ping endpoint
// validated against the system trust store — exactly what a browser does.
// The connection dials 127.0.0.1 directly since DNS correctness is already
// covered by checkOSResolution. A freshly started Traefik container accepts
// TCP (Docker publishes the port immediately) but drops requests for a few
// seconds until its config is loaded, so transient failures are retried;
// certificate errors are permanent and fail immediately.
func checkHTTPS(ctx context.Context, pingHostname string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, "127.0.0.1:443")
			},
		},
	}

	deadline := time.Now().Add(8 * time.Second)

	for {
		err := pingOnce(ctx, client, pingHostname)
		if err == nil {
			return nil
		}

		// Certificate errors can be transient too: a restarting Traefik
		// briefly serves its self-signed default certificate until the
		// dynamic configuration is applied. Only a failure that persists
		// past the deadline is reported, classified for the right hint.
		if time.Now().After(deadline) {
			var certErr *tls.CertificateVerificationError
			if errors.As(err, &certErr) || strings.Contains(err.Error(), "certificate") {
				return fmt.Errorf("the certificate is not trusted: %w", err)
			}

			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func pingOnce(ctx context.Context, client *http.Client, pingHostname string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/ping", pingHostname), nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("https://%s/ping answered status %d instead of 200", pingHostname, resp.StatusCode)
	}

	return nil
}
