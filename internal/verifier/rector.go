package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/shop"
)

type Rector struct{}

func (r Rector) Name() string {
	return "rector"
}

func (r Rector) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}

	// Fix against the minimum compatible dependency set, independently of the
	// extension's locally installed dependencies. Projects keep their vendor/.
	config.CheckAgainst = "lowest"
	autoloadFile, toolErr := prepareComposerAutoload(ctx, config, execution)
	if toolErr != nil {
		execution.Fail(toolErr)
		return ToolSummary{}
	}

	var rectorConfigFile string
	rulesNote := "custom rules from rector.php"

	if _, err := os.Stat(path.Join(config.RootDir, "rector.php")); err == nil {
		rectorConfigFile = path.Join(config.RootDir, "rector.php")
	} else {
		if len(config.MinShopwareVersion) < 3 {
			execution.Fail(&ToolError{Err: errors.New("cannot select rector configuration: invalid Shopware version")})
			return ToolSummary{}
		}
		rulesNote = "bundled rules for Shopware " + config.MinShopwareVersion[0:3]
		rectorConfigFile = path.Join(config.ToolDirectory, "php", "vendor", "frosh", "shopware-rector", "config", fmt.Sprintf("shopware-%s.0.php", config.MinShopwareVersion[0:3]))
	}

	result := ToolSummary{Note: rulesNote}
	if installed := shop.DetectShopwareVersion(path.Dir(path.Dir(autoloadFile))); installed != "" {
		result.Note = "Shopware " + installed
		if !config.IsProject {
			result.Note += " (lowest)"
		}
		result.Note += "; " + rulesNote
	}

	for _, sourceDirectory := range config.SourceDirectories {
		rector := CommandContext(ctx, "php", "-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "rector"), "process", "--config", rectorConfigFile, "--autoload-file", autoloadFile, sourceDirectory)
		rector.Dir = path.Dir(path.Dir(autoloadFile))
		if subprocess := execution.Run(OperationFix, rector); subprocess.Err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run rector in %s: %w", sourceDirectory, subprocess.Err)))
		}
	}

	return result
}

func init() {
	AddTool(Rector{})
}
