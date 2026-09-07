package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/dev"
)

// bootstrapProxyFallback sets up the shared proxy for a proxy-mode project
// before its development environment starts, so `project dev` serves it at its
// stable hostname, and records the outcome in e.proxyFallback. A proxy that
// cannot start (e.g. its port is taken) never blocks dev: the compose file is
// regenerated in plain fixed-port mode, the user is pointed at a fix and the
// shop is marked as fallen back to a local port. Only a failure to write that
// plain compose file is an error, because starting would then use the proxy
// file without a proxy. It is a no-op for port-based projects.
func (e *devEnvironment) bootstrapProxyFallback(cmd *cobra.Command) error {
	if !proxy.IsProxyProject(e.cfg) {
		return nil
	}

	ctx := cmd.Context()
	err := func() error {
		env, err := newProxyEnvironmentForRoot(ctx, e.projectRoot, projectConfigPath)
		if err != nil {
			return err
		}
		if err := runStep(ctx, "Preparing shared proxy...", env.bootstrapInfra); err != nil {
			return err
		}
		env.ensureHostnameResolves(ctx)
		return nil
	}()
	if err == nil {
		return nil
	}

	// An interrupted or timed-out bootstrap is not a proxy failure: propagate
	// it instead of falling back and starting the environment anyway.
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// Never block dev on the proxy: regenerate the compose file in fixed-port
	// mode (newDevEnvironment wrote it in proxy mode) and tell the user how to
	// diagnose the proxy.
	plain, writeErr := proxy.NewEnvironment(e.projectRoot, e.cfg, true)
	if writeErr == nil {
		writeErr = plain.WriteCompose()
	}
	if writeErr != nil {
		return fmt.Errorf("shared proxy unavailable (%v) and the fixed-port compose file could not be written: %w", err, writeErr)
	}

	fmt.Println(tui.RedText.Render("  Shared proxy unavailable: " + err.Error()))
	fmt.Println(tui.DimText.Render("  Serving on a local port instead — run ") + tui.BoldText.Render("shopware-cli project proxy verify") + tui.DimText.Render(" to diagnose."))
	e.proxyFallback = true

	return nil
}

// ErrEnvironmentDown is returned by the `project dev status` command when the
// development environment is not running. It causes the CLI to exit with code 1
// without printing an additional error message.
var ErrEnvironmentDown = errors.New("development environment is down")

type devEnvironment struct {
	projectRoot string
	configPath  string
	cfg         *shop.Config
	envCfg      *shop.EnvironmentConfig
	executor    executor.Executor
	// proxyFallback is set when a proxy project could not start the shared
	// proxy and was served on a local port instead, so URLs are shown for ports.
	proxyFallback bool
}

// Values for the --on-port-conflict flag.
const (
	portConflictModeFail   = "fail"
	portConflictModeRandom = "random"
)

var projectDevCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the development environment",
	Long:  "Start the development environment. Launches the interactive TUI dashboard when run in a terminal, or starts containers in the background otherwise.",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		// If the compatibility date is too old, offer to set up dev mode via the TUI
		if cfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
			if !isatty.IsTerminal(os.Stdin.Fd()) {
				return shop.ErrDevModeNotSupported
			}
			return runMigrationWizardTUI(cmd.Context(), projectRoot, cfg)
		}

		env, err := newDevEnvironment(cmd, projectRoot, cfg)
		if err != nil {
			return err
		}

		if err := env.bootstrapProxyFallback(cmd); err != nil {
			return err
		}

		if !isatty.IsTerminal(os.Stdin.Fd()) {
			if err := env.start(cmd); err != nil {
				return err
			}
			if !install.IsInstalled(cmd.Context(), env.executor) {
				fmt.Println(tui.DimText.Render("  Shopware is not installed yet. Run ") + tui.BoldText.Render("shopware-cli project dev install") + tui.DimText.Render(" to install it."))
				fmt.Println()
			}
			return nil
		}

		return env.runTUI(cmd.Context())
	},
}

var projectDevStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the development environment in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := setupDevEnvironment(cmd)
		if err != nil {
			return err
		}

		if err := env.bootstrapProxyFallback(cmd); err != nil {
			return err
		}

		return env.start(cmd)
	},
}

var projectDevStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the development environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := setupDevEnvironment(cmd)
		if err != nil {
			return err
		}

		removeVolumes, _ := cmd.Flags().GetBool("remove-data")

		return env.stop(cmd, executor.StopOptions{RemoveVolumes: removeVolumes})
	},
}

var projectDevStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Report whether the development environment is running",
	Long:         "Report whether the development environment is running. Exits with code 0 when it is up and code 1 when it is down.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := setupDevEnvironment(cmd)
		if err != nil {
			return err
		}

		return env.status(cmd)
	},
}

