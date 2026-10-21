package shop

import (
	"encoding/json"
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

// SystemConfigRule is one extension-declared system_config.configuration_key action.
// Omit drops the row. Faker stores {"_value":"<generated>"}. HasValue stores
// {"_value": Value}, including an explicit null.
type SystemConfigRule struct {
	Key      string
	Omit     bool
	Faker    string
	HasValue bool
	Value    any
}

// NewSystemConfigRule validates a system config anonymization rule.
// omit with no value drops the row. A string value that starts with faker.
// is evaluated per dumped row. Any other value is stored as JSON.
func NewSystemConfigRule(key string, omit bool, hasValue bool, value any) (SystemConfigRule, error) {
	key = strings.TrimSpace(key)
	if err := ValidateSystemConfigKey(key); err != nil {
		return SystemConfigRule{}, err
	}
	if omit && hasValue {
		return SystemConfigRule{}, fmt.Errorf("system_config key %q sets omit and value", key)
	}
	if omit || !hasValue {
		return SystemConfigRule{Key: key, Omit: true}, nil
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "faker.") {
			return SystemConfigRule{Key: key, Faker: text}, nil
		}
	}

	return SystemConfigRule{Key: key, HasValue: true, Value: value}, nil
}

// BuildSystemConfigAnonymization returns the configuration_value rewrite and
// the WHERE fragment that drops omitted keys. Both are empty when rules is empty.
func BuildSystemConfigAnonymization(rules []SystemConfigRule) (rewrite string, where string, err error) {
	omitKeys := make([]string, 0)
	replacements := make([]SystemConfigRule, 0)
	seen := map[string]string{}

	for _, rule := range rules {
		if err := ValidateSystemConfigKey(rule.Key); err != nil {
			return "", "", err
		}
		fingerprint, err := rule.fingerprint()
		if err != nil {
			return "", "", err
		}
		if previous, exists := seen[rule.Key]; exists {
			if previous != fingerprint {
				return "", "", fmt.Errorf("system_config key %q has conflicting anonymization rules", rule.Key)
			}
			continue
		}
		seen[rule.Key] = fingerprint
		if rule.Omit {
			omitKeys = append(omitKeys, rule.Key)
			continue
		}
		replacements = append(replacements, rule)
	}

	slices.Sort(omitKeys)
	slices.SortFunc(replacements, func(a, b SystemConfigRule) int {
		return strings.Compare(a.Key, b.Key)
	})

	if len(omitKeys) > 0 {
		quoted := make([]string, len(omitKeys))
		for i, key := range omitKeys {
			quoted[i] = "'" + key + "'"
		}
		where = "`configuration_key` NOT IN (" + strings.Join(quoted, ", ") + ")"
	}

	if len(replacements) == 0 {
		return "", where, nil
	}

	var builder strings.Builder
	builder.WriteString("CASE")
	for _, rule := range replacements {
		literal, err := rule.valueLiteral()
		if err != nil {
			return "", "", err
		}
		builder.WriteString(" WHEN `configuration_key` = '")
		builder.WriteString(rule.Key)
		builder.WriteString("' THEN ")
		builder.WriteString(mysqlStringLiteral(literal))
	}
	builder.WriteString(" ELSE `configuration_value` END")

	return builder.String(), where, nil
}

// Equal reports whether both rules anonymize a key in the same way.
func (r SystemConfigRule) Equal(other SystemConfigRule) bool {
	left, leftErr := r.fingerprint()
	right, rightErr := other.fingerprint()
	return leftErr == nil && rightErr == nil && left == right
}

func (r SystemConfigRule) fingerprint() (string, error) {
	if r.Omit {
		return "omit", nil
	}
	if r.Faker != "" {
		return "faker:" + r.Faker, nil
	}
	if !r.HasValue {
		return "", fmt.Errorf("system_config key %q needs omit or a value", r.Key)
	}
	literal, err := r.valueLiteral()
	if err != nil {
		return "", err
	}
	return "value:" + literal, nil
}

func (r SystemConfigRule) valueLiteral() (string, error) {
	var payload any
	if r.Faker != "" {
		payload = map[string]string{"_value": "{{- " + r.Faker + " -}}"}
	} else {
		payload = map[string]any{"_value": r.Value}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("system_config key %q: %w", r.Key, err)
	}

	return string(encoded), nil
}

func mysqlStringLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "'", "''")
	return "'" + value + "'"
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

// mergeWhere ANDs condition onto an existing WHERE for table.
// Matching ignores ASCII case. A new condition is stored under the lowercase table name.
func (c *ConfigDump) mergeWhere(table, condition string) {
	if c == nil || condition == "" {
		return
	}
	if c.Where == nil {
		c.Where = map[string]string{}
	}

	for existing, current := range c.Where {
		if strings.EqualFold(existing, table) {
			c.Where[existing] = "(" + current + ") AND (" + condition + ")"
			return
		}
	}

	c.Where[strings.ToLower(table)] = condition
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
