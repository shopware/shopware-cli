package symfonyconfig

import (
	"strconv"
	"strings"
)

// Get returns the value at a dotted path inside the merged config tree.
// Returns (value, true) if found, (nil, false) otherwise.
//
// The path supports map keys only (e.g. "framework.messenger.transports.async.dsn").
// To inspect sequences, retrieve the parent and walk manually.
//
// Any string value containing %parameter% or %env(...)% expressions is
// resolved against the loaded environment and parameters before being
// returned. Maps and sequences are returned without recursive resolution
// (call Resolve directly on inner values when needed).
func (c *Config) Get(path string) (any, bool) {
	if path == "" {
		return c.Data, true
	}

	parts := strings.Split(path, ".")
	var current any = c.Data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := m[part]
		if !exists {
			return nil, false
		}
		current = v
	}

	if s, ok := current.(string); ok {
		return c.Resolve(s), true
	}
	return current, true
}

// Has reports whether the dotted path exists in the merged config.
func (c *Config) Has(path string) bool {
	_, ok := c.Get(path)
	return ok
}

// GetString returns the value at path as a string, applying %env(...)% and
// %parameter% resolution. Returns the zero value and false if missing or
// not a stringifiable scalar.
func (c *Config) GetString(path string) (string, bool) {
	v, ok := c.Get(path)
	if !ok {
		return "", false
	}
	return toString(v)
}

// GetBool returns the value at path as a bool, applying resolution. Accepts
// real booleans plus the strings "true"/"false"/"1"/"0"/"yes"/"no"
// (case-insensitive) - matching Symfony's env(BOOL:...) casting.
func (c *Config) GetBool(path string) (bool, bool) {
	v, ok := c.Get(path)
	if !ok {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return parseBool(x)
	}
	return false, false
}

// LookupEnv returns the value of an environment variable from the merged
// dotenv + process env set, applying ExtraEnv overrides.
func (c *Config) LookupEnv(name string) (string, bool) {
	if c.EnvVars == nil {
		return "", false
	}
	v, ok := c.EnvVars[name]
	return v, ok
}

func toString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off", "":
		return false, true
	}
	return false, false
}
