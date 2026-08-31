package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DNSContainerName is the fixed name of the shared DNS container that
	// answers *.<domain> with 127.0.0.1 for the OS resolver.
	DNSContainerName = "shopware-cli-proxy-dns"
	// DNSImage is the CoreDNS image used as the DNS responder. CoreDNS is a
	// CNCF project, so there is no DNS-server code for shopware-cli to
	// maintain — only the one-zone Corefile below.
	DNSImage = "coredns/coredns:1.14.6"

	// dnsConfigVersion is stamped on the container as a label; bumping it makes
	// EnsureDNSContainerRunning recreate containers started by older CLI
	// versions with an incompatible Corefile or run arguments.
	dnsConfigVersion      = "2"
	dnsConfigVersionLabel = "com.shopware-cli.proxy-dns-config-version"
	// dnsDomainLabel records the base domain the container's Corefile answers
	// for, so a container serving an outdated domain is recreated.
	dnsDomainLabel = "com.shopware-cli.proxy-dns-domain"
)

// corefileTemplate is CoreDNS's whole configuration: one zone that answers
// every A query under the base domain with 127.0.0.1 (short TTL so teardown or
// a domain change propagates quickly) and returns an empty NOERROR for AAAA,
// so browsers do not stall on IPv6 before falling back to the A record.
const corefileTemplate = `%s {
    template IN A {
        answer "{{ .Name }} %d IN A 127.0.0.1"
    }
    template IN AAAA {
        rcode NOERROR
    }
    errors
}
`

// writeDNSCorefile writes the CoreDNS Corefile for baseDomain below dir (the
// shared state directory), where it is mounted into the container.
func writeDNSCorefile(dir, baseDomain string) error {
	dnsDir := filepath.Join(dir, "dns")
	if err := os.MkdirAll(dnsDir, 0o700); err != nil {
		return err
	}

	content := fmt.Sprintf(corefileTemplate, baseDomain, dnsTTL)
	path := filepath.Join(dnsDir, "Corefile")

	// 0o644: the Corefile is not secret (it only answers 127.0.0.1) and the
	// CoreDNS image runs as UID 65532 (nonroot). A 0o600 file owned by the
	// host user is unreadable through the bind mount and CoreDNS crash-loops
	// with "open /Corefile: permission denied". WriteFile does not change
	// the mode of an existing file, so chmod after write so a leftover 0o600
	// Corefile from earlier CLI versions is repaired in place (same inode,
	// so an already-mounted container sees the new mode immediately).
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	return os.Chmod(path, 0o644)
}

// EnsureDNSContainerRunning idempotently starts the shared DNS container: it
// writes the Corefile, recreates a container left over from an older config or
// a different domain, and (re-)starts one that publishes the DNS port on
// 127.0.0.1 only. It mirrors EnsureTraefikRunning and is safe to call from any
// project at any time.
func EnsureDNSContainerRunning(ctx context.Context, baseDomain string) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}

	if err := writeDNSCorefile(dir, baseDomain); err != nil {
		return err
	}

	if dnsContainerExists(ctx) && !dnsContainerIsCurrent(ctx, baseDomain) {
		if _, err := runDocker(ctx, "rm", "-f", DNSContainerName); err != nil {
			return err
		}
	}

	if dnsContainerIsRunning(ctx) {
		return nil
	}

	if dnsContainerExists(ctx) {
		_, err := runDocker(ctx, "start", DNSContainerName)
		return err
	}

	// Bind to 127.0.0.1 only: the resolver points there and nothing outside the
	// host should reach this responder.
	_, err = runDocker(ctx, "run", "-d",
		"--name", DNSContainerName,
		"--restart", "unless-stopped",
		"--label", dnsConfigVersionLabel+"="+dnsConfigVersion,
		"--label", dnsDomainLabel+"="+baseDomain,
		"-p", fmt.Sprintf("127.0.0.1:%d:53/udp", DNSPort),
		"-p", fmt.Sprintf("127.0.0.1:%d:53/tcp", DNSPort),
		// File mount (not a directory): CoreDNS 1.11+ is USER 65532, so the
		// host Corefile must be world-readable — see writeDNSCorefile.
		"-v", filepath.Join(dir, "dns", "Corefile")+":/Corefile:ro",
		DNSImage,
		"-conf", "/Corefile",
	)
	return err
}

// dnsContainerExists reports whether the shared DNS container exists, running
// or not.
func dnsContainerExists(ctx context.Context) bool {
	out, err := runDocker(ctx, "ps", "-a", "--filter", "name=^"+DNSContainerName+"$", "--format", "{{.Names}}")
	return err == nil && strings.TrimSpace(out) != ""
}

// dnsContainerIsRunning reports whether the shared DNS container is currently
// running.
func dnsContainerIsRunning(ctx context.Context) bool {
	out, err := runDocker(ctx, "ps", "--filter", "name=^"+DNSContainerName+"$", "--filter", "status=running", "--format", "{{.Names}}")
	return err == nil && strings.TrimSpace(out) != ""
}

// dnsContainerIsCurrent reports whether the existing container was created with
// the current config version and answers for baseDomain.
func dnsContainerIsCurrent(ctx context.Context, baseDomain string) bool {
	out, err := runDocker(ctx, "inspect", DNSContainerName, "--format",
		fmt.Sprintf("{{index .Config.Labels %q}}|{{index .Config.Labels %q}}", dnsConfigVersionLabel, dnsDomainLabel))
	return err == nil && strings.TrimSpace(out) == dnsConfigVersion+"|"+baseDomain
}

// StopDNSContainer stops and removes the shared DNS container. It leaves the
// OS resolver configuration in place (removed only by a domain change or a
// manual resolver reset).
func StopDNSContainer(ctx context.Context) error {
	if !dnsContainerExists(ctx) {
		return nil
	}

	_, err := runDocker(ctx, "rm", "-f", DNSContainerName)
	return err
}
