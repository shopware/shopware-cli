package mysqldump

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				{Name: "fk.app_translation.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
			},
		},
		&TableSchema{
			Name:       "app_action_button",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.app_action_button.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
			},
		},
		&TableSchema{
			Name:       "app_action_button_translation",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "app_action_button_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.app_action_button_translation.app_action_button_id", Columns: []string{"app_action_button_id"}, ReferencedTable: "app_action_button", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
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

	assert.Equal(t, "(`app_secret` IS NOT NULL) IS NOT TRUE", dumper.excludeWhere["app"])
	assert.Equal(t, "((`app_id` IN ("+excludedApps+")) IS NOT TRUE)", dumper.excludeWhere["app_translation"])
	assert.Equal(t, "((`app_id` IN ("+excludedApps+")) IS NOT TRUE)", dumper.excludeWhere["app_action_button"])
	assert.Equal(t,
		"((`app_action_button_id` IN (SELECT `id` FROM `app_action_button` WHERE `app_id` IN ("+excludedApps+"))) IS NOT TRUE)",
		dumper.excludeWhere["app_action_button_translation"])

	// IS NOT TRUE also preserves NULL references; unrelated tables stay complete.
	assert.NotContains(t, dumper.excludeWhere, "product")
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
				{Name: "fk.child.parent_id", Columns: []string{"parent_id", "parent_version_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id", "version_id"}, OnDelete: "CASCADE"},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"parent": "secret IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	assert.Equal(t, "(secret IS NOT NULL) IS NOT TRUE", dumper.excludeWhere["parent"])
	assert.Equal(t,
		"(((`parent_id`, `parent_version_id`) IN (SELECT `id`, `version_id` FROM `parent` WHERE (secret IS NOT NULL))) IS NOT TRUE)",
		dumper.excludeWhere["child"])
}

