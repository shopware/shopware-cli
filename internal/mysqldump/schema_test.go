package mysqldump

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapCollation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"utf8mb4_0900_ai_ci", "utf8mb4_unicode_ci"},
		{"utf8mb4_0900_as_ci", "utf8mb4_unicode_ci"},
		{"utf8mb4_0900_as_cs", "utf8mb4_bin"},
		{"utf8mb4_unicode_ci", "utf8mb4_unicode_ci"},
		{"latin1_swedish_ci", "latin1_swedish_ci"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapCollation(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestColumnSchema_IsBinary(t *testing.T) {
	tests := []struct {
		colType  string
		expected bool
	}{
		{"binary(16)", true},
		{"varbinary(255)", true},
		{"blob", true},
		{"tinyblob", true},
		{"mediumblob", true},
		{"longblob", true},
		{"BINARY(16)", true},
		{"varchar(255)", false},
		{"int", false},
		{"text", false},
	}

	for _, tt := range tests {
		t.Run(tt.colType, func(t *testing.T) {
			col := ColumnSchema{Type: tt.colType}
			assert.Equal(t, tt.expected, col.IsBinary())
		})
	}
}

func TestColumnSchema_IsGenerated(t *testing.T) {
	t.Run("generated column", func(t *testing.T) {
		col := ColumnSchema{
			GenerationExpr: sql.NullString{String: "CONCAT(first_name, last_name)", Valid: true},
		}
		assert.True(t, col.IsGenerated())
	})

	t.Run("regular column", func(t *testing.T) {
		col := ColumnSchema{
			GenerationExpr: sql.NullString{Valid: false},
		}
		assert.False(t, col.IsGenerated())
	})

	t.Run("empty expression", func(t *testing.T) {
		col := ColumnSchema{
			GenerationExpr: sql.NullString{String: "", Valid: true},
		}
		assert.False(t, col.IsGenerated())
	})
}

func TestTableSchema_GetBinaryColumns(t *testing.T) {
	schema := &TableSchema{
		Columns: []ColumnSchema{
			{Name: "id", Type: "binary(16)"},
			{Name: "name", Type: "varchar(255)"},
			{Name: "data", Type: "blob"},
			{Name: "count", Type: "int"},
		},
	}

	binaryCols := schema.GetBinaryColumns()
	assert.ElementsMatch(t, []string{"id", "data"}, binaryCols)
}

func TestTableSchema_GetGeneratedColumns(t *testing.T) {
	schema := &TableSchema{
		Columns: []ColumnSchema{
			{Name: "id", Type: "int"},
			{Name: "full_name", Type: "varchar(255)", GenerationExpr: sql.NullString{String: "CONCAT(first, last)", Valid: true}},
			{Name: "first", Type: "varchar(100)"},
			{Name: "last", Type: "varchar(100)"},
		},
	}

	genCols := schema.GetGeneratedColumns()
	assert.ElementsMatch(t, []string{"full_name"}, genCols)
}

func TestTableSchema_BuildCreateTableSQL_Basic(t *testing.T) {
	schema := &TableSchema{
		Name:      "users",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "bigint", Nullable: false, Extra: "AUTO_INCREMENT"},
			{Name: "name", Type: "varchar(255)", Nullable: false},
			{Name: "email", Type: "varchar(255)", Nullable: true, Default: sql.NullString{String: "NULL", Valid: true}},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "CREATE TABLE `users`")
	assert.Contains(t, sql, "`id` bigint NOT NULL AUTO_INCREMENT")
	assert.Contains(t, sql, "`name` varchar(255) NOT NULL")
	assert.Contains(t, sql, "`email` varchar(255),")
	assert.NotContains(t, sql, "DEFAULT NULL")
	assert.Contains(t, sql, "PRIMARY KEY (`id`)")
	assert.Contains(t, sql, "ENGINE=InnoDB")
	assert.Contains(t, sql, "DEFAULT CHARSET=utf8mb4")
	assert.Contains(t, sql, "COLLATE=utf8mb4_unicode_ci")
}

func TestTableSchema_BuildCreateTableSQL_WithIndexes(t *testing.T) {
	schema := &TableSchema{
		Name:      "products",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "binary(16)", Nullable: false},
			{Name: "name", Type: "varchar(255)", Nullable: false},
			{Name: "sku", Type: "varchar(64)", Nullable: false},
		},
		PrimaryKey: []string{"id"},
		Indexes: []IndexSchema{
			{
				Name:     "idx_name",
				Columns:  []IndexColumnSchema{{Name: sql.NullString{String: "name", Valid: true}}},
				IsUnique: false,
				Type:     "BTREE",
			},
			{
				Name:     "idx_name_desc",
				Columns:  []IndexColumnSchema{{Name: sql.NullString{String: "name", Valid: true}, Order: "DESC"}},
				IsUnique: false,
				Type:     "BTREE",
			},
			{
				Name:     "uniq_sku",
				Columns:  []IndexColumnSchema{{Name: sql.NullString{String: "sku", Valid: true}}},
				IsUnique: true,
				Type:     "BTREE",
			},
			{
				Name:     "idx_created_at_before_2026",
				Columns:  []IndexColumnSchema{{Expression: sql.NullString{String: "(`created_at` < _utf8mb4'2026-01-01 00:00:00')", Valid: true}}},
				IsUnique: false,
				Type:     "BTREE",
			},
		},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "KEY `idx_name` (`name`)")
	assert.Contains(t, sql, "KEY `idx_name_desc` (`name` DESC)")
	assert.Contains(t, sql, "UNIQUE KEY `uniq_sku` (`sku`)")
	assert.Contains(t, sql, "KEY `idx_created_at_before_2026` ((`created_at` < _utf8mb4'2026-01-01 00:00:00'))")
}

