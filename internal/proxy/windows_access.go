package proxy

import (
	"fmt"
	"strings"
)

// ProxyHostnames returns the browser-facing hostnames for a shop served through
// the proxy: the root hostname plus every routed subdomain (matching the routes
// in internal/docker/compose_override.go). It is used to build the Windows hosts
// file line under WSL, where wildcards are not available.
func ProxyHostnames(hostname string, hasAMQP, hasElasticsearch, hasK8sMeta bool) []string {
	subdomains := []string{"", "admin-watch", "storefront-watch", "adminer", "mailer"}
	if hasAMQP {
		subdomains = append(subdomains, "lavinmq")
	}
	if hasElasticsearch {
		subdomains = append(subdomains, "opensearch")
	}
	if hasK8sMeta {
		subdomains = append(subdomains, "s3", "rustfs")
	}

	hosts := make([]string, 0, len(subdomains))
	for _, sub := range subdomains {
		if sub == "" {
			hosts = append(hosts, hostname)
			continue
		}
		hosts = append(hosts, sub+"."+hostname)
	}
	return hosts
}

const (
	// windowsHostsFile is the Windows hosts file the WSL guidance points at; the
	// CLI cannot edit it from inside the subsystem.
	windowsHostsFile = `C:\Windows\System32\drivers\etc\hosts`
	// windowsCACopyPath / wslWindowsCACopyMount are the same file seen from
	// Windows and from WSL. Users\Public is world-accessible, so the copy and
	// trust commands need no Windows username and stay copy-pasteable for anyone.
	windowsCACopyPath     = `C:\Users\Public\shopware-cli-rootCA.pem`
	wslWindowsCACopyMount = "/mnt/c/Users/Public/shopware-cli-rootCA.pem"
)

// WSLWindowsAccessGuidance describes the one-time manual steps needed to reach
// proxy-served shops from a browser running on Windows when shopware-cli runs
// under WSL: the setup only touches the Linux side of the subsystem, so Windows
// still needs the CA imported into its trust store and the shop hostnames added
// to its hosts file. hostnames is the ready-to-paste host list for step 3; when
// it is empty (e.g. `proxy setup` run outside a project) that line is replaced
// by a pointer to run `proxy up` in a shop folder to obtain it.
func WSLWindowsAccessGuidance(caPath string, hostnames []string) string {
	var b strings.Builder

	// Singular for a concrete shop (proxy up / create), generic when there is no
	// project context (proxy setup run outside a shop folder).
	target := "this shop"
	if len(hostnames) == 0 {
		target = "your shops"
	}

	b.WriteString("WSL detected — shopware-cli configures only the Linux side of the subsystem.\n")
	fmt.Fprintf(&b, "To open %s in a browser on Windows, do these one-time manual steps:\n", target)
	b.WriteString("\n")

	b.WriteString("1. Copy the certificate authority to Windows:\n")
	fmt.Fprintf(&b, "     cp %s %s\n", caPath, wslWindowsCACopyMount)
	b.WriteString("\n")

	b.WriteString("2. Trust it — in PowerShell or CMD opened as Administrator:\n")
	fmt.Fprintf(&b, "     certutil -addstore -f ROOT %s\n", windowsCACopyPath)
	b.WriteString("   then fully restart the browser.\n")
	b.WriteString("\n")

	b.WriteString("3. Add the hostnames to the Windows hosts file (open the editor as Administrator):\n")
	fmt.Fprintf(&b, "     %s\n", windowsHostsFile)
	if len(hostnames) > 0 {
		fmt.Fprintf(&b, "     127.0.0.1 %s\n", strings.Join(hostnames, " "))
	} else {
		b.WriteString("   Run \"shopware-cli project proxy up\" in a shop folder to get the exact line to add.\n")
	}

	return b.String()
}
