package extension

import (
	"context"
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
	Tables map[string]ColumnSQLMap
	// SystemConfig anonymizes system_config rows. Keys are sorted.
	SystemConfig []shop.SystemConfigRule
}

type columnOwner struct {
	extension  string
	expression string
	jsonColumn string
	jsonPaths  []string
	// MySQL applies JSON_REMOVE paths left to right, so repeated inputs must
	// not remove another array element after earlier removals shift its index.
	inputs map[string]struct{}
}

type systemConfigOwner struct {
	extension string
	rule      shop.SystemConfigRule
}

// CollectAnonymization reads the anonymize section of every extension in the
// Shopware project and merges them in name order. JSON_REMOVE rules on the same
// column are combined; for other conflicts the first extension wins with a warning.
// An unreadable extension config or a Composer-listed extension that cannot be
// loaded is an error, so no rules are skipped silently. Folders under custom/
// that are not extensions are ignored.
func CollectAnonymization(ctx context.Context, project string) (AnonymizationRules, error) {
	extensions, err := FindExtensionsFromProject(ctx, project, false)
	if err != nil {
		return AnonymizationRules{}, fmt.Errorf("cannot collect anonymize rules from extensions: %w", err)
	}

	rules := AnonymizationRules{
		Tables:       collectTableAnonymization(ctx, extensions),
		SystemConfig: collectSystemConfigAnonymization(ctx, extensions),
	}

	if len(rules.Tables) > 0 || len(rules.SystemConfig) > 0 {
		logging.FromContext(ctx).Infof(
			"Merging anonymization rules from extensions: %d tables, %d system config keys",
			len(rules.Tables),
			len(rules.SystemConfig),
		)
	}

	return rules, nil
}

// collectTableAnonymization combines column rewrites in extension discovery order.
func collectTableAnonymization(ctx context.Context, extensions []Extension) map[string]ColumnSQLMap {
	owners := map[string]map[string]*columnOwner{}
	for _, ext := range extensions {
		cfg := ext.GetExtensionConfig()
		if cfg == nil {
			continue
		}

		name, _ := ext.GetName()
		for table, columns := range cfg.Anonymize.Tables {
			table = strings.ToLower(table)
			if owners[table] == nil {
				owners[table] = map[string]*columnOwner{}
			}
			for column, expression := range columns {
				column = strings.ToLower(column)
				if owner, exists := owners[table][column]; exists {
					if !owner.merge(column, expression) {
						logging.FromContext(ctx).Warnf(
							"Extension %s rewrites %s.%s, keeping the anonymization rule from %s",
							name, table, column, owner.extension,
						)
					}
					continue
				}

				jsonColumn, paths := parseJSONRemove(column, expression)
				owners[table][column] = &columnOwner{
					extension: name, expression: expression,
					jsonColumn: jsonColumn, jsonPaths: paths,
					inputs: map[string]struct{}{expression: {}},
				}
			}
		}
	}

	tables := map[string]ColumnSQLMap{}
	for table, columns := range owners {
		tables[table] = ColumnSQLMap{}
		for column, owner := range columns {
			expression := owner.expression
			if len(owner.jsonPaths) > 1 {
				expression = "JSON_REMOVE(" + owner.jsonColumn + ", " + strings.Join(owner.jsonPaths, ", ") + ")"
			}
			tables[table][column] = expression
		}
	}
	return tables
}

// merge collects compatible JSON_REMOVE arguments without reparsing prior inputs.
func (o *columnOwner) merge(column, expression string) bool {
	if _, exists := o.inputs[expression]; exists {
		return true
	}
	_, paths := parseJSONRemove(column, expression)
	if len(o.jsonPaths) == 0 || len(paths) == 0 {
		return false
	}
	// An extension may repeat the complete set accumulated from earlier rules.
	if paths[0] != strings.Join(o.jsonPaths, ", ") {
		o.jsonPaths = append(o.jsonPaths, paths...)
	}
	o.inputs[expression] = struct{}{}
	return true
}

// parseJSONRemove keeps path arguments intact, including quoted commas and escapes.
func parseJSONRemove(column, expression string) (string, []string) {
	match := jsonRemovePattern.FindStringSubmatch(expression)
	if match == nil || !strings.EqualFold(match[2], column) {
		return "", nil
	}
	return match[1], []string{strings.TrimSpace(match[3])}
}

// collectSystemConfigAnonymization keeps the first rule for each configuration key.
func collectSystemConfigAnonymization(ctx context.Context, extensions []Extension) []shop.SystemConfigRule {
	seen := map[string]systemConfigOwner{}
	var rules []shop.SystemConfigRule
	for _, ext := range extensions {
		cfg := ext.GetExtensionConfig()
		if cfg == nil {
			continue
		}

		name, _ := ext.GetName()
		for _, entry := range cfg.Anonymize.SystemConfig {
			rule, err := entry.DumpRule()
			if err != nil {
				logging.FromContext(ctx).Warnf("Extension %s has an invalid system_config anonymization rule: %s", name, err)
				continue
			}
			if previous, exists := seen[rule.Key]; exists {
				if !previous.rule.Equal(rule) {
					logging.FromContext(ctx).Warnf(
						"Extension %s anonymizes system_config %s, keeping the rule from %s",
						name, rule.Key, previous.extension,
					)
				}
				continue
			}
			seen[rule.Key] = systemConfigOwner{extension: name, rule: rule}
			rules = append(rules, rule)
		}
	}

	slices.SortFunc(rules, func(a, b shop.SystemConfigRule) int {
		return strings.Compare(a.Key, b.Key)
	})
	return rules
}
