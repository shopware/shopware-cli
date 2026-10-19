package project

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/logging"
)

func scaffoldProject(ctx context.Context, opts *createOptions, chosenVersion string) error {
	go tracking.Track(ctx, tracking.EventProjectCreate, map[string]string{
		tracking.TagVersion:           opts.selectedVersion,
		tracking.TagDeployment:        opts.selectedDeployment,
		tracking.TagCI:                opts.selectedCI,
		tracking.TagDocker:            strconv.FormatBool(opts.useDocker),
		tracking.TagWithElasticsearch: strconv.FormatBool(opts.withElasticsearch),
		tracking.TagWithAMQP:          strconv.FormatBool(opts.withAMQP),
		tracking.TagInteractive:       strconv.FormatBool(opts.interactive),
	})

	scaffold := newShopwareProjectScaffold(opts, chosenVersion)
	scaffold.SymfonyCLIInstalled = !opts.useDocker && system.IsSymfonyCliInstalled()

	return scaffold.Scaffold(ctx)
}

// rollbackFailedProjectCreation removes a partially created project after a
// failed or cancelled creation and returns the original error, joined with
// the cleanup error when the rollback itself fails.
func rollbackFailedProjectCreation(ctx context.Context, opts *createOptions, existedBefore bool, cause error) error {
	if err := shop.CleanupFailedProjectCreation(opts.projectFolder, existedBefore); err != nil {
		return errors.Join(cause, err)
	}

	displayFolder := opts.projectFolder
	if absFolder, err := filepath.Abs(opts.projectFolder); err == nil {
		displayFolder = absFolder
	}
	if existedBefore {
		logging.FromContext(ctx).Infof("Removed partially created project files in %s", displayFolder)
	} else {
		logging.FromContext(ctx).Infof("Removed partially created project in %s", displayFolder)
	}

	return cause
}

func newShopwareProjectScaffold(opts *createOptions, chosenVersion string) shop.ShopwareProjectScaffold {
	return shop.ShopwareProjectScaffold{
		ProjectFolder:      opts.projectFolder,
		Version:            chosenVersion,
		DeploymentMethod:   opts.selectedDeployment,
		CISystem:           opts.selectedCI,
		PHPVersion:         opts.phpVersion,
		UseDocker:          opts.useDocker,
		UseElasticsearch:   opts.withElasticsearch,
		UseAMQP:            opts.withAMQP,
		NoAudit:            opts.noAudit,
		ComposeProjectName: opts.composeProjectName,
	}
}
