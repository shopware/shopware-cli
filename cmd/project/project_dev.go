package project

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2/spinner"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/devtui"
	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

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
		env, err := setupDevEnvironment(cmd)
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

// setupDevEnvironment reads config, creates the executor, and writes the compose
// file if using Docker. It is shared by all dev subcommands.
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

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}

	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return nil, err
	}

	if exec.Type() == executor.TypeDocker {
		if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(cfg)); err != nil {
			return nil, err
		}
	}

	return &devEnvironment{
		projectRoot: projectRoot,
		cfg:         cfg,
		envCfg:      envCfg,
		executor:    exec,
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

	fmt.Println(tui.GreenText.Bold(true).Render(fmt.Sprintf("  ✓ Development environment started in %s", elapsed)))
	fmt.Println()

	shopURL := e.cfg.URL
	if e.envCfg.URL != "" {
		shopURL = e.envCfg.URL
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

	if e.executor.Type() == executor.TypeDocker {
		services, _ := devtui.DiscoverServices(cmd.Context(), e.projectRoot)
		if len(services) > 0 {
			fmt.Println(tui.SectionTitleStyle.Render("  Services"))
			for _, svc := range services {
				fmt.Println(tui.DimText.Render("  "+svc.Name+": ") + tui.BoldText.Render(svc.URL))
			}
			fmt.Println()
		}
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

	fmt.Println(tui.GreenText.Bold(true).Render(fmt.Sprintf("  ✓ Development environment stopped in %s", elapsed)))
	fmt.Println()

	return nil
}

func (e *devEnvironment) runTUI() error {
	m := devtui.New(devtui.Options{
		ProjectRoot: e.projectRoot,
		Config:      e.cfg,
		EnvConfig:   e.envCfg,
		Executor:    e.executor,
	})

	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func init() {
	projectRootCmd.AddCommand(projectDevCmd)
	projectDevCmd.AddCommand(projectDevStartCmd)
	projectDevCmd.AddCommand(projectDevStopCmd)
}
