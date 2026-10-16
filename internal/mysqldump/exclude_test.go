package mysqldump

import (
	"bytes"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func appGraphDumper() *Dumper {
	return limitTestDumper(
		&TableSchema{
			Name:       "app",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_secret"),
		},
		&TableSchema{
			Name:       "app_translation",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.app_translation.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "app_action_button",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.app_action_button.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "app_action_button_translation",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_action_button_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.app_action_button_translation.app_action_button_id", Columns: []string{"app_action_button_id"}, ReferencedTable: "app_action_button", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "product",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id"),
		},
	)
}

func TestExcludeCascadesOverForeignKeys(t *testing.T) {
	dumper := appGraphDumper()
	dumper.ExcludeMap = map[string]string{"app": "`app_secret` IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedApps := "SELECT `id` FROM `app` WHERE (`app_secret` IS NOT NULL)"

	assert.Equal(t, "NOT (`app_secret` IS NOT NULL)", dumper.excludeWhere["app"])
	assert.Equal(t, "(`app_id` NOT IN ("+excludedApps+"))", dumper.excludeWhere["app_translation"])
	assert.Equal(t, "(`app_id` NOT IN ("+excludedApps+"))", dumper.excludeWhere["app_action_button"])
	assert.Equal(t,
		"(`app_action_button_id` NOT IN (SELECT `id` FROM `app_action_button` WHERE `app_id` IN ("+excludedApps+")))",
		dumper.excludeWhere["app_action_button_translation"])

	// product does not reference an excluded table, so it stays complete
	assert.NotContains(t, dumper.excludeWhere, "product")
}

func TestExcludeKeepsRowsWithNullableForeignKey(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "app",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "app_secret", Nullable: true}},
		},
		&TableSchema{
			Name:       "custom_field_set",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "app_id", Nullable: true}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.custom_field_set.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"app": "`app_secret` IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedApps := "SELECT `id` FROM `app` WHERE (`app_secret` IS NOT NULL)"
	assert.Equal(t, "(`app_id` IS NULL OR `app_id` NOT IN ("+excludedApps+"))", dumper.excludeWhere["custom_field_set"])
}

func TestExcludeCascadesOverCompositeForeignKey(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "parent",
			PrimaryKey: []string{"id", "version_id"},
			Columns:    notNullColumns("id", "version_id", "secret"),
		},
		&TableSchema{
			Name:       "child",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "parent_id", "parent_version_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.child.parent_id", Columns: []string{"parent_id", "parent_version_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id", "version_id"}},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"parent": "secret IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	assert.Equal(t, "NOT (secret IS NOT NULL)", dumper.excludeWhere["parent"])
	assert.Equal(t,
		"((`parent_id`, `parent_version_id`) NOT IN (SELECT `id`, `version_id` FROM `parent` WHERE (secret IS NOT NULL)))",
		dumper.excludeWhere["child"])
}

func TestExcludeCombinesSeedAndForeignKeyConditions(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "app",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_secret"),
		},
		&TableSchema{
			Name:       "webhook",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_id", "inactive"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.webhook.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"app": "`app_secret` IS NOT NULL", "webhook": "inactive = 1"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedApps := "SELECT `id` FROM `app` WHERE (`app_secret` IS NOT NULL)"
	assert.Equal(t, "NOT (inactive = 1) AND (`app_id` NOT IN ("+excludedApps+"))", dumper.excludeWhere["webhook"])
}

func TestExcludeTerminatesOnForeignKeyCycles(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "s",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "secret"),
		},
		&TableSchema{
			Name:       "a",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "b_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.a.b_id", Columns: []string{"b_id"}, ReferencedTable: "b", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "b",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "a_id", Nullable: true}, {Name: "s_id"}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.b.a_id", Columns: []string{"a_id"}, ReferencedTable: "a", ReferencedColumns: []string{"id"}},
				{Name: "fk.b.s_id", Columns: []string{"s_id"}, ReferencedTable: "s", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"s": "secret IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedS := "SELECT `id` FROM `s` WHERE (secret IS NOT NULL)"
	assert.Contains(t, dumper.excludeWhere["b"], "(`s_id` NOT IN ("+excludedS+"))")
	assert.Contains(t, dumper.excludeWhere["a"], "`b_id` NOT IN (SELECT `id` FROM `b` WHERE")
}

func TestExcludeUnknownTableIsIgnored(t *testing.T) {
	dumper := limitTestDumper(&TableSchema{
		Name:       "app",
		PrimaryKey: []string{"id"},
		Columns:    notNullColumns("id"),
	})
	dumper.ExcludeMap = map[string]string{"missing": "secret IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))
	assert.Empty(t, dumper.excludeWhere)
}

func TestDumpTableDataDirectSkipsLockingForExcludeFilteredTables(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.LockTables = true
	dumper.excludeWhere = map[string]string{"table": "`id` NOT IN (SELECT 1)"}

	// No FLUSH TABLES expectation: the exclusion condition contains a subquery,
	// so locking must be skipped and the count query is the first statement.
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `table` WHERE `id` NOT IN \\(SELECT 1\\)").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	var buf bytes.Buffer
	assert.NoError(t, dumper.dumpTableDataDirect(t.Context(), &buf, "table"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEffectiveWhereCombinesUserLimitAndExcludeConditions(t *testing.T) {
	dumper := NewMySQLDumper(nil)
	dumper.WhereMap = map[string]string{"t": "a = 1"}
	dumper.limitWhere = map[string]string{"t": "`b` IN (SELECT 1)"}
	dumper.excludeWhere = map[string]string{"t": "NOT (c IS NOT NULL)", "u": "NOT (d IS NOT NULL)"}

	assert.Equal(t, "(a = 1) AND (`b` IN (SELECT 1)) AND (NOT (c IS NOT NULL))", dumper.effectiveWhere("T"))
	assert.Equal(t, "NOT (d IS NOT NULL)", dumper.effectiveWhere("u"))
	assert.Equal(t, "", dumper.effectiveWhere("other"))
}
