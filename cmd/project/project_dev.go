package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2/spinner"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/dev"
)

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
		projectRoot, err := findClosestShopwareProject()
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
			return runMigrationWizardTUI(projectRoot, cfg)
		}

		env, err := newDevEnvironment(cmd, projectRoot, cfg)
		if err != nil {
			return err
		}

		if !isatty.IsTerminal(os.Stdin.Fd()) {
			return env.start(cmd)
		}

		return env.runTUI()
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

		return env.stop(cmd)
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

func runMigrationWizardTUI(projectRoot string, cfg *shop.Config) error {
	envCfg := &shop.EnvironmentConfig{Type: "docker", URL: "http://127.0.0.1:8000"}
	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return err
	}

	_, err = dev.NewMigrationWizardApp(dev.Options{
		ProjectRoot: projectRoot,
		ConfigPath:  projectConfigPath,
		Config:      cfg,
		EnvConfig:   envCfg,
		Executor:    exec,
	}).Run()
	return err
}

func setupDevEnvironment(cmd *cobra.Command) (*devEnvironment, error) {
	projectRoot, err := findClosestShopwareProject()
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
	if err := system.ValidateProjectDependencies(cmd.Context(), useDocker, nil, "start the development environment", dockerHint); err != nil {
		return nil, err
	}

	if useDocker {
		if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(cfg)); err != nil {
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

func (e *devEnvironment) dockerPorts() shop.ConfigDockerPorts {
	if e.cfg == nil || e.cfg.Docker == nil {
		return nil
	}
	return e.cfg.Docker.Ports
}

// resolvePortConflicts probes the host ports the compose file will publish.
// Conflicting ports either abort the start with a descriptive error (fail) or
// are remapped to random free ports (random), persisted to the local config
// override so future runs reuse them.
func (e *devEnvironment) resolvePortConflicts(ctx context.Context, mode string) error {
	if e.executor.Type() != executor.TypeDocker {
		return nil
	}

	conflicts, err := dockerpkg.FindPortConflicts(ctx, e.projectRoot, e.dockerPorts())
	if err != nil || len(conflicts) == 0 {
		return err
	}

	if mode != portConflictModeRandom {
		var lines []string
		for _, conflict := range conflicts {
			lines = append(lines, fmt.Sprintf("  %s (%s): port %d is already in use", conflict.Definition.Label, conflict.Definition.Key, conflict.HostPort))
		}
		return fmt.Errorf("cannot start the development environment, host ports are already in use:\n%s\nrerun with --on-port-conflict=random to switch them to free ports, or set docker.ports in %s", strings.Join(lines, "\n"), projectConfigPath)
	}

	overrides, err := dockerpkg.AllocateRandomPorts(ctx, conflicts)
	if err != nil {
		return err
	}

	if err := shop.UpdateLocalDockerPorts(e.configPath, overrides); err != nil {
		return err
	}

	e.cfg.SetDockerPortOverrides(overrides)

	if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
		return err
	}

	for _, conflict := range conflicts {
		fmt.Println("  " + tui.DimText.Render(fmt.Sprintf("%s: port %d is in use, switched to %d", conflict.Definition.Label, conflict.HostPort, overrides[conflict.Definition.Key])))
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

	// Errors past flag validation are runtime failures, not usage mistakes.
	cmd.SilenceUsage = true

	if err := e.resolvePortConflicts(cmd.Context(), mode); err != nil {
		return err
	}

	start := time.Now()

	err = spinner.New().
		Title("Starting development environment...").
		Context(cmd.Context()).
		ActionWithErr(func(ctx context.Context) error {
			return e.executor.StartEnvironment(ctx)
		}).
		Run()

	if err != nil {
		return fmt.Errorf("starting environment: %w", err)
	}

	elapsed := time.Since(start).Round(time.Millisecond)

	fmt.Println("  " + tui.SuccessLine(fmt.Sprintf("Development environment started in %s", elapsed)))
	fmt.Println()

	shopURL := e.cfg.URL
	if e.envCfg.URL != "" {
		shopURL = e.envCfg.URL
	}

	var services []dev.DiscoveredService
	if e.executor.Type() == executor.TypeDocker {
		var webPort int
		services, webPort, _ = dev.DiscoverComposeServices(cmd.Context(), e.projectRoot)
		shopURL = dev.ResolveShopURL(shopURL, webPort)
	}

	if shopURL != "" {
		adminURL := shopURL
		if !strings.HasSuffix(adminURL, "/") {
			adminURL += "/"
		}
		adminURL += "admin"

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

func (e *devEnvironment) stop(cmd *cobra.Command) error {
	start := time.Now()

	err := spinner.New().
		Title("Stopping development environment...").
		Context(cmd.Context()).
		ActionWithErr(func(ctx context.Context) error {
			return e.executor.StopEnvironment(ctx)
		}).
		Run()

	if err != nil {
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

func (e *devEnvironment) runTUI() error {
	_, err := dev.NewApp(dev.Options{
		ProjectRoot: e.projectRoot,
		ConfigPath:  e.configPath,
		Config:      e.cfg,
		EnvConfig:   e.envCfg,
		Executor:    e.executor,
	}).Run()
	return err
}

func init() {
	projectRootCmd.AddCommand(projectDevCmd)
	projectDevCmd.AddCommand(projectDevStartCmd)
	projectDevCmd.AddCommand(projectDevStopCmd)
	projectDevCmd.AddCommand(projectDevStatusCmd)
	projectDevCmd.PersistentFlags().String("on-port-conflict", portConflictModeFail, "What to do when host ports are already in use: fail or random. Applies when starting non-interactively; the dashboard asks instead.")
}