func TestTableSchema_BuildCreateTableSQL_WithForeignKeys(t *testing.T) {
	schema := &TableSchema{
		Name:      "orders",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "binary(16)", Nullable: false},
			{Name: "customer_id", Type: "binary(16)", Nullable: false},
		},
		PrimaryKey: []string{"id"},
		ForeignKeys: []ForeignKeySchema{
			{
				Name:              "fk_customer",
				Columns:           []string{"customer_id"},
				ReferencedTable:   "customers",
				ReferencedColumns: []string{"id"},
				OnDelete:          "CASCADE",
				OnUpdate:          "RESTRICT",
			},
		},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "CONSTRAINT `fk_customer` FOREIGN KEY (`customer_id`) REFERENCES `customers` (`id`)")
	assert.Contains(t, sql, "ON DELETE CASCADE")
}

func TestTableSchema_BuildCreateTableSQL_GeneratedColumn(t *testing.T) {
	schema := &TableSchema{
		Name:      "people",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false, Extra: "AUTO_INCREMENT"},
			{Name: "first_name", Type: "varchar(100)", Nullable: false},
			{Name: "last_name", Type: "varchar(100)", Nullable: false},
			{
				Name:           "full_name",
				Type:           "varchar(201)",
				Nullable:       true,
				GenerationExpr: sql.NullString{String: "CONCAT(first_name, ' ', last_name)", Valid: true},
				IsVirtual:      true,
			},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "GENERATED ALWAYS AS (CONCAT(first_name, ' ', last_name)) VIRTUAL")
}

func TestTableSchema_BuildCreateTableSQL_GeneratedColumn_WithEscapedQuotes(t *testing.T) {
	// This test covers issue #846: generation expressions from INFORMATION_SCHEMA.COLUMNS
	// contain escaped quotes that need to be unescaped
	schema := &TableSchema{
		Name:      "b2b_components_pending_order",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false, Extra: "AUTO_INCREMENT"},
			{Name: "price", Type: "json", Nullable: false},
			{
				Name:     "tax_status",
				Type:     "varchar(255)",
				Nullable: true,
				// This simulates what MySQL's INFORMATION_SCHEMA.COLUMNS returns with escaped quotes
				GenerationExpr: sql.NullString{String: "json_unquote(json_extract(`price`,_utf8mb4\\'$.taxStatus\\'))", Valid: true},
				IsVirtual:      true,
			},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	// The output should have unescaped quotes
	assert.Contains(t, sql, "GENERATED ALWAYS AS (json_unquote(json_extract(`price`,_utf8mb4'$.taxStatus'))) VIRTUAL")
	// Ensure the broken version with backslash-escaped quotes is NOT in the output
	assert.NotContains(t, sql, "\\'")
}

func TestTableSchema_BuildCreateTableSQL_ColumnCollation(t *testing.T) {
	schema := &TableSchema{
		Name:      "test",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false},
			{Name: "name", Type: "varchar(255)", Nullable: false, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "case_sensitive", Type: "varchar(255)", Nullable: false, Collation: sql.NullString{String: "utf8mb4_bin", Valid: true}},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.NotContains(t, sql, "`name` varchar(255) COLLATE utf8mb4_unicode_ci")
	assert.Contains(t, sql, "`case_sensitive` varchar(255) COLLATE utf8mb4_bin")
}

