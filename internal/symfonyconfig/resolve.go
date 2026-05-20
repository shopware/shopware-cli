package symfonyconfig

import (
	"regexp"
	"strings"
)

// placeholderRe matches Symfony parameter / env placeholders: %something%
//
// It is non-greedy so that adjacent placeholders inside a single string
// (e.g. "%foo%-%bar%") are matched separately. Escaped percent signs (%%)
// are handled in Resolve itself.
var placeholderRe = regexp.MustCompile(`%([^%\s]+)%`)

// Resolve expands %env(...)% and %parameter% placeholders in s using the
// loaded EnvVars and the parameters: section of the merged config.
//
// If a placeholder cannot be resolved, the original placeholder text is
// kept so callers can still see something useful in error messages.
//
// Recursion is bounded to avoid infinite loops on parameter cycles.
func (c *Config) Resolve(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	return c.resolveBounded(s, 8)
}

func (c *Config) resolveBounded(s string, depth int) string {
	if depth <= 0 || !strings.Contains(s, "%") {
		return s
	}

	// Handle escaped %% by temporarily replacing with a sentinel.
	const sentinel = "\x00ESC_PCT\x00"
	s = strings.ReplaceAll(s, "%%", sentinel)

	out := placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[1 : len(match)-1]
		v, ok := c.resolvePlaceholder(inner, depth-1)
		if !ok {
			return match
		}
		return v
	})

	out = strings.ReplaceAll(out, sentinel, "%")
	return out
}

// resolvePlaceholder evaluates a single placeholder body (the text between
// the % markers) and returns its expanded value.
func (c *Config) resolvePlaceholder(body string, depth int) (string, bool) {
	if strings.HasPrefix(body, "env(") && strings.HasSuffix(body, ")") {
		return c.resolveEnvExpression(body[len("env(") : len(body)-1])
	}
	return c.resolveParameter(body, depth)
}

// resolveEnvExpression evaluates the inside of env(...): a colon-separated
// list of processors followed by the variable name, e.g.
//
//	MESSENGER_TRANSPORT_DSN              -> raw env value
//	default:my_default:MESSENGER_DSN     -> env or default
//	bool:APP_URL_CHECK_DISABLED          -> raw value (we don't cast)
//	string:default::SOME_VAR             -> env or empty string
//
// Casting processors (bool, int, float, json, string, trim, lower, upper)
// pass through untouched - rule code can do its own typed comparison via
// parseBool / strconv. The "default" processor is honored so config such as
// `'%env(default:doctrine://...:MESSENGER_TRANSPORT_DSN)%'` resolves
// usefully when the env var is unset.
func (c *Config) resolveEnvExpression(expr string) (string, bool) {
	parts := splitEnvProcessors(expr)
	if len(parts) == 0 {
		return "", false
	}

	varName := parts[len(parts)-1]
	processors := parts[:len(parts)-1]

	value, ok := c.EnvVars[varName]

	if !ok || value == "" {
		// Look for a default processor: the value following "default" is the
		// fallback, which itself may be the literal next token.
		for i := 0; i+1 < len(processors); i++ {
			if processors[i] == "default" {
				return processors[i+1], true
			}
		}
		if !ok {
			return "", false
		}
	}
	return value, true
}

// splitEnvProcessors splits an env() expression on ':' but treats the final
// segment as a single variable name. Symfony's env processor syntax doesn't
// allow ':' inside variable names so a plain split works.
func splitEnvProcessors(expr string) []string {
	return strings.Split(expr, ":")
}

// resolveParameter looks up a top-level parameter reference such as
// "%shopware.cart.compress%". Symfony stores these flat under the
// "parameters:" key. Values may themselves contain placeholders so we
// recurse with a decremented depth budget.
func (c *Config) resolveParameter(name string, depth int) (string, bool) {
	params, ok := c.Data["parameters"].(map[string]any)
	if !ok {
		return "", false
	}
	v, exists := params[name]
	if !exists {
		return "", false
	}
	s, ok := toString(v)
	if !ok {
		return "", false
	}
	if strings.Contains(s, "%") {
		s = c.resolveBounded(s, depth)
	}
	return s, true
}
