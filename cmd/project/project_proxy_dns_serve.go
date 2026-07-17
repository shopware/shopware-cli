package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
)

// projectProxyDNSServeCmd is the re-exec target for the DNS daemon spawned by
// `project proxy up`. It blocks serving DNS until terminated and is hidden
// because it is never meant to be invoked by users directly.
var projectProxyDNSServeCmd = &cobra.Command{
	Use:    "internal-dns-serve",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		domain, _ := cmd.Flags().GetString("domain")

		return proxy.RunDNSServer(cmd.Context(), port, domain)
	},
}

func init() {
	projectProxyCmd.AddCommand(projectProxyDNSServeCmd)
	projectProxyDNSServeCmd.Flags().Int("port", proxy.DNSPort, "UDP port to serve DNS on")
	projectProxyDNSServeCmd.Flags().String("domain", proxy.DefaultDomain, "Domain to answer queries for")
}