func TestTableSchema_BuildCreateTableSQL_WithCheckConstraints(t *testing.T) {
	schema := &TableSchema{
		Name:      "products",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false},
			{Name: "data", Type: "longtext", Nullable: false},
		},
		PrimaryKey: []string{"id"},
		CheckConstraints: []CheckConstraintSchema{
			{Name: "json.products.data", Expression: "json_valid(`data`)"},
		},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "CONSTRAINT `json.products.data` CHECK (json_valid(`data`))")
}

func TestTableSchema_BuildCreateTableSQL_WithAutoIncrement(t *testing.T) {
	schema := &TableSchema{
		Name:          "users",
		Engine:        "InnoDB",
		Charset:       "utf8mb4",
		Collation:     "utf8mb4_unicode_ci",
		AutoIncrement: sql.NullInt64{Int64: 100, Valid: true},
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false, Extra: "AUTO_INCREMENT"},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "AUTO_INCREMENT=100")
}

func TestTableSchema_BuildCreateTableSQL_WithCharacterSet(t *testing.T) {
	schema := &TableSchema{
		Name:      "test",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "id", Type: "int", Nullable: false},
			{Name: "data", Type: "longtext", Nullable: false, CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_bin", Valid: true}},
		},
		PrimaryKey: []string{"id"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "CHARACTER SET utf8mb4 COLLATE utf8mb4_bin")
}

func TestTableSchema_BuildCreateTableSQL_NoSpacesInKeys(t *testing.T) {
	schema := &TableSchema{
		Name:      "test",
		Engine:    "InnoDB",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
		Columns: []ColumnSchema{
			{Name: "col1", Type: "int", Nullable: false},
			{Name: "col2", Type: "int", Nullable: false},
		},
		PrimaryKey: []string{"col1", "col2"},
	}

	sql := schema.BuildCreateTableSQL()

	assert.Contains(t, sql, "PRIMARY KEY (`col1`,`col2`)")
}

