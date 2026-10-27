package deployment

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopware/shopware-cli/logging"
)

var _ Initializer = (*sshGroup)(nil)

// InitializeDeployment prompts for configuration and initializes the SSH deployment target.
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
		var err error
		states[i], err = host.backend.initStore.Inspect(ctx)
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
	if !state.Initialized {
		config, confirmed, err := prompt(ctx, state)
		if err != nil {
			return err
		}
		if !confirmed {
			return ErrInitializationCanceled
		}
		host := g.hosts[g.migration]
		if err := host.backend.initStore.Apply(ctx, config); err != nil {
			return fmt.Errorf("host %s: configuration may already have been written: %w", host.name, err)
		}
	}
	// Compare file contents, not mount identity. Completed initialization must
	// still be verified on repeat runs, without prompting or writing again.
	if err := g.parallel(ctx, func(i int) error {
		var err error
		states[i], err = g.hosts[i].backend.initStore.Inspect(ctx)
		if err != nil {
			return err
		}
		if !states[i].HasValidRuntimeConfig() {
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
	if err := errors.Join(mismatches...); err != nil {
		return err
	}
	if state.Initialized {
		logging.FromContext(ctx).Info("Deployment is already initialized")
	}
	return nil
}
