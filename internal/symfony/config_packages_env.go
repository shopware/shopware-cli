package symfony

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/envfile"
)

// Symfony only treats a value as an env reference when the whole string is a
// single %env(...)% expression, so partial matches inside longer strings are
// left untouched.
const (
	envPrefix = "%env("
	envSuffix = ")%"
)

// ResolvedConfig is like Config but additionally resolves %env(...)% references
// in string values against the project's .env files for that environment
// (.env.dist < .env < .env.local < .env.<env> < .env.<env>.local, following
// Symfony precedence). Values that are not env expressions are returned
// unchanged, and an unresolved variable becomes an empty string unless a
// default processor supplies a fallback.
//
// Only string scalars are inspected; a successful cast processor (bool, int,
// json, ...) can change the Go type of the resulting value.
func (pc *ProjectConfig) ResolvedConfig(environment string) (map[string]any, error) {
	cfg, err := pc.Config(environment)
	if err != nil {
		return nil, err
	}

	resolver, err := pc.newEnvResolver(environment, cfg)
	if err != nil {
		return nil, err
	}

	resolved := resolver.resolveValue(cfg)

	return resolved.(map[string]any), nil
}

// GetResolvedConfigValue is GetConfigValue with %env(...)% resolution applied to
// the returned value (and recursively to nested values).
func (pc *ProjectConfig) GetResolvedConfigValue(environment, path string) (any, bool, error) {
	cfg, err := pc.Config(environment)
	if err != nil {
		return nil, false, err
	}

	value, ok, err := getConfigValue(cfg, path)
	if err != nil || !ok {
		return nil, ok, err
	}

	resolver, err := pc.newEnvResolver(environment, cfg)
	if err != nil {
		return nil, false, err
	}

	return resolver.resolveValue(value), true, nil
}

// ResolveEnvExpression resolves a single value the way ResolvedConfig would: if
// it is a %env(...)% expression it is resolved against the project's .env files,
// otherwise it is returned unchanged.
func (pc *ProjectConfig) ResolveEnvExpression(value any) (any, error) {
	resolver, err := pc.newEnvResolver("", nil)
	if err != nil {
		return nil, err
	}

	return resolver.resolveValue(value), nil
}

// envResolver resolves env expressions against a single env map loaded once via
// ProjectConfig.Env. paramDefaults holds the defaults declared as
// `parameters: { env(VAR): value }`, keyed by VAR, used when the variable is
// unset.
type envResolver struct {
	env           map[string]string
	paramDefaults map[string]string
}

// newEnvResolver builds a resolver, loading the project's env files for the
// given environment once. When cfg is provided, env(VAR) defaults declared in
// the top-level parameters block are collected so they can act as fallbacks,
// mirroring Symfony's behaviour.
func (pc *ProjectConfig) newEnvResolver(environment string, cfg map[string]any) (*envResolver, error) {
	env, err := envfile.ReadAllForEnvironment(pc.projectRoot, environment)
	if err != nil {
		return nil, err
	}

	return &envResolver{
		env:           env,
		paramDefaults: collectEnvParamDefaults(cfg),
	}, nil
}

// resolveValue recursively resolves env expressions inside maps, sequences and
// string scalars. Non-string scalars are returned unchanged.
func (r *envResolver) resolveValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = r.resolveValue(v)
		}

		return out
	case map[any]any:
		m, _ := asStringMap(typed)

		return r.resolveValue(m)
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = r.resolveValue(v)
		}

		return out
	case string:
		return r.resolveString(typed)
	default:
		return value
	}
}

// resolveString resolves a string when it is exactly an %env(...)% expression.
func (r *envResolver) resolveString(value string) any {
	inner, ok := envExpressionInner(value)
	if !ok {
		return value
	}

	processors, varName := splitProcessors(inner)

	raw, found := r.lookup(varName)

	resolved, err := applyProcessors(processors, raw, found, r)
	if err != nil {
		// On a processing error keep the original expression rather than
		// silently dropping configuration.
		return value
	}

	return resolved
}

// lookup returns the value of an environment variable, falling back to a
// declared env(VAR) parameter default. The bool reports whether a value (env or
// default) was found.
func (r *envResolver) lookup(varName string) (string, bool) {
	if v := r.env[varName]; v != "" {
		return v, true
	}

	if def, ok := r.paramDefaults[varName]; ok {
		return def, true
	}

	return "", false
}

// envExpressionInner returns the inner content of a %env(...)% expression when
// value is exactly such an expression.
func envExpressionInner(value string) (string, bool) {
	if !strings.HasPrefix(value, envPrefix) || !strings.HasSuffix(value, envSuffix) {
		return "", false
	}

	inner := value[len(envPrefix) : len(value)-len(envSuffix)]
	if inner == "" {
		return "", false
	}

	return inner, true
}