func TestFetchTableSchema(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
			AddRow("test_table", "InnoDB", "utf8mb4_0900_ai_ci", "Test table", "Dynamic", 100))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT",
			"EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION",
		}).
			AddRow("test_table", "id", "binary(16)", nil, "NO", nil, "", nil, "", nil).
			AddRow("test_table", "name", "varchar(255)", "utf8mb4", "NO", nil, "", "utf8mb4_0900_ai_ci", "", nil).
			AddRow("test_table", "created_at", "datetime", nil, "NO", "CURRENT_TIMESTAMP", "", nil, "", nil))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*TABLE_SCHEMA = 'information_schema'.*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).
			AddRow(1))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX",
		}).
			AddRow("test_table", "PRIMARY", "id", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("test_table", "idx_created_at", "created_at", nil, 1, "BTREE", nil, "A", "", 1).
			AddRow("test_table", "idx_created_at_before_2026", nil, "(`created_at` < _utf8mb4\\'2026-01-01 00:00:00\\')", 1, "BTREE", nil, "A", "", 1).
			AddRow("test_table", "idx_complex", nil, "(LENGTH(`name`) < 5)", 1, "BTREE", nil, "A", "Some comment", 1).
			AddRow("test_table", "idx_complex", "name", nil, 1, "BTREE", nil, "D", "Some comment", 2).
			AddRow("test_table", "idx_prefix", "name", nil, 1, "BTREE", "5", "", "", 1).
			AddRow("test_table", "unq_name", "name", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("wrong_table", "idx_test", "test", nil, 1, "BTREE", nil, "A", "", 1))

	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME",
			"REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION",
		}))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE",
		}).
			AddRow("test_table", "test_table.check.with_string_literal", "`name` LIKE _utf8mb4\\'A%\\'"))

	err := dumper.prefetchAllSchemas(t.Context(), []string{"test_table"})
	require.NoError(t, err)

	schema, err := dumper.fetchTableSchema("test_table")
	require.NoError(t, err)

	assert.Equal(t, "test_table", schema.Name)
	assert.Equal(t, "InnoDB", schema.Engine)
	assert.Equal(t, "utf8mb4_unicode_ci", schema.Collation) // Should be mapped
	assert.Equal(t, "utf8mb4", schema.Charset)
	assert.Equal(t, int64(100), schema.AutoIncrement.Int64)
	assert.Len(t, schema.Columns, 3)
	assert.Equal(t, []string{"id"}, schema.PrimaryKey)
	assert.Equal(t, "utf8mb4_unicode_ci", schema.Columns[1].Collation.String)
	assert.Equal(t, 1, len(schema.CheckConstraints))
	assert.Equal(t, "test_table.check.with_string_literal", schema.CheckConstraints[0].Name)
	assert.Equal(t, "`name` LIKE _utf8mb4'A%'", schema.CheckConstraints[0].Expression)
	assert.Len(t, schema.Indexes, 5)
	assert.Equal(t, IndexSchema{
		Name: "idx_created_at",
		Columns: []IndexColumnSchema{
			{
				Name:  sql.NullString{String: "created_at", Valid: true},
				Order: "ASC",
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 0,
	}, schema.Indexes[0])
	assert.Equal(t, IndexSchema{
		Name: "idx_created_at_before_2026",
		Columns: []IndexColumnSchema{
			{
				Expression: sql.NullString{String: "(`created_at` < _utf8mb4'2026-01-01 00:00:00')", Valid: true},
				Order:      "ASC",
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 1,
	}, schema.Indexes[1])
	assert.Equal(t, IndexSchema{
		Name: "idx_complex",
		Columns: []IndexColumnSchema{
			{
				Expression: sql.NullString{String: "(LENGTH(`name`) < 5)", Valid: true},
				Order:      "ASC",
			},
			{
				Name:  sql.NullString{String: "name", Valid: true},
				Order: "DESC",
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 2,
		Comment:    "Some comment",
	}, schema.Indexes[2])
	assert.Equal(t, IndexSchema{
		Name: "idx_prefix",
		Columns: []IndexColumnSchema{
			{
				Name:    sql.NullString{String: "name", Valid: true},
				Order:   "ASC",
				SubPart: sql.NullInt64{Int64: 5, Valid: true},
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 3,
	}, schema.Indexes[3])
	assert.Equal(t, IndexSchema{
		Name: "unq_name",
		Columns: []IndexColumnSchema{
			{
				Name:  sql.NullString{String: "name", Valid: true},
				Order: "ASC",
			},
		},
		IsUnique:   true,
		Type:       "BTREE",
		SeqInTable: 4,
	}, schema.Indexes[4])
}

// Same as TestFetchTableSchema but the query for the INFORMATION_SCHEMA.STATISTICS.EXPRESSION column returns 0 and no expression indexes are defined
func TestFetchTableSchemaMariaDB(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
			AddRow("test_table", "InnoDB", "utf8mb4_0900_ai_ci", "Test table", "Dynamic", 100))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT",
			"EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION",
		}).
			AddRow("test_table", "id", "binary(16)", nil, "NO", nil, "", nil, "", nil).
			AddRow("test_table", "name", "varchar(255)", "utf8mb4", "NO", nil, "", "utf8mb4_0900_ai_ci", "", nil).
			AddRow("test_table", "created_at", "datetime", nil, "NO", "CURRENT_TIMESTAMP", "", nil, "", nil))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*TABLE_SCHEMA = 'information_schema'.*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).
			AddRow(0))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX",
		}).
			AddRow("test_table", "PRIMARY", "id", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("test_table", "idx_created_at", "created_at", nil, 1, "BTREE", nil, "A", "", 1).
			AddRow("test_table", "idx_prefix", "name", nil, 1, "BTREE", "5", "", "", 1).
			AddRow("test_table", "unq_name", "name", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("wrong_table", "idx_test", "test", nil, 1, "BTREE", nil, "A", "", 1))

	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME",
			"REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION",
		}))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE",
		}).
			AddRow("test_table", "test_table.check.with_string_literal", "`name` LIKE _utf8mb4\\'A%\\'"))

	err := dumper.prefetchAllSchemas(t.Context(), []string{"test_table"})
	require.NoError(t, err)

	schema, err := dumper.fetchTableSchema("test_table")
	require.NoError(t, err)

	assert.Equal(t, "test_table", schema.Name)
	assert.Equal(t, "InnoDB", schema.Engine)
	assert.Equal(t, "utf8mb4_unicode_ci", schema.Collation) // Should be mapped
	assert.Equal(t, "utf8mb4", schema.Charset)
	assert.Equal(t, int64(100), schema.AutoIncrement.Int64)
	assert.Len(t, schema.Columns, 3)
	assert.Equal(t, []string{"id"}, schema.PrimaryKey)
	assert.Equal(t, "utf8mb4_unicode_ci", schema.Columns[1].Collation.String)
	assert.Equal(t, 1, len(schema.CheckConstraints))
	assert.Equal(t, "test_table.check.with_string_literal", schema.CheckConstraints[0].Name)
	assert.Equal(t, "`name` LIKE _utf8mb4'A%'", schema.CheckConstraints[0].Expression)
	assert.Len(t, schema.Indexes, 3)
	assert.Equal(t, IndexSchema{
		Name: "idx_created_at",
		Columns: []IndexColumnSchema{
			{
				Name:  sql.NullString{String: "created_at", Valid: true},
				Order: "ASC",
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 0,
	}, schema.Indexes[0])
	assert.Equal(t, IndexSchema{
		Name: "idx_prefix",
		Columns: []IndexColumnSchema{
			{
				Name:    sql.NullString{String: "name", Valid: true},
				Order:   "ASC",
				SubPart: sql.NullInt64{Int64: 5, Valid: true},
			},
		},
		IsUnique:   false,
		Type:       "BTREE",
		SeqInTable: 1,
	}, schema.Indexes[1])
	assert.Equal(t, IndexSchema{
		Name: "unq_name",
		Columns: []IndexColumnSchema{
			{
				Name:  sql.NullString{String: "name", Valid: true},
				Order: "ASC",
			},
		},
		IsUnique:   true,
		Type:       "BTREE",
		SeqInTable: 2,
	}, schema.Indexes[2])
}

func TestGetCreateTableStatement_Integration(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
			AddRow("products", "InnoDB", "utf8mb4_0900_ai_ci", "", "Dynamic", nil))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT",
			"EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION",
		}).
			AddRow("products", "id", "binary(16)", nil, "NO", nil, "", nil, "", nil).
			AddRow("products", "name", "varchar(255)", "utf8mb4", "NO", nil, "", "utf8mb4_0900_ai_ci", "", nil).
			AddRow("products", "created_at", "datetime", nil, "NO", "CURRENT_TIMESTAMP", "", nil, "", nil))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*TABLE_SCHEMA = 'information_schema'.*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).
			AddRow(1))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX",
		}).
			AddRow("products", "PRIMARY", "id", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("products", "idx_created_at", "created_at", nil, 1, "BTREE", nil, "A", "", 1).
			AddRow("products", "idx_created_at_before_2026", nil, "(`created_at` < _utf8mb4\\'2026-01-01 00:00:00\\')", 1, "BTREE", nil, "A", "", 1).
			AddRow("products", "idx_complex", nil, "(LENGTH(`name`) < 5)", 1, "BTREE", nil, "A", "Some comment", 1).
			AddRow("products", "idx_complex", "name", nil, 1, "BTREE", nil, "D", "Some comment", 2).
			AddRow("products", "idx_prefix", "name", nil, 1, "BTREE", "5", "", "", 1).
			AddRow("products", "unq_name", "name", nil, 0, "BTREE", nil, "A", "", 1).
			AddRow("wrong_table", "idx_test", "test", nil, 1, "BTREE", nil, "A", "", 1))

	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME",
			"REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION",
		}))

	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE",
		}))

	err := dumper.prefetchAllSchemas(t.Context(), []string{"products"})
	require.NoError(t, err)

	stmt, err := dumper.getCreateTableStatement("products")
	require.NoError(t, err)

	assert.Contains(t, stmt, "DROP TABLE IF EXISTS `products`")
	assert.Contains(t, stmt, "CREATE TABLE `products`")
	assert.NotContains(t, stmt, "utf8mb4_0900_ai_ci")
	assert.Contains(t, stmt, "utf8mb4_unicode_ci")
	assert.True(t, dumper.isColumnBinary("products", "id"))
	assert.Contains(t, stmt, " KEY `idx_created_at` (`created_at`)")
	assert.Contains(t, stmt, " KEY `idx_created_at_before_2026` ((`created_at` < _utf8mb4'2026-01-01 00:00:00'))")
	assert.Contains(t, stmt, " KEY `idx_complex` ((LENGTH(`name`) < 5),`name` DESC) COMMENT 'Some comment'")
	assert.Contains(t, stmt, " KEY `idx_prefix` (`name`(5))")
	assert.Contains(t, stmt, " UNIQUE KEY `unq_name` (`name`)")
}

