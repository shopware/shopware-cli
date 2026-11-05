package mysqldump

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateKeyRewrites(t *testing.T) {
	parent := &TableSchema{Name: "parent", Columns: notNullColumns("id")}
	fk := ForeignKeySchema{Name: "child_parent_fk", Columns: []string{"parent_id"}, ReferencedTable: "parent", ReferencedColumns: []string{"id"}}

	tests := []struct {
		name      string
		nullable  bool
		rewrite   string
		generated bool
		wantError bool
	}{
		{name: "nullable key can be nulled", nullable: true, rewrite: "NULL"},
		{name: "required key cannot be nulled", rewrite: "NULL", wantError: true},
		{name: "non-null rewrite is unsupported", nullable: true, rewrite: "42", wantError: true},
		{name: "generated foreign key cannot be rewritten", nullable: true, rewrite: "NULL", generated: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			column := ColumnSchema{Name: "parent_id", Nullable: tt.nullable}
			if tt.generated {
				column.GenerationExpr.String = "id + 1"
				column.GenerationExpr.Valid = true
			}
			child := &TableSchema{Name: "child", Columns: []ColumnSchema{column}}
			dumper := limitTestDumper(child, parent)
			dumper.SelectMap = map[string]map[string]string{"child": {"parent_id": tt.rewrite}}

			err := dumper.validateKeyRewrites(child, parent, fk)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestForeignKeyValidationUsesExportedKeyValues(t *testing.T) {
	dumper := NewMySQLDumper(nil)
	dumper.SelectMap = map[string]map[string]string{"child": {"owner_id": "NULL"}}
	dumper.excludeSetNull = map[string]map[string][]string{"owner": {"app_id": {"app_id = 1"}}}
	child := &TableSchema{Name: "child"}
	parent := &TableSchema{Name: "owner", PrimaryKey: []string{"id"}}
	fk := ForeignKeySchema{Columns: []string{"owner_id"}, ReferencedColumns: []string{"app_id"}}

	query := dumper.foreignKeyValidationQuery(child, parent, fk)
	assert.Contains(t, query, "SELECT NULL AS `owner_id` FROM `child`")
	assert.Contains(t, query, "CASE WHEN app_id = 1 THEN NULL ELSE `app_id` END AS `app_id` FROM `owner`")
	assert.Contains(t, query, "`_sw_cli_child`.`owner_id` IS NOT NULL")
}
