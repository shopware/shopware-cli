package shop

import (
	"database/sql/driver"

	"github.com/DATA-DOG/go-sqlmock"
)

// dumpTestColumn is a column of the table dumped by expectSingleTableDump.
type dumpTestColumn struct {
	name, columnType string
	nullable         bool
}

// expectSingleTableDump expects the queries of dumping a database that only
// has the given table. dataQuery matches the query selecting its rows, which
// returns row.
func expectSingleTableDump(mock sqlmock.Sqlmock, table string, columns []dumpTestColumn, dataQuery string, row ...driver.Value) {
	names := make([]string, 0, len(columns))
	columnSchemas := sqlmock.NewRows([]string{"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT", "EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION"})
	for _, column := range columns {
		nullable := "NO"
		if column.nullable {
			nullable = "YES"
		}
		names = append(names, column.name)
		columnSchemas.AddRow(table, column.name, column.columnType, nil, nullable, nil, "", nil, "", nil)
	}

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"table", "type"}).AddRow(table, "BASE TABLE"))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
			AddRow(table, "InnoDB", "utf8mb4_unicode_ci", "", "Dynamic", nil))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(columnSchemas)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").WillReturnRows(
		sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX"}))
	mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION"}))
	mock.ExpectQuery("SELECT COUNT.*FROM INFORMATION_SCHEMA.COLUMNS.*TABLE_NAME = 'CHECK_CONSTRAINTS'.*COLUMN_NAME = 'TABLE_NAME'").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
	mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE"}))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `" + table + "`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	for range 2 {
		mock.ExpectQuery("SELECT \\* FROM `" + table + "` LIMIT 1").WillReturnRows(sqlmock.NewRows(names))
	}
	mock.ExpectQuery(dataQuery).WillReturnRows(sqlmock.NewRows(names).AddRow(row...))
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(sqlmock.NewRows([]string{"table", "type"}))
	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(sqlmock.NewRows([]string{"trigger", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}))
}