func TestIsStringType(t *testing.T) {
	tests := []struct {
		colType  string
		expected bool
	}{
		{"varchar(255)", true},
		{"char(10)", true},
		{"text", true},
		{"tinytext", true},
		{"mediumtext", true},
		{"longtext", true},
		{"enum('a','b')", true},
		{"set('x','y')", true},
		{"VARCHAR(100)", true},
		{"int", false},
		{"bigint", false},
		{"binary(16)", false},
		{"blob", false},
		{"datetime", false},
		{"json", false},
	}

	for _, tt := range tests {
		t.Run(tt.colType, func(t *testing.T) {
			assert.Equal(t, tt.expected, isStringType(tt.colType))
		})
	}
}

func TestIsNumericType(t *testing.T) {
	tests := []struct {
		colType  string
		expected bool
	}{
		{"int", true},
		{"tinyint", true},
		{"smallint", true},
		{"mediumint", true},
		{"bigint", true},
		{"float", true},
		{"double", true},
		{"decimal(10,2)", true},
		{"numeric", true},
		{"bit", true},
		{"varchar(255)", false},
		{"text", false},
		{"datetime", false},
		{"blob", false},
	}

	for _, tt := range tests {
		t.Run(tt.colType, func(t *testing.T) {
			assert.Equal(t, tt.expected, isNumericType(tt.colType))
		})
	}
}

