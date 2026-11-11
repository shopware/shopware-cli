package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readWhereConfig(t *testing.T, files map[string]string) *Config {
	t.Helper()

	dir := t.TempDir()
	t.Chdir(dir)
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	cfg, err := ReadConfig(t.Context(), filepath.Join(dir, "project.yml"), false)
	require.NoError(t, err)

	return cfg
}

func TestWhereAppendCombinesWithIncludedCondition(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"inc.yml": "dump:\n  where:\n    system_config: \"a = 1 OR b = 2\"\n    product: \"x = 1\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include:
  - inc.yml
dump:
  where:
    system_config: !append "c = 3 OR d = 4"
`,
	})

	assert.Equal(t, map[string]string{
		"system_config": "(a = 1 OR b = 2) AND (c = 3 OR d = 4)",
		"product":       "x = 1",
	}, cfg.ConfigDump.Where)
}

func TestWhereAppendMultipleIncludesInOrder(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"a.yml": "dump:\n  where:\n    t: \"base\"\n",
		"b.yml": "dump:\n  where:\n    t: !append \"b1 OR b2\"\n",
		"c.yml": "dump:\n  where:\n    t: !append \"c1\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include: [a.yml, b.yml, c.yml]
dump:
  where:
    t: !append "own"
`,
	})

	assert.Equal(t, "(((base) AND (b1 OR b2)) AND (c1)) AND (own)", cfg.ConfigDump.Where["t"])
}

func TestWhereAppendNestedIncludes(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"deep.yml": "dump:\n  where:\n    t: \"deep\"\n",
		"mid.yml":  "include: [deep.yml]\ndump:\n  where:\n    t: !append \"mid\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include: [mid.yml]
dump:
  where:
    t: !append "own"
`,
	})

	assert.Equal(t, "((deep) AND (mid)) AND (own)", cfg.ConfigDump.Where["t"])
}

func TestWhereAppendWithoutBaseCondition(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"project.yml": "compatibility_date: \"2026-01-01\"\ndump:\n  where:\n    t: !append \"a OR b\"\n",
	})

	assert.Equal(t, map[string]string{"t": "a OR b"}, cfg.ConfigDump.Where)
}

func TestWhereAppendMatchesTableCaseInsensitive(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"inc.yml": "dump:\n  where:\n    System_Config: \"a\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include: [inc.yml]
dump:
  where:
    system_config: !append "b"
`,
	})

	assert.Equal(t, map[string]string{"System_Config": "(a) AND (b)"}, cfg.ConfigDump.Where)
}

func TestWhereStillOverridesIncludedCondition(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"inc.yml": "dump:\n  where:\n    t: \"included\"\n    other: \"o\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include: [inc.yml]
dump:
  where:
    t: "own"
`,
	})

	// Existing behaviour: the included value replaces the project's value, no AND.
	assert.Equal(t, map[string]string{"t": "included", "other": "o"}, cfg.ConfigDump.Where)
}

func TestWhereAppendInLocalOverride(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"project.yml":       "compatibility_date: \"2026-01-01\"\ndump:\n  where:\n    t: \"a OR b\"\n",
		"project.local.yml": "dump:\n  where:\n    t: !append \"c\"\n",
	})

	assert.Equal(t, map[string]string{"t": "(a OR b) AND (c)"}, cfg.ConfigDump.Where)
}

func TestWhereAppendSurvivesLocalOverrideOfOtherKeys(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"inc.yml": "dump:\n  where:\n    t: \"a OR b\"\n",
		"project.yml": `compatibility_date: "2026-01-01"
include: [inc.yml]
dump:
  where:
    t: !append "c"
`,
		"project.local.yml": "url: https://local.example.com\n",
	})

	assert.Equal(t, map[string]string{"t": "(a OR b) AND (c)"}, cfg.ConfigDump.Where)
}
