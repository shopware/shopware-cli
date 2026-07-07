package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/logging"
)

var projectProxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Reach local instances via stable hostnames instead of ports",
	Long: `Manages a shared local reverse proxy (Traefik) that routes stable per-project
hostnames like https://my-shop.127.0.0.1.sslip.io to your local Shopware
instances. This allows running multiple instances in parallel without juggling
ports, with locally trusted HTTPS.`,
}

var projectProxyUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the shared proxy container",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		logger := logging.FromContext(ctx)

		dir, err := proxy.Dir()
		if err != nil {
			return err
		}

		settings, err := proxy.LoadSettings(dir)
		if err != nil {
			return err
		}

		if domain, _ := cmd.Flags().GetString("domain"); domain != "" {
			settings.Domain = domain
		}

		if port, _ := cmd.Flags().GetInt("http-port"); port != 0 {
			settings.HTTPPort = port
		}

		if port, _ := cmd.Flags().GetInt("https-port"); port != 0 {
			settings.HTTPSPort = port
		}

		if err := proxy.SaveSettings(dir, settings); err != nil {
			return err
		}

		certInfo, err := proxy.EnsureCertificate(dir, proxy.CertHosts(settings.Domain, settings.Hosts))
		if err != nil {
			return err
		}

		if err := proxy.WriteTraefikConfig(dir, settings); err != nil {
			return err
		}

		if err := proxy.EnsureNetwork(ctx); err != nil {
			return err
		}

		if err := proxy.StartContainer(ctx, dir, settings); err != nil {
			return err
		}

		logger.Infof("Proxy is running, instances will be reachable at https://<name>.%s%s", settings.Domain, httpsPortSuffix(settings))
		logger.Infof("Register a project by running \"shopware-cli project proxy add\" inside the project")

		if certInfo.CACreated {
			logger.Infof("A new mkcert root CA was created at %s. Run \"shopware-cli project proxy trust\" once so browsers accept the HTTPS certificates", certInfo.CAPath)
		} else {
			logger.Infof("Certificates are issued by the mkcert root CA at %s. If it is not trusted yet, run \"shopware-cli project proxy trust\" once", certInfo.CAPath)
		}

		return nil
	},
}

var projectProxyDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop and remove the shared proxy container",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := proxy.StopContainer(cmd.Context()); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Proxy container removed")

		return nil
	},
}

var projectProxyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the proxy state and running instances",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		logger := logging.FromContext(ctx)

		dir, err := proxy.Dir()
		if err != nil {
			return err
		}

		settings, err := proxy.LoadSettings(dir)
		if err != nil {
			return err
		}

		if proxy.ContainerIsRunning(ctx) {
			logger.Infof("Proxy is running on port %d (HTTP) and %d (HTTPS), domain %s", settings.HTTPPort, settings.HTTPSPort, settings.Domain)
		} else {
			logger.Infof("Proxy is not running, start it with \"shopware-cli project proxy up\"")

			return nil
		}

		instances, err := proxy.RunningInstances(ctx)
		if err != nil {
			return err
		}

		if len(instances) == 0 {
			logger.Infof("No proxied instances are running")

			return nil
		}

		for _, instance := range instances {
			logger.Infof("%s -> https://%s%s", instance.Container, instance.Host, httpsPortSuffix(settings))
		}

		return nil
	},
}

var projectProxyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Route a stable hostname to the project in the current directory",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		logger := logging.FromContext(ctx)

		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		dir, err := proxy.Dir()
		if err != nil {
			return err
		}

		settings, err := proxy.LoadSettings(dir)
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = filepath.Base(projectRoot)
		}
		name = proxy.SanitizeName(name)

		host, _ := cmd.Flags().GetString("host")
		if host == "" {
			host = fmt.Sprintf("%s.%s", name, settings.Domain)
		}

		service, _ := cmd.Flags().GetString("service")
		upstreamPort, _ := cmd.Flags().GetInt("upstream-port")

		overridePath, err := proxy.WriteComposeOverride(projectRoot, name, host, service, upstreamPort)
		if err != nil {
			return err
		}

		appURL := fmt.Sprintf("https://%s%s", host, httpsPortSuffix(settings))

		if err := envfile.UpsertEnvVar(filepath.Join(projectRoot, ".env.local"), "APP_URL", appURL); err != nil {
			return err
		}

		if err := proxy.UpdateProjectConfigURL(projectConfigPathFor(projectRoot), appURL); err != nil {
			return err
		}

		if settings.RegisterHost(host) {
			if err := proxy.SaveSettings(dir, settings); err != nil {
				return err
			}
		}

		certInfo, err := proxy.EnsureCertificate(dir, proxy.CertHosts(settings.Domain, settings.Hosts))
		if err != nil {
			return err
		}

		if certInfo.Changed {
			if err := proxy.RestartContainer(ctx); err != nil {
				return err
			}
		}

		logger.Infof("Created %s", overridePath)
		logger.Infof("Set APP_URL=%s in .env.local and updated the project config", appURL)
		logger.Infof("Apply it by restarting the instance: make up (or docker compose up -d)")
		logger.Infof("For a fresh install run \"make setup\", for an existing database update the sales channel domain:")
		logger.Infof("  docker compose exec %s bin/console sales-channel:update:domain %s", service, appURL)

		if !proxy.ContainerIsRunning(ctx) {
			logger.Infof("The proxy is not running yet, start it with \"shopware-cli project proxy up\"")
		}

		return nil
	},
}

var projectProxyRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the proxy routing from the project in the current directory",
	RunE: func(cmd *cobra.Command, _ []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		path, err := proxy.RemoveComposeOverride(projectRoot)
		if err != nil {
			return err
		}

		logger := logging.FromContext(cmd.Context())
		logger.Infof("Removed %s", path)
		logger.Infof("APP_URL in .env.local and the url in the project config were kept, adjust them if needed")
		logger.Infof("Apply it by restarting the instance: make up (or docker compose up -d)")

		return nil
	},
}

var projectProxyTrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Install the mkcert root CA into the trust stores",
	RunE: func(cmd *cobra.Command, _ []string) error {
		caPath, err := proxy.CACertPath()
		if err != nil {
			return err
		}

		summary, err := proxy.InstallTrust(cmd.Context(), caPath)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("%s", summary)

		return nil
	},
}

// projectConfigPathFor resolves the project config path relative to the
// project root, honoring an absolute --project-config flag.
func projectConfigPathFor(projectRoot string) string {
	if filepath.IsAbs(projectConfigPath) {
		return projectConfigPath
	}

	return filepath.Join(projectRoot, filepath.Base(projectConfigPath))
}

// httpsPortSuffix returns the port part of instance URLs, empty for the default port.
func httpsPortSuffix(settings proxy.Settings) string {
	if settings.HTTPSPort == 443 {
		return ""
	}

	return fmt.Sprintf(":%d", settings.HTTPSPort)
}

func init() {
	projectRootCmd.AddCommand(projectProxyCmd)
	projectProxyCmd.AddCommand(projectProxyUpCmd)
	projectProxyCmd.AddCommand(projectProxyDownCmd)
	projectProxyCmd.AddCommand(projectProxyStatusCmd)
	projectProxyCmd.AddCommand(projectProxyAddCmd)
	projectProxyCmd.AddCommand(projectProxyRemoveCmd)
	projectProxyCmd.AddCommand(projectProxyTrustCmd)

	projectProxyUpCmd.Flags().String("domain", "", fmt.Sprintf("Base domain for instance hostnames (default %s)", proxy.DefaultDomain))
	projectProxyUpCmd.Flags().Int("http-port", 0, "Host port for HTTP (default 80)")
	projectProxyUpCmd.Flags().Int("https-port", 0, "Host port for HTTPS (default 443)")

	projectProxyAddCmd.Flags().String("name", "", "Instance name used as subdomain (default: sanitized folder name)")
	projectProxyAddCmd.Flags().String("host", "", "Full hostname for the instance (default: <name>.<domain>)")
	projectProxyAddCmd.Flags().String("service", "web", "Compose service that serves the shop")
	projectProxyAddCmd.Flags().Int("upstream-port", 8000, "Container port the shop listens on")
}
