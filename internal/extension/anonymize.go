package extension

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

// anonymizeIdentifierPattern matches Shopware table and column names.
var anonymizeIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// AnonymizationRules are the column rewrites and system config keys declared
// by extensions installed in a project.
type AnonymizationRules struct {
	// Tables is table -> column -> SQL expression. Keys are lowercase.
	Tables map[string]map[string]string
	// SystemConfig lists system_config.configuration_key values to clear.
	SystemConfig []string
}

type columnOwner struct {
	extension  string
	expression string
}

// CollectAnonymization reads the anonymize section of every extension in the
// Shopware project and merges them. Extensions are visited in name order.
// When two extensions rewrite the same column with different expressions, the
// first extension is kept and a warning is logged.
func CollectAnonymization(ctx context.Context, project string) AnonymizationRules {
	extensions := FindExtensionsFromProject(ctx, project, false)
	slices.SortFunc(extensions, func(a, b Extension) int {
		return strings.Compare(extensionSortName(a), extensionSortName(b))
	})

	rules := AnonymizationRules{
		Tables: map[string]map[string]string{},
	}
	owners := map[string]columnOwner{}
	seenKeys := map[string]struct{}{}

	for _, ext := range extensions {
		cfg := ext.GetExtensionConfig()
		if cfg == nil {
			continue
		}

		name := extensionSortName(ext)
		for table, columns := range cfg.Anonymize.Tables {
			table = strings.ToLower(table)
			for column, expression := range columns {
				column = strings.ToLower(column)
				key := table + "." + column
				if owner, exists := owners[key]; exists {
					if owner.expression != expression {
						logging.FromContext(ctx).Warnf(
							"Extension %s rewrites %s.%s, keeping the anonymization rule from %s",
							name,
							table,
							column,
							owner.extension,
						)
					}
					continue
				}

				owners[key] = columnOwner{extension: name, expression: expression}
				if rules.Tables[table] == nil {
					rules.Tables[table] = map[string]string{}
				}
				rules.Tables[table][column] = expression
			}
		}

		for _, key := range cfg.Anonymize.SystemConfig {
			if _, exists := seenKeys[key]; exists {
				continue
			}
			seenKeys[key] = struct{}{}
			rules.SystemConfig = append(rules.SystemConfig, key)
		}
	}

	slices.Sort(rules.SystemConfig)

	if len(rules.Tables) > 0 || len(rules.SystemConfig) > 0 {
		logging.FromContext(ctx).Infof(
			"Merging anonymization rules from extensions: %d tables, %d system config keys",
			len(rules.Tables),
			len(rules.SystemConfig),
		)
	}

	return rules
}

func extensionSortName(ext Extension) string {
	name, err := ext.GetName()
	if err != nil || name == "" {
		return ext.GetPath()
	}

	return name
}
