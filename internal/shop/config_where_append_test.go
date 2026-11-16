package shop

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/mysqldump"
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

	assert.Equal(t, map[string]string{"system_config": "(a) AND (b)"}, cfg.ConfigDump.Where)

	// Exercise the dumper's actual query lookup, stopping after its row count.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(sqlmock.NewRows([]string{"name", "type"}).AddRow("system_config", "BASE TABLE"))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES").WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).AddRow("system_config", "InnoDB", "utf8mb4_unicode_ci", "", "", nil))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS").WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT", "EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION"}))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS").WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX"}))
	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT DISTINCT.*KEY_COLUMN_USAGE").WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION"}))
	mock.ExpectQuery("SELECT.*TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS").WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE"}))
	stop := errors.New("stop after filtered query")
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `system_config` WHERE \\(a\\) AND \\(b\\)").WillReturnError(stop)
	dumper := mysqldump.NewMySQLDumper(db)
	configureDumper(dumper, cfg.ConfigDump, DumpDatabaseOptions{SkipLockTables: true})
	require.ErrorIs(t, dumper.Dump(t.Context(), io.Discard), stop)
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestWhereAppendLocalOverrideWithIncludes(t *testing.T) {
	cfg := readWhereConfig(t, map[string]string{
		"project.yml":       "compatibility_date: \"2026-01-01\"\ninclude: [inc.yml]\ndump:\n  where:\n    t: project\n",
		"inc.yml":           "dump:\n  where:\n    t: included\n",
		"project.local.yml": "dump:\n  where:\n    t: !append local\n",
	})
	assert.Equal(t, "(included) AND (local)", cfg.ConfigDump.Where["t"])
}

func TestWhereAppendWritePreservesSource(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "local"}[local], func(t *testing.T) {
			files := map[string]string{
				"project.yml": "compatibility_date: \"2026-01-01\"\ninclude: [mid.yml]\ndump:\n  where:\n    t: !append own\n",
				"mid.yml":     "include: [inc.yml]\ndump:\n  where:\n    t: !append middle\n",
				"inc.yml":     "dump:\n  where:\n    t: base\n",
			}
			if local {
				files["project.local.yml"] = "dump:\n  where:\n    t: !append local\n"
			}
			cfg := readWhereConfig(t, files)
			want := cfg.ConfigDump.Where["t"]
			for range 2 {
				require.NoError(t, WriteConfig(cfg, "."))
				source, err := readConfigAsMap("project.yml")
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"t": appendValue("own")}, source["dump"].(map[string]any)["where"])
				cfg, err = ReadConfig(t.Context(), "project.yml", false)
				require.NoError(t, err)
				assert.Equal(t, want, cfg.ConfigDump.Where["t"])
			}
			// Removing the original addition must not leave it in a saved base.
			source, err := readConfigAsMap("project.yml")
			require.NoError(t, err)
			delete(source["dump"].(map[string]any), "where")
			data, err := marshalMap(source)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile("project.yml", data, 0o644))
			cfg, err = ReadConfig(t.Context(), "project.yml", false)
			require.NoError(t, err)
			assert.NotContains(t, cfg.ConfigDump.Where["t"], "own")
		})
	}
}
