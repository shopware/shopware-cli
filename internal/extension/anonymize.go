package extension

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// anonymizeIdentifierPattern matches Shopware table and column names.
var anonymizeIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// jsonRemovePattern matches JSON_REMOVE(column, paths) and captures the column and the paths.
var jsonRemovePattern = regexp.MustCompile("(?is)^\\s*JSON_REMOVE\\s*\\(\\s*(`?([A-Za-z_][A-Za-z0-9_]*)`?)\\s*,(.+)\\)\\s*$")

// AnonymizationRules are the column rewrites and system config keys declared
// by extensions installed in a project.
type AnonymizationRules struct {
	// Tables is table -> column -> SQL expression. Keys are lowercase.
	Tables map[string]map[string]string
	// SystemConfig anonymizes system_config rows. Keys are sorted.
	SystemConfig []shop.SystemConfigRule
}

type columnOwner struct {
	extension  string
	expression string
}

type systemConfigOwner struct {
	extension string
	rule      shop.SystemConfigRule
}

// CollectAnonymization reads the anonymize section of every extension in the
// Shopware project and merges them in name order. JSON_REMOVE rules on the same
// column are combined; for other conflicts the first extension wins with a warning.
// An unreadable extension config is an error, so no rules are skipped silently.
func CollectAnonymization(ctx context.Context, project string) (AnonymizationRules, error) {
	var configErrors []error
	seenPaths := map[string]bool{}
	extensions := findExtensionsFromProject(ctx, project, false, func(err error) {
		var configErr *ConfigError
		if errors.As(err, &configErr) && !seenPaths[configErr.Path] {
			seenPaths[configErr.Path] = true
			configErrors = append(configErrors, err)
		}
	})
	if len(configErrors) > 0 {
		return AnonymizationRules{}, fmt.Errorf("cannot collect anonymize rules from extensions: %w", errors.Join(configErrors...))
	}

	slices.SortFunc(extensions, func(a, b Extension) int {
		return strings.Compare(extensionSortName(a), extensionSortName(b))
	})

	rules := AnonymizationRules{
		Tables: map[string]map[string]string{},
	}
	owners := map[string]columnOwner{}
	seenConfig := map[string]systemConfigOwner{}

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
					if owner.expression == expression {
						continue
					}
					if merged, ok := mergeJSONRemove(column, owner.expression, expression); ok {
						owners[key] = columnOwner{extension: owner.extension, expression: merged}
						rules.Tables[table][column] = merged
						continue
					}
					logging.FromContext(ctx).Warnf(
						"Extension %s rewrites %s.%s, keeping the anonymization rule from %s",
						name,
						table,
						column,
						owner.extension,
					)
					continue
				}

				owners[key] = columnOwner{extension: name, expression: expression}
				if rules.Tables[table] == nil {
					rules.Tables[table] = map[string]string{}
				}
				rules.Tables[table][column] = expression
			}
		}

		for _, entry := range cfg.Anonymize.SystemConfig {
			rule, err := entry.DumpRule()
			if err != nil {
				logging.FromContext(ctx).Warnf("Extension %s has an invalid system_config anonymization rule: %s", name, err)
				continue
			}
			if previous, exists := seenConfig[rule.Key]; exists {
				if !previous.rule.Equal(rule) {
					logging.FromContext(ctx).Warnf(
						"Extension %s anonymizes system_config %s, keeping the rule from %s",
						name,
						rule.Key,
						previous.extension,
					)
				}
				continue
			}
			seenConfig[rule.Key] = systemConfigOwner{extension: name, rule: rule}
			rules.SystemConfig = append(rules.SystemConfig, rule)
		}
	}

	slices.SortFunc(rules.SystemConfig, func(a, b shop.SystemConfigRule) int {
		return strings.Compare(a.Key, b.Key)
	})

	if len(rules.Tables) > 0 || len(rules.SystemConfig) > 0 {
		logging.FromContext(ctx).Infof(
			"Merging anonymization rules from extensions: %d tables, %d system config keys",
			len(rules.Tables),
			len(rules.SystemConfig),
		)
	}

	return rules, nil
}

// mergeJSONRemove combines two JSON_REMOVE rules on the same column into one, so both remove their keys.
func mergeJSONRemove(column, existing, incoming string) (string, bool) {
	current := jsonRemovePattern.FindStringSubmatch(existing)
	next := jsonRemovePattern.FindStringSubmatch(incoming)
	if current == nil || next == nil || !strings.EqualFold(current[2], column) || !strings.EqualFold(next[2], column) {
		return "", false
	}

	return "JSON_REMOVE(" + current[1] + ", " + strings.TrimSpace(current[3]) + ", " + strings.TrimSpace(next[3]) + ")", true
}

func extensionSortName(ext Extension) string {
	name, err := ext.GetName()
	if err != nil || name == "" {
		return ext.GetPath()
	}

	return name
}
