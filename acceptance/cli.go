package acceptance

import (
	"context"
	"os"

	"github.com/shopware/shopware-cli/cmd"
	"github.com/shopware/shopware-cli/internal/extension"
)

// runCLI is what the scripts execute: main, pointed at the harness's local Packagist.
// It lives outside the test files so coverage runs instrument this package.
func runCLI() {
	if url := os.Getenv("ACCEPTANCE_PACKAGIST_URL"); url != "" {
		extension.PackagistURL = url
	}

	os.Exit(cmd.Execute(context.Background()))
}