func runMigrationWizardTUI(ctx context.Context, projectRoot string, cfg *shop.Config) error {
	envCfg := &shop.EnvironmentConfig{Type: "docker", URL: shop.DefaultShopURL}
	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return err
	}

	_, err = dev.NewMigrationWizardApp(ctx, dev.Options{
		ProjectRoot: projectRoot,
		ConfigPath:  projectConfigPath,
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    exec,
	}).Run()
	return err
}

func setupDevEnvironment(cmd *cobra.Command) (*devEnvironment, error) {
	projectRoot, err := findClosestShopwareProject(false)
	if err != nil {
		return nil, err
	}

	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
	if err != nil {
		return nil, err
	}

	if cfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
		return nil, shop.ErrDevModeNotSupported
	}

	return newDevEnvironment(cmd, projectRoot, cfg)
}

// newDevEnvironment resolves the configured environment, verifies that the
// system requirements for it are met (Docker running for Docker environments,
// PHP and Composer for local ones) and prepares the executor. Shared by
// `project dev` and its start/stop/status subcommands.
func newDevEnvironment(cmd *cobra.Command, projectRoot string, cfg *shop.Config) (*devEnvironment, error) {
	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}

	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return nil, err
	}

	useDocker := exec.Type() == executor.TypeDocker
	dockerHint := "set the environment " + tui.BoldText.Render("type") + " to " + tui.BoldText.Render("docker") + " in " + tui.BoldText.Render(".shopware-project.yml")

	// Docker gets its PHP from the image. Must use the same precedence as the
	// executor, or the dependencies of a different PHP would be validated.
	var phpBinary string
	if !useDocker {
		phpBinary, err = system.ResolveProjectPHPBinary(cmd.Context(), cfg.PHPVersion)
		if err != nil {
			return nil, err
		}
	}

	if err := system.ValidateProjectDependencies(cmd.Context(), useDocker, nil, "start the development environment", dockerHint, phpBinary); err != nil {
		return nil, err
	}

	if useDocker {
		// Proxy-aware: a project configured for a local domain gets a proxy-mode
		// compose file, a port-based one the plain fixed-port file. A failed
		// proxy bootstrap later reverts it to plain (see the fallback above).
		env, err := proxy.NewEnvironment(projectRoot, cfg, false)
		if err != nil {
			return nil, err
		}
		if err := env.WriteCompose(); err != nil {
			return nil, err
		}
	}

	return &devEnvironment{
		projectRoot: projectRoot,
		configPath:  projectConfigPath,
		cfg:         cfg,
		envCfg:      envCfg,
		executor:    exec,
	}, nil
}

// dockerEnvironment resolves the project's dev environment for its effective
// run mode, honoring a proxy fallback.
func (e *devEnvironment) dockerEnvironment() (*dockerpkg.Environment, error) {
	return proxy.NewEnvironment(e.projectRoot, e.cfg, e.proxyFallback)
}

// resolvePortConflicts probes the host ports the compose file will publish.
// Conflicting ports either abort the start (fail) or are remapped to random
// free ports (random) and persisted to the local config override.
func (e *devEnvironment) resolvePortConflicts(ctx context.Context, mode string) error {
	if e.executor.Type() != executor.TypeDocker {
		return nil
	}

	env, err := e.dockerEnvironment()
	if err != nil {
		return err
	}
	conflicts := env.PortConflicts(ctx)
	if len(conflicts) == 0 {
		return nil
	}

	if mode != portConflictModeRandom {
		var lines []string
		for _, conflict := range conflicts {
			lines = append(lines, fmt.Sprintf("  %s (%s): port %d is already in use", conflict.Label, conflict.ConfigPath(), conflict.HostPort))
		}
		return fmt.Errorf("cannot start the development environment, host ports are already in use:\n%s\nrerun with --on-port-conflict=random to switch them to free ports, or set docker.services in %s", strings.Join(lines, "\n"), shop.LocalConfigFileName(e.configPath))
	}

	cfg, overrides, err := proxy.ApplyRandomPorts(ctx, e.projectRoot, e.configPath, e.cfg, e.proxyFallback, conflicts)
	if err != nil {
		return err
	}
	e.cfg = cfg

	for i, conflict := range conflicts {
		fmt.Println("  " + tui.DimText.Render(fmt.Sprintf("%s: port %d is in use, switched to %d", conflict.Label, conflict.HostPort, overrides[i].HostPort)))
	}
	fmt.Println("  " + tui.DimText.Render("Saved the new ports to "+shop.LocalConfigFileName(e.configPath)))
	fmt.Println()

	return nil
}

