package deployment

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

var _ Initializer = (*sshGroup)(nil)

func (g *sshGroup) InitializeDeployment(ctx context.Context) error {
	return g.initializeDeployment(ctx, g.hosts[g.migration].backend.promptDeploymentInitialization)
}

// Initialization uses each host's ordinary deployment lock during inspection
// and the migration host's lock during the single write. It is not a group
// transaction: a later verification failure does not undo published config.
func (g *sshGroup) initializeDeployment(ctx context.Context, prompt func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error)) error {
	states := make([]sshDeploymentInitState, len(g.hosts))
	if err := g.parallel(ctx, func(i int) error {
		host := g.hosts[i]
		files, err := host.backend.sharedFiles()
		if err != nil {
			return err
		}
		if !slices.Contains(files, ".env.local") {
			return errors.New(`SSH deployment initialization requires ".env.local" in ssh.shared.files`)
		}
		states[i], err = host.backend.inspectDeploymentInitialization(ctx)
		if err != nil {
			return err
		}
		if !states[i].HasSharedDirectory {
			return errors.New("provision the shared directory before initializing an SSH group")
		}
		if i != g.migration && states[i].HasInstallConfig {
			return errors.New("pending .shopware-cli/install.env on a non-migration host; resolve it before initializing the group")
		}
		return nil
	}); err != nil {
		return err
	}
	state := states[g.migration]
	for _, member := range states {
		state.HasCurrent = state.HasCurrent || member.HasCurrent
	}
	config, confirmed, err := prompt(ctx, state)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	if len(config.RuntimeValues) != 0 || len(config.InstallValues) != 0 {
		host := g.hosts[g.migration]
		if err := host.backend.applyDeploymentInitialization(ctx, config); err != nil {
			return fmt.Errorf("host %s: configuration may already have been written: %w", host.name, err)
		}
	}
	// Compare file contents, not mount identity. Even unchanged configuration
	// must be visible consistently before reporting a successful initialization.
	if err := g.parallel(ctx, func(i int) error {
		var err error
		states[i], err = g.hosts[i].backend.inspectDeploymentInitialization(ctx)
		if err != nil {
			return err
		}
		if !states[i].HasRuntimeConfig || states[i].RuntimeSHA256 == "" {
			return errors.New("shared runtime configuration is not visible after initialization")
		}
		if i != g.migration && states[i].HasInstallConfig {
			return errors.New("pending .shopware-cli/install.env on a non-migration host after initialization")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("verify SSH group initialization (configuration may already have been written): %w", err)
	}
	var mismatches []error
	for i, member := range states {
		if member.RuntimeSHA256 != states[g.migration].RuntimeSHA256 {
			mismatches = append(mismatches, fmt.Errorf("host %s: shared runtime configuration differs from migration host %s; configuration may already have been written", g.hosts[i].name, g.hosts[g.migration].name))
		}
	}
	return errors.Join(mismatches...)
}
