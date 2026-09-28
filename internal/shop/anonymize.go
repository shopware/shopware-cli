package shop

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// systemConfigKeyPattern matches a system_config.configuration_key.
// Keys are interpolated into a SQL string literal, so the pattern excludes quotes.
var systemConfigKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// ValidateSystemConfigKey reports whether key can be used in an anonymized dump.
func ValidateSystemConfigKey(key string) error {
	if !systemConfigKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid system_config key %q", key)
	}

	return nil
}

// SystemConfigRewriteExpression builds the SQL expression that clears
// configuration_value for the given system_config.configuration_key values.
// Matching rows are dumped as {"_value": null}. An empty key list returns an empty expression.
func SystemConfigRewriteExpression(keys []string) (string, error) {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := ValidateSystemConfigKey(key); err != nil {
			return "", err
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}

	if len(unique) == 0 {
		return "", nil
	}

	slices.Sort(unique)
	quoted := make([]string, len(unique))
	for i, key := range unique {
		quoted[i] = "'" + key + "'"
	}

	return "CASE WHEN `configuration_key` IN (" + strings.Join(quoted, ", ") + ") THEN '{\"_value\":null}' ELSE `configuration_value` END", nil
}

// MergeRewrite adds column rewrites that are not already present.
// Existing values, including project dump.rewrite entries, take precedence.
// Incoming table and column names are matched case-insensitively.
func (c *ConfigDump) MergeRewrite(rewrites map[string]map[string]string) {
	if c == nil || len(rewrites) == 0 {
		return
	}

	if c.Rewrite == nil {
		c.Rewrite = make(map[string]map[string]string, len(rewrites))
	}

	for table, columns := range rewrites {
		table = strings.ToLower(strings.TrimSpace(table))
		if table == "" {
			continue
		}

		dst, exists := c.Rewrite[table]
		if !exists {
			dst = make(map[string]string, len(columns))
			c.Rewrite[table] = dst
		}

		for column, expression := range columns {
			column = strings.ToLower(strings.TrimSpace(column))
			if column == "" {
				continue
			}
			if _, columnExists := dst[column]; columnExists {
				continue
			}
			dst[column] = expression
		}
	}
}

// hasColumnRewrite reports whether table.column is already rewritten.
// Matching ignores ASCII case so a project rewrite is recognized whatever casing it uses.
func (c *ConfigDump) hasColumnRewrite(table, column string) bool {
	if c == nil || c.Rewrite == nil {
		return false
	}

	for candidate, cols := range c.Rewrite {
		if !strings.EqualFold(candidate, table) {
			continue
		}
		for name := range cols {
			if strings.EqualFold(name, column) {
				return true
			}
		}
	}

	return false
}

// forceRewrite sets one column rewrite, replacing any previous expression.
func (c *ConfigDump) forceRewrite(table, column, expression string) {
	if c.Rewrite == nil {
		c.Rewrite = map[string]map[string]string{}
	}

	table = strings.ToLower(table)
	column = strings.ToLower(column)
	if c.Rewrite[table] == nil {
		c.Rewrite[table] = map[string]string{}
	}
	c.Rewrite[table][column] = expression
}
