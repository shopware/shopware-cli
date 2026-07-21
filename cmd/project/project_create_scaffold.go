package project

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
)

func scaffoldProject(ctx context.Context, opts *createOptions, chosenVersion string) error {
	go tracking.Track(ctx, tracking.EventProjectCreate, map[string]string{
		tracking.TagVersion:           opts.selectedVersion,
		tracking.TagDeployment:        opts.selectedDeployment,
		tracking.TagCI:                opts.selectedCI,
		tracking.TagDocker:            fmt.Sprintf("%v", opts.useDocker),
		tracking.TagWithElasticsearch: fmt.Sprintf("%v", opts.withElasticsearch),
		tracking.TagWithAMQP:          fmt.Sprintf("%v", opts.withAMQP),
		tracking.TagInteractive:       fmt.Sprintf("%v", opts.interactive),
	})

	scaffold := newShopwareProjectScaffold(opts, chosenVersion)
	scaffold.SymfonyCLIInstalled = !opts.useDocker && system.IsSymfonyCliInstalled()

	return scaffold.Scaffold(ctx)
}

func newShopwareProjectScaffold(opts *createOptions, chosenVersion string) shop.ShopwareProjectScaffold {
	return shop.ShopwareProjectScaffold{
		ProjectFolder:    opts.projectFolder,
		Version:          chosenVersion,
		DeploymentMethod: opts.selectedDeployment,
		CISystem:         opts.selectedCI,
		UseDocker:        opts.useDocker,
		UseElasticsearch: opts.withElasticsearch,
		UseAMQP:          opts.withAMQP,
		NoAudit:          opts.noAudit,
	}
}