func (e *devEnvironment) start(cmd *cobra.Command) error {
	mode, err := cmd.Flags().GetString("on-port-conflict")
	if err != nil {
		return err
	}
	if mode != portConflictModeFail && mode != portConflictModeRandom {
		return fmt.Errorf("invalid value %q for --on-port-conflict, must be %q or %q", mode, portConflictModeFail, portConflictModeRandom)
	}

	if err := e.resolvePortConflicts(cmd.Context(), mode); err != nil {
		return err
	}

	start := time.Now()

	// runStep falls back to running the action directly without a spinner when
	// there is no interactive terminal, so `project dev start` also works
	// headless (e.g. an agent, CI, or a pipe with no /dev/tty).
	if err := runStep(cmd.Context(), "Starting development environment...", e.executor.StartEnvironment); err != nil {
		return fmt.Errorf("starting environment: %w", err)
	}

	elapsed := time.Since(start).Round(time.Millisecond)

	fmt.Println("  " + tui.SuccessLine(fmt.Sprintf("Development environment started in %s", elapsed)))
	fmt.Println()

	shopURL := e.cfg.URL
	if e.envCfg.URL != "" {
		shopURL = e.envCfg.URL
	}
	// After a proxy fallback the shop is on a local port, not its hostname.
	if e.proxyFallback {
		shopURL = shop.DefaultShopURL
	}

	var services []dockerpkg.DiscoveredService
	if e.executor.Type() == executor.TypeDocker {
		if env, err := e.dockerEnvironment(); err == nil {
			if running, err := env.Discover(cmd.Context()); err == nil {
				services = running.Services
				shopURL = dev.ResolveShopURL(shopURL, running.WebPort)
			}
		}
	}

	if shopURL != "" {
		adminURL := dev.DeriveAdminURL(shopURL)

		fmt.Println(tui.SectionTitleStyle.Render("  Shop"))
		fmt.Println(tui.DimText.Render("  Shop URL:  ") + tui.BoldText.Render(shopURL))
		fmt.Println(tui.DimText.Render("  Admin URL: ") + tui.BoldText.Render(adminURL))
		fmt.Println()
	}

	if len(services) > 0 {
		fmt.Println(tui.SectionTitleStyle.Render("  Services"))
		for _, svc := range services {
			fmt.Println(tui.DimText.Render("  "+svc.Name+": ") + tui.BoldText.Render(svc.URL))
		}
		fmt.Println()
	}

	fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project dev stop") + tui.DimText.Render(" to stop it."))
	fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project logs") + tui.DimText.Render(" to view application logs."))
	fmt.Println()

	return nil
}

func (e *devEnvironment) stop(cmd *cobra.Command, opts executor.StopOptions) error {
	start := time.Now()

	title := "Stopping development environment..."
	if opts.RemoveVolumes {
		title = "Stopping development environment and removing data..."
	}

	stop := func(ctx context.Context) error {
		return e.executor.StopEnvironment(ctx, opts)
	}

	if err := runStep(cmd.Context(), title, stop); err != nil {
		return fmt.Errorf("stopping environment: %w", err)
	}

	elapsed := time.Since(start).Round(time.Millisecond)

	fmt.Println("  " + tui.SuccessLine(fmt.Sprintf("Development environment stopped in %s", elapsed)))
	fmt.Println()

	return nil
}

func (e *devEnvironment) status(cmd *cobra.Command) error {
	running, err := e.executor.EnvironmentStatus(cmd.Context())
	if err != nil {
		if errors.Is(err, executor.ErrNotSupported) {
			return fmt.Errorf("the %s environment does not manage a development environment", e.executor.Type())
		}
		return fmt.Errorf("checking environment status: %w", err)
	}

	if running {
		fmt.Println("  " + tui.SuccessLine("Development environment is up"))
		return nil
	}

	fmt.Println("  " + tui.FailLine("Development environment is down"))
	return ErrEnvironmentDown
}

func (e *devEnvironment) runTUI(ctx context.Context) error {
	_, err := dev.NewApp(ctx, dev.Options{
		ProjectRoot:   e.projectRoot,
		ConfigPath:    e.configPath,
		Config:        e.cfg,
		EnvConfig:     e.envCfg,
		Executor:      e.executor,
		ProxyFallback: e.proxyFallback,
	}).Run()
	return err
}

func init() {
	projectRootCmd.AddCommand(projectDevCmd)
	projectDevCmd.AddCommand(projectDevStartCmd)
	projectDevCmd.AddCommand(projectDevStopCmd)
	projectDevCmd.AddCommand(projectDevStatusCmd)

	projectDevStopCmd.Flags().Bool("remove-data", false, "Also remove the named volumes declared in the compose file, deleting all data stored in them")
	projectDevCmd.PersistentFlags().String("on-port-conflict", portConflictModeFail, "What to do when host ports are already in use: fail or random. Applies when starting non-interactively; the dashboard asks instead.")
}