// splitProcessors splits the inner part of an env expression into its processor
// chain and the variable name. For "int:default:0:PORT" it returns
// (["int", "default", "0"], "PORT"). The variable name is always the last
// colon-separated segment.
func splitProcessors(inner string) ([]string, string) {
	parts := strings.Split(inner, ":")
	if len(parts) == 1 {
		return nil, parts[0]
	}

	return parts[:len(parts)-1], parts[len(parts)-1]
}

// applyProcessors applies the processor chain (right-most processor closest to
// the variable is applied first) to the raw variable value.
func applyProcessors(processors []string, raw string, found bool, r *envResolver) (any, error) {
	// Handle a leading default processor specially: it can supply a value when
	// the variable is missing and consumes the following segment as the
	// fallback parameter name.
	if len(processors) >= 2 && processors[0] == "default" {
		fallbackParam := processors[1]
		remaining := processors[2:]

		if !found {
			if fallbackParam == "" {
				// %env(default::VAR)% with no fallback resolves to null.
				return nil, nil //nolint:nilnil // nil is the resolved value (YAML null), not an error
			}

			if def, ok := r.paramDefaults[fallbackParam]; ok {
				raw = def
				found = true
			}
		}

		return applyCasts(remaining, raw, found)
	}

	return applyCasts(processors, raw, found)
}

// applyCasts applies the type/decoding processors (no default handling) from the
// outermost to the innermost. Symfony applies the chain left-to-right where the
// left-most processor is the last transformation, so we walk the slice in
// reverse.
func applyCasts(processors []string, raw string, found bool) (any, error) {
	var current any = raw

	for i := len(processors) - 1; i >= 0; i-- {
		casted, err := applyCast(processors[i], current)
		if err != nil {
			return nil, err
		}

		current = casted
	}

	if !found {
		if _, ok := current.(string); ok && current == "" {
			return "", nil
		}
	}

	return current, nil
}

// applyCast applies a single processor to value.
func applyCast(processor string, value any) (any, error) {
	str := fmt.Sprintf("%v", value)

	switch processor {
	case "string":
		return str, nil
	case "trim":
		return strings.TrimSpace(str), nil
	case "bool":
		return parseEnvBool(str), nil
	case "not":
		return !parseEnvBool(str), nil
	case "int":
		n, err := strconv.Atoi(strings.TrimSpace(str))
		if err != nil {
			return nil, fmt.Errorf("env processor int: %w", err)
		}

		return n, nil
	case "float":
		f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
		if err != nil {
			return nil, fmt.Errorf("env processor float: %w", err)
		}

		return f, nil
	case "json":
		var out any
		if strings.TrimSpace(str) == "" {
			// json of an empty string resolves to null, matching Symfony.
			return nil, nil //nolint:nilnil // nil is the resolved value (YAML null), not an error
		}
		if err := json.Unmarshal([]byte(str), &out); err != nil {
			return nil, fmt.Errorf("env processor json: %w", err)
		}

		return out, nil
	case "csv":
		return parseEnvCSV(str)
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimRight(str, "="))
		if err != nil {
			// Fall back to padded decoding before failing.
			decoded, err = base64.StdEncoding.DecodeString(str)
			if err != nil {
				return nil, fmt.Errorf("env processor base64: %w", err)
			}
		}

		return string(decoded), nil
	default:
		// Unsupported processors (file, require, resolve, ...) are left as a
		// best-effort pass-through of the underlying value.
		return value, nil
	}
}

// parseEnvBool mirrors Symfony's bool casting: 'true', 'on', 'yes' and non-zero
// numbers are true; everything else is false.
func parseEnvBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "on", "yes", "1":
		return true
	case "false", "off", "no", "0", "":
		return false
	}

	if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		return f != 0
	}

	return false
}

// parseEnvCSV decodes a single CSV line into a slice of strings.
func parseEnvCSV(value string) ([]any, error) {
	if strings.TrimSpace(value) == "" {
		return []any{}, nil
	}

	reader := csv.NewReader(strings.NewReader(value))

	record, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("env processor csv: %w", err)
	}

	out := make([]any, len(record))
	for i, field := range record {
		out[i] = field
	}

	return out, nil
}

// collectEnvParamDefaults scans every package's parameters block for env(VAR)
// keys, which declare default values for environment variables. Symfony
// declares them in the top-level parameters: section, which the merged config
// exposes as the "parameters" key.
func collectEnvParamDefaults(cfg map[string]any) map[string]string {
	defaults := map[string]string{}

	params, ok := asStringMap(cfg["parameters"])
	if !ok {
		return defaults
	}

	for key, value := range params {
		varName, ok := envParamKey(key)
		if !ok {
			continue
		}

		if str, ok := value.(string); ok {
			defaults[varName] = str
		}
	}

	return defaults
}

// envParamKey extracts VAR from an "env(VAR)" parameter key.
func envParamKey(key string) (string, bool) {
	if rest, ok := strings.CutPrefix(key, "env("); ok {
		if varName, ok := strings.CutSuffix(rest, ")"); ok && varName != "" {
			return varName, true
		}
	}

	return "", false
}
