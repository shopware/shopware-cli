package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
	cfg         *shop.Config
	envCfg      *shop.EnvironmentConfig
	executor    executor.Executor
}

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
			return runMigrationWizardTUI(projectRoot, cfg, updateBackgroundCmd(cmd.Context()))
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

func runMigrationWizardTUI(projectRoot string, cfg *shop.Config, backgroundCmd tea.Cmd) error {
	envCfg := &shop.EnvironmentConfig{Type: "docker", URL: "http://127.0.0.1:8000"}
	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return err
	}

	_, err = dev.NewMigrationWizardApp(dev.Options{
		ProjectRoot:   projectRoot,
		Config:        cfg,
		EnvConfig:     envCfg,
		Executor:      exec,
		BackgroundCmd: backgroundCmd,
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
		if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(cfg)); err != nil {
			return nil, err
		}
	}

	return &devEnvironment{
		projectRoot:   projectRoot,
		cfg:           cfg,
		envCfg:        envCfg,
		executor:      exec,
		backgroundCmd: updateBackgroundCmd(cmd.Context()),
	}, nil
}

func (e *devEnvironment) start(cmd *cobra.Command) error {
	start := time.Now()

	err := spinner.New().
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

func (e *devEnvironment) stop(cmd *cobra.Command, opts executor.StopOptions) error {
	start := time.Now()

	title := "Stopping development environment..."
	if opts.RemoveVolumes {
		title = "Stopping development environment and removing data..."
	}

	err := spinner.New().
		Title(title).
		Context(cmd.Context()).
		ActionWithErr(func(ctx context.Context) error {
			return e.executor.StopEnvironment(ctx, opts)
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

	projectDevStopCmd.Flags().Bool("remove-data", false, "Also remove the named volumes declared in the compose file, deleting all data stored in them")
}
