package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

// SelectEnvironment prompts for an explicitly configured deployment environment.
func SelectEnvironment(ctx context.Context, root, configPath string, input io.Reader, output io.Writer) (string, error) {
	if !system.IsInteractionEnabled(ctx) {
		return "", errors.New("cannot select deployment environment without interaction: pass --env")
	}
	if configPath == "" {
		configPath = shop.SearchConfigPath(ctx, root, "")
	}
	cfg, err := shop.ReadConfig(ctx, configPath, false)
	if err != nil {
		return "", err
	}
	return selectEnvironment(ctx, cfg, func(names []string) (string, error) {
		selected := names[0]
		options := make([]huh.Option[string], 0, len(names))
		for _, name := range names {
			options = append(options, huh.NewOption(name, name))
		}
		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Choose a deployment environment").Options(options...).Value(&selected),
		)).WithTheme(tui.ShopwareTheme()).WithInput(input).WithOutput(output).RunWithContext(ctx)
		return selected, err
	})
}

func selectEnvironment(ctx context.Context, cfg *shop.Config, pick func([]string) (string, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	names := make([]string, 0, len(cfg.Environments))
	for name, env := range cfg.Environments {
		if env != nil && constructors[env.Type] != nil {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "", errors.New("cannot select deployment environment: no configured environments support deployments")
	}
	selected, err := pick(names)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("cannot select deployment environment: %w", err)
	}
	return selected, nil
}