func TestFormatDefault(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		colType  string
		expected string
	}{
		{"null value", "NULL", "varchar(255)", "NULL"},
		{"numeric default", "0", "int", "0"},
		{"string default", "hello", "varchar(255)", "'hello'"},
		{"current timestamp", "CURRENT_TIMESTAMP", "datetime", "CURRENT_TIMESTAMP"},
		{"current date", "CURRENT_DATE", "date", "CURRENT_DATE"},
		{"current time", "CURRENT_TIME", "time", "CURRENT_TIME"},
		{"function call with parens", "uuid()", "binary(16)", "(uuid())"},
		{"json_object function", "json_object()", "longtext", "(json_object())"},
		{"uuid_to_bin function", "uuid_to_bin(uuid())", "binary(16)", "(uuid_to_bin(uuid()))"},
		{"string with quotes", "it's", "varchar(255)", "'it\\'s'"},
		{"hex literal lowercase", "x'0fa91ce3e96a4bc2be4bd9ce752c3425'", "binary(16)", "x'0fa91ce3e96a4bc2be4bd9ce752c3425'"},
		{"hex literal uppercase", "X'0FA91CE3'", "binary(16)", "X'0FA91CE3'"},
		{"hex literal 0x format", "0x0fa91ce3e96a4bc2be4bd9ce752c3425", "binary(16)", "0x0fa91ce3e96a4bc2be4bd9ce752c3425"},
		// MariaDB returns COLUMN_DEFAULT as a backslash-escaped SQL string literal.
		// These must not be double-quoted/escaped.
		{"mariadb escaped string default", "\\'product\\'", "varchar(255)", "'product'"},
		{"mariadb escaped binary string default", "\\'somevalue\\'", "binary(16)", "'somevalue'"},
		{"mysql escaped json default", "_utf8mb4\\'{}\\'", "json", "(_utf8mb4'{}')"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDefault(tt.value, tt.colType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHexLiteralRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"x'0fa91ce3e96a4bc2be4bd9ce752c3425'", true},
		{"X'0FA91CE3'", true},
		{"x''", true},
		{"0x0fa91ce3", true},
		{"0xABCDEF", true},
		{"hello", false},
		{"x'hello'", false}, // invalid hex chars
		{"uuid()", false},
		{"'x'0fa91ce3''", false}, // quoted
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, hexLiteralRegex.MatchString(tt.input))
		})
	}
}

func TestFetchAllForeignKeys_NumericName(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)

	dumper.schemaCache = map[string]*TableSchema{
		"product": {Name: "product"},
	}

	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").
		WillReturnRows(sqlmock.NewRows([]string{
			"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME",
			"REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION",
		}).
			AddRow("product", "1", "product_manufacturer_id", "product_manufacturer", "id", "CASCADE", "SET NULL", 1))

	err := dumper.fetchAllForeignKeys(t.Context())
	require.NoError(t, err)

	require.Len(t, dumper.schemaCache["product"].ForeignKeys, 1)
	assert.Equal(t, "fk.product.product_manufacturer_id", dumper.schemaCache["product"].ForeignKeys[0].Name)
}