func TestExcludeCombinesDirectExclusionAndForeignKeyConditions(t *testing.T) {
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
				{Name: "fk.webhook.app_id", Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"app": "`app_secret` IS NOT NULL", "webhook": "inactive = 1"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedApps := "SELECT `id` FROM `app` WHERE (`app_secret` IS NOT NULL)"
	assert.Equal(t, "(inactive = 1) IS NOT TRUE AND ((`app_id` IN ("+excludedApps+")) IS NOT TRUE)", dumper.excludeWhere["webhook"])
}

func TestExcludeSetNullRewritesOnlyMatchingForeignKeys(t *testing.T) {
	for _, configuredNull := range []bool{false, true} {
		name, retainedValue := "keep original reference", "`parent_id`"
		if configuredNull {
			name, retainedValue = "configured NULL rewrite", "NULL"
		}
		t.Run(name, func(t *testing.T) {
			db, mock := getDB(t)
			t.Cleanup(func() { _ = db.Close() })
			dumper := NewMySQLDumper(db)
			dumper.schemaCache = map[string]*TableSchema{
				"parent": {Name: "parent", Columns: notNullColumns("id", "secret")},
				"child": {Name: "child", Columns: []ColumnSchema{{Name: "id"}, {Name: "parent_id", Nullable: true}}, ForeignKeys: []ForeignKeySchema{{
					Name: "fk.child.parent", Columns: []string{"parent_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id"}, OnDelete: "SET NULL",
				}}},
			}
			dumper.ExcludeMap = map[string]string{"parent": "secret = 1"}
			if configuredNull {
				dumper.SelectMap = map[string]map[string]string{"child": {"parent_id": "NULL"}}
				mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			}
			require.NoError(t, dumper.computeExcludeFilters(t.Context()))

			mock.ExpectQuery("SELECT \\* FROM `child` LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}))
			columns, query, err := dumper.getSelectQueryFor(t.Context(), "child")
			require.NoError(t, err)
			assert.Len(t, columns, 2)
			assert.Contains(t, columns, "`id`")
			assert.Contains(t, query, "CASE WHEN `parent_id` IN (SELECT `id` FROM `parent` WHERE")
			assert.Contains(t, query, "THEN NULL ELSE "+retainedValue+" END AS `parent_id`")
			if configuredNull {
				assert.Equal(t, "NULL", dumper.SelectMap["child"]["parent_id"])
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestExcludeFailsWhenRetainedRowsReferenceExcludedParent(t *testing.T) {
	db, mock := getDB(t)
	dumper := NewMySQLDumper(db)
	dumper.schemaCache = map[string]*TableSchema{
		"parent": {Name: "parent", Columns: notNullColumns("id", "secret")},
		"child": {Name: "child", Columns: notNullColumns("id", "parent_id"), ForeignKeys: []ForeignKeySchema{{
			Name: "fk.child.parent", Columns: []string{"parent_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id"}, OnDelete: "RESTRICT",
		}}},
	}
	dumper.ExcludeMap = map[string]string{"parent": "secret = 1"}
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err := dumper.computeExcludeFilters(t.Context())
	assert.ErrorContains(t, err, "retained rows reference excluded rows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExcludeValidatesSelfReferencingRowsAgainstFinalFilter(t *testing.T) {
	db, mock := getDB(t)
	dumper := NewMySQLDumper(db)
	dumper.schemaCache = map[string]*TableSchema{
		"node": {Name: "node", Columns: notNullColumns("id", "secret", "parent_id"), ForeignKeys: []ForeignKeySchema{{
			Name: "fk.node.parent", Columns: []string{"parent_id"}, ReferencedTable: "node", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE",
		}}},
	}
	dumper.ExcludeMap = map[string]string{"node": "secret = 1"}
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err := dumper.computeExcludeFilters(t.Context())
	assert.ErrorContains(t, err, "retained rows reference excluded rows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExcludeAllowsEmptyFilteredForeignKeyGraph(t *testing.T) {
	db, mock := getDB(t)
	dumper := NewMySQLDumper(db)
	dumper.schemaCache = map[string]*TableSchema{
		"parent": {Name: "parent", Columns: notNullColumns("id", "secret")},
		"child": {Name: "child", Columns: notNullColumns("id", "parent_id"), ForeignKeys: []ForeignKeySchema{{
			Name: "fk.child.parent", Columns: []string{"parent_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE",
		}}},
	}
	dumper.ExcludeMap = map[string]string{"parent": "secret = 1"}
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))
	assert.NoError(t, mock.ExpectationsWereMet())
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
				{Name: "fk.a.b_id", Columns: []string{"b_id"}, ReferencedTable: "b", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
			},
		},
		&TableSchema{
			Name:       "b",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "a_id", Nullable: true}, {Name: "s_id"}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.b.a_id", Columns: []string{"a_id"}, ReferencedTable: "a", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
				{Name: "fk.b.s_id", Columns: []string{"s_id"}, ReferencedTable: "s", ReferencedColumns: []string{"id"}, OnDelete: "CASCADE"},
			},
		},
	)
	dumper.ExcludeMap = map[string]string{"s": "secret IS NOT NULL"}

	assert.NoError(t, dumper.computeExcludeFilters(t.Context()))

	excludedS := "SELECT `id` FROM `s` WHERE (secret IS NOT NULL)"
	assert.Contains(t, dumper.excludeWhere["b"], "(`s_id` IN ("+excludedS+")) IS NOT TRUE")
	assert.Contains(t, dumper.excludeWhere["a"], "`b_id` IN (SELECT `id` FROM `b` WHERE")
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

func TestExcludeValidatesStructureOnlyParentReferences(t *testing.T) {
	for _, tc := range []struct {
		name     string
		action   string
		omitLeaf bool
	}{
		{name: "restrictive reference", action: "RESTRICT"},
		{name: "unplanned nulling", action: "SET NULL"},
		{name: "referencing data also omitted", action: "RESTRICT", omitLeaf: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := getDB(t)
			t.Cleanup(func() { _ = db.Close() })
			dumper := limitTestDumper(
				&TableSchema{Name: "app", PrimaryKey: []string{"id"}, Columns: notNullColumns("id", "secret")},
				&TableSchema{Name: "owner", PrimaryKey: []string{"id"},
					Columns:     []ColumnSchema{{Name: "id"}, {Name: "app_id", Nullable: true}},
					ForeignKeys: []ForeignKeySchema{{Columns: []string{"app_id"}, ReferencedTable: "app", ReferencedColumns: []string{"id"}, OnDelete: "SET NULL"}}},
				&TableSchema{Name: "leaf", Columns: []ColumnSchema{{Name: "owner_id", Nullable: true}},
					ForeignKeys: []ForeignKeySchema{{Columns: []string{"owner_id"}, ReferencedTable: "owner", ReferencedColumns: []string{"id"}, OnDelete: tc.action}}},
			)
			dumper.db = db
			dumper.ExcludeMap = map[string]string{"app": "secret=1"}
			dumper.filterMap = map[string]string{"owner": NoDataMapPlacement}
			if tc.omitLeaf {
				dumper.filterMap["leaf"] = NoDataMapPlacement
			} else {
				query := "SELECT EXISTS (SELECT 1 FROM `leaf` WHERE `owner_id` IN (SELECT `id` FROM `owner`))"
				mock.ExpectQuery(regexp.QuoteMeta(query)).
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			}
			err := dumper.computeExcludeFilters(t.Context())
			if tc.omitLeaf {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestExcludeRejectsOverlappingDeleteActions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rightColumns []string
		rightAction  string
		placement    string
		excludeRight bool
		wantError    bool
	}{
		{name: "partial null overlap", rightColumns: []string{"y", "z"}, rightAction: "SET NULL", excludeRight: true, wantError: true},
		{name: "cascade overlap", rightColumns: []string{"y", "z"}, rightAction: "CASCADE", excludeRight: true, wantError: true},
		{name: "identical null columns", rightColumns: []string{"y", "x"}, rightAction: "SET NULL", excludeRight: true},
		{name: "unaffected parent", rightColumns: []string{"y", "z"}, rightAction: "CASCADE"},
		{name: "structure only", rightColumns: []string{"y", "z"}, rightAction: "CASCADE", excludeRight: true, placement: NoDataMapPlacement},
		{name: "ignored", rightColumns: []string{"y", "z"}, rightAction: "CASCADE", excludeRight: true, placement: IgnoreMapPlacement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dumper := limitTestDumper(
				&TableSchema{Name: "a"}, &TableSchema{Name: "b"},
				&TableSchema{Name: "child", ForeignKeys: []ForeignKeySchema{
					{Columns: []string{"x", "y"}, ReferencedTable: "a", ReferencedColumns: []string{"x", "y"}, OnDelete: "SET NULL"},
					{Columns: tc.rightColumns, ReferencedTable: "b", ReferencedColumns: tc.rightColumns, OnDelete: tc.rightAction},
				}},
			)
			dumper.ExcludeMap = map[string]string{"a": "secret = 1"}
			if tc.excludeRight {
				dumper.ExcludeMap["b"] = "secret = 1"
			}
			dumper.filterMap = map[string]string{"child": tc.placement}
			// Schema rejection must not depend on querying source rows.
			err := dumper.computeExcludeFilters(t.Context())
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExcludeSkipsStructureOnlyTableValidation(t *testing.T) {
	db, mock := getDB(t)
	t.Cleanup(func() { _ = db.Close() })
	dumper := NewMySQLDumper(db)
	dumper.filterMap = map[string]string{"child": NoDataMapPlacement}
	schemas := map[string]*TableSchema{"child": {Name: "child", ForeignKeys: []ForeignKeySchema{
		{ReferencedTable: "unavailable"},
	}}}
	assert.NoError(t, dumper.validateExcludedReferences(t.Context(), schemas))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteActionsCanInterfere(t *testing.T) {
	left := ForeignKeySchema{OnDelete: "SET NULL", Columns: []string{"x", "y"}}
	for _, tc := range []struct {
		name  string
		right ForeignKeySchema
		want  bool
	}{
		{name: "partial null overlap", right: ForeignKeySchema{OnDelete: "SET NULL", Columns: []string{"y", "z"}}, want: true},
		{name: "cascade overlap", right: ForeignKeySchema{OnDelete: "CASCADE", Columns: []string{"y", "z"}}, want: true},
		{name: "same null columns", right: ForeignKeySchema{OnDelete: "SET NULL", Columns: []string{"y", "x"}}},
		{name: "disjoint columns", right: ForeignKeySchema{OnDelete: "SET NULL", Columns: []string{"z"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deleteActionsCanInterfere(left, tc.right))
		})
	}
}

func TestSetNullUsesFinalParentFilters(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{Name: "parent"},
		&TableSchema{Name: "child", ForeignKeys: []ForeignKeySchema{{
			Columns: []string{"parent_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id"}, OnDelete: "SET NULL",
		}}},
	)
	dumper.excludeWhere = map[string]string{"parent": "secret IS NULL"}
	dumper.limitWhere = map[string]string{"parent": "id IN (SELECT id FROM frozen_parent_rows)"}
	dumper.computeExcludeSetNull(dumper.schemaCache)
	assert.Contains(t, dumper.selectColumnExpression("child", "parent_id"), "frozen_parent_rows")
}
