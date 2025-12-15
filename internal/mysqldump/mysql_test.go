package mysqldump

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func getDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	assert.Nil(t, err)
	return db, mock
}

func getInternalMySQLInstance(db *sql.DB) *Dumper {
	return NewMySQLDumper(db)
}

func TestMySQLFlushTable(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectExec("FLUSH TABLES `table`").WillReturnResult(sqlmock.NewResult(0, 1))
	_, err := dumper.mysqlFlushTable(t.Context(), "table")
	assert.Nil(t, err)
}

func TestMySQLUnlockTables(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectExec("UNLOCK TABLES").WillReturnResult(sqlmock.NewResult(0, 1))
	_, err := dumper.mysqlUnlockTables(t.Context())
	assert.Nil(t, err)
}

func TestMySQLGetTables(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).
			AddRow("table1", "BASE TABLE").
			AddRow("table2", "BASE TABLE"),
	)
	tables, err := dumper.getTables(t.Context())
	assert.Equal(t, []string{"table1", "table2"}, tables)
	assert.Nil(t, err)
}

func TestMySQLGetTablesHandlingErrorWhenListingTables(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	expectedErr := errors.New("broken")
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnError(expectedErr)
	tables, err := dumper.getTables(t.Context())
	assert.Equal(t, []string{}, tables)
	assert.Equal(t, expectedErr, err)
}

func TestMySQLGetTablesHandlingErrorWhenScanningRow(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).AddRow(1, nil),
	)
	tables, err := dumper.getTables(t.Context())
	assert.Equal(t, []string{}, tables)
	assert.NotNil(t, err)
}

func TestMySQLDumpCreateTable(t *testing.T) {
	var ddl = "CREATE TABLE `table` (" +
		"`id` bigint(20) NOT NULL AUTO_INCREMENT, " +
		"`name` varchar(255) NOT NULL, " +
		"PRIMARY KEY (`id`), KEY `idx_name` (`name`) " +
		") ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8"
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW CREATE TABLE `table`").WillReturnRows(
		sqlmock.NewRows([]string{"Table", "Create Table"}).
			AddRow("table", ddl),
	)
	str, err := dumper.getCreateTableStatement(t.Context(), "table")

	assert.Nil(t, err)
	assert.Contains(t, str, "DROP TABLE IF EXISTS `table`")
	assert.Contains(t, str, ddl)
}

func TestMySQLDumpCreateTableHandlingErrorWhenScanningRows(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW CREATE TABLE `table`").WillReturnRows(
		sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow("table", nil),
	)

	_, err := dumper.getCreateTableStatement(t.Context(), "table")
	assert.NotNil(t, err)
}

func TestMySQLGetColumnsForSelect(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.SelectMap = map[string]map[string]string{"table": {"col2": "NOW()"}}
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnRows(
		sqlmock.NewRows([]string{"col1", "col2", "col3"}).AddRow("a", "b", "c"),
	)
	columns, err := dumper.getColumnsForSelect(t.Context(), "table", true)
	assert.Nil(t, err)
	assert.Equal(t, []string{"`col1`", "NOW() AS `col2`", "`col3`"}, columns)

	// Test that exclusion columns (e.g., generated columns) are always excluded from data dump
	dumper.mapExclusionColumns = map[string][]string{"table": {"col1"}}
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnRows(
		sqlmock.NewRows([]string{"col1", "col2", "col3"}).AddRow("a", "b", "c"),
	)
	columns, err = dumper.getColumnsForSelect(t.Context(), "table", true)
	assert.Nil(t, err)
	assert.Equal(t, []string{"NOW() AS `col2`", "`col3`"}, columns)
}

func TestMySQLGetColumnsForSelectHandlingErrorWhenQuerying(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.SelectMap = map[string]map[string]string{"table": {"col2": "NOW()"}}
	err := errors.New("broken")
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnError(err)
	columns, dErr := dumper.getColumnsForSelect(t.Context(), "table", true)
	assert.Equal(t, dErr, err)
	assert.Empty(t, columns)
}

func TestMySQLGetSelectQueryFor(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.SelectMap = map[string]map[string]string{"table": {"c2": "NOW()"}}
	dumper.WhereMap = map[string]string{"table": "c1 > 0"}
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnRows(
		sqlmock.NewRows([]string{"c1", "c2"}).AddRow("a", "b"),
	)
	_, query, err := dumper.getSelectQueryFor(t.Context(), "table")
	assert.Nil(t, err)
	assert.Equal(t, "SELECT `c1`, NOW() AS `c2` FROM `table` WHERE c1 > 0", query)
}

func TestMySQLGetSelectQueryForHandlingError(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.SelectMap = map[string]map[string]string{"table": {"c2": "NOW()"}}
	dumper.WhereMap = map[string]string{"table": "c1 > 0"}
	dErr := errors.New("broken")
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnError(dErr)
	_, query, err := dumper.getSelectQueryFor(t.Context(), "table")
	assert.Equal(t, dErr, err)
	assert.Equal(t, "", query)
}

func TestMySQLGetRowCount(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.WhereMap = map[string]string{"table": "c1 > 0"}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `table` WHERE c1 > 0").WillReturnRows(
		sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1234),
	)
	count, err := dumper.rowCount(t.Context(), "table")
	assert.Nil(t, err)
	assert.Equal(t, uint64(1234), count)
}

func TestMySQLGetRowCountHandlingError(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.WhereMap = map[string]string{"table": "c1 > 0"}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `table` WHERE c1 > 0").WillReturnRows(
		sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(nil),
	)
	count, err := dumper.rowCount(t.Context(), "table")
	assert.NotNil(t, err)
	assert.Equal(t, uint64(0), count)
}

func TestMySQLDumpTableHeader(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `table`").WillReturnRows(
		sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1234),
	)
	str, count, err := dumper.getTableHeader(t.Context(), "table")
	assert.Equal(t, uint64(1234), count)
	assert.Nil(t, err)
	assert.Contains(t, str, "Data for table `table`")
	assert.Contains(t, str, "1234 rows")
}

func TestMySQLDumpTableHeaderHandlingError(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `table`").WillReturnRows(
		sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(nil),
	)
	_, count, err := dumper.getTableHeader(t.Context(), "table")
	assert.Equal(t, uint64(0), count)
	assert.NotNil(t, err)
}

func TestMySQLDumpTableLockWrite(t *testing.T) {
	dumper := getInternalMySQLInstance(nil)
	str := dumper.getLockTableWriteStatement("table")
	assert.Contains(t, str, "LOCK TABLES `table` WRITE;")
}

func TestMySQLDumpUnlockTables(t *testing.T) {
	dumper := getInternalMySQLInstance(nil)
	str := dumper.getUnlockTablesStatement()
	assert.Contains(t, str, "UNLOCK TABLES;")
}

func TestMySQLDumpTableData(t *testing.T) {
	db, mock := getDB(t)
	buffer := bytes.NewBuffer(make([]byte, 0))

	dumper := getInternalMySQLInstance(db)
	dumper.Quick = true // Use quick mode to get one row per insert

	r := []struct {
		ID    int
		Value string
	}{
		{1, "Lettuce"},
		{2, "Cabbage"},
		{3, "Cucumber"},
		{4, "Potatoes"},
		{5, "Carrot"},
		{6, "Leek"},
	}

	mock.ExpectQuery("SELECT \\* FROM `vegetable_list` LIMIT 1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "vegetable"}).
			AddRow(1, "Lettuce"),
	)

	mock.ExpectQuery("SELECT \\* FROM `vegetable_list` LIMIT 1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "vegetable"}).
			AddRow(1, "Lettuce"),
	)

	rows := sqlmock.NewRows([]string{"id", "vegetable_list"})
	for _, row := range r {
		rows.AddRow(row.ID, row.Value)
	}
	mock.ExpectQuery("SELECT `id`, `vegetable` FROM `vegetable_list`").
		WillReturnRows(rows)

	assert.Nil(t, dumper.dumpTableData(t.Context(), buffer, "vegetable_list"))

	assert.Equal(t, strings.Count(buffer.String(), "INSERT INTO `vegetable_list` (`id`, `vegetable`) VALUES"), 6)

	for _, row := range r {
		assert.Contains(t, buffer.String(), fmt.Sprintf("'%s'", row.Value))
	}
}

func TestMySQLDumpTableDataHandlingErrorFromSelectAllDataFor(t *testing.T) {
	db, mock := getDB(t)
	buffer := bytes.NewBuffer(make([]byte, 0))
	dumper := getInternalMySQLInstance(db)
	err := errors.New("fail")
	mock.ExpectQuery("SELECT \\* FROM `table` LIMIT 1").WillReturnError(err)
	assert.Equal(t, err, dumper.dumpTableData(t.Context(), buffer, "table"))
}

func Test_mySQL_parseBinaryRelations(t *testing.T) {
	db, _ := getDB(t)
	type args struct {
		table       string
		createTable string
		expectedMap map[string][]string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"manage create table successfully",
			args{
				"table",
				`CREATE TABLE ` + "`table`" + ` (
  ` + "`id`" + ` binary(16) NOT NULL AUTO_INCREMENT,
  ` + "`s`" + ` char(60) DEFAULT NULL,
  PRIMARY KEY (` + "`id`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
				map[string][]string{
					"table": {"id"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				d := getInternalMySQLInstance(db)
				d.parseBinaryRelations(tt.args.table, tt.args.createTable)
				assert.Equal(t, d.mapBins, tt.args.expectedMap)
			},
		)
	}
}

func Test_mySQL_removeGeneratedColumns(t *testing.T) {
	db, _ := getDB(t)
	type args struct {
		table         string
		createTable   string
		strippedTable string
		expectedMap   map[string][]string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"removes successfully generated columns",
			args{
				"table",
				`CREATE TABLE ` + "`table`" + ` (
  ` + "`id`" + ` binary(16) NOT NULL AUTO_INCREMENT,
  ` + "`s`" + ` char(60) DEFAULT NULL,
  ` + "`reversed`" + ` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci GENERATED ALWAYS AS (reverse(` +
					"`keyword`" + `)) STORED
  PRIMARY KEY (` + "`id`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE ` + "`table`" + ` (
  ` + "`id`" + ` binary(16) NOT NULL AUTO_INCREMENT,
  ` + "`s`" + ` char(60) DEFAULT NULL,
  PRIMARY KEY (` + "`id`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
				map[string][]string{
					"table": {"reversed"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				d := getInternalMySQLInstance(db)
				d.excludeGeneratedColumns(tt.args.table, tt.args.createTable)
				assert.Equal(t, d.mapExclusionColumns, tt.args.expectedMap)
			},
		)
	}
}

func Test_mySQL_isColumnBinary(t *testing.T) {
	db, _ := getDB(t)
	type args struct {
		table      string
		columnName string
		m          map[string][]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"should get true",
			args{
				"table",
				"id",
				map[string][]string{
					"table": {"id"},
				},
			},
			true,
		},
		{
			"should get false",
			args{
				"table",
				"potatoes",
				map[string][]string{
					"table": {"id"},
				},
			},
			false,
		},
		{
			"should get false",
			args{
				"cabbage",
				"id",
				map[string][]string{
					"table": {"id"},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				d := getInternalMySQLInstance(db)
				d.mapBins = tt.args.m
				assert.Equalf(
					t,
					tt.want,
					d.isColumnBinary(tt.args.table, tt.args.columnName),
					"isColumnBinary(%v, %v)",
					tt.args.table,
					tt.args.columnName,
				)
			},
		)
	}
}

func Test_mySQL_isColumnExcluded(t *testing.T) {
	db, _ := getDB(t)
	type args struct {
		table      string
		columnName string
		m          map[string][]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"should get true",
			args{
				"table",
				"id",
				map[string][]string{
					"table": {"id"},
				},
			},
			true,
		},
		{
			"should get false",
			args{
				"table",
				"potatoes",
				map[string][]string{
					"table": {"id"},
				},
			},
			false,
		},
		{
			"should get false",
			args{
				"cabbage",
				"id",
				map[string][]string{
					"table": {"id"},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				d := getInternalMySQLInstance(db)
				d.mapExclusionColumns = tt.args.m
				assert.Equalf(
					t,
					tt.want,
					d.isColumnExcluded(tt.args.table, tt.args.columnName),
					"isColumnBinary(%v, %v)",
					tt.args.table,
					tt.args.columnName,
				)
			},
		)
	}
}

func Test_mySQL_ignoresTable(t *testing.T) {
	db, mock := getDB(t)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).
			AddRow("OLD_table", "BASE TABLE"),
	)

	// Expect SHOW FULL TABLES query for views
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	// Expect SHOW TRIGGERS query since triggers are dumped by default
	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "Event", "Table", "Statement", "Timing", "Created", "sql_mode", "Definer", "character_set_client", "collation_connection", "Database Collation"}),
	)

	dumper := getInternalMySQLInstance(db)

	dumper.Ignore = []string{"OLD_table"}

	b := new(strings.Builder)

	err := dumper.Dump(t.Context(), b)

	if err != nil {
		t.Error(err)
	}

	expected := "SET NAMES utf8mb4;\nSET FOREIGN_KEY_CHECKS = 0;\n\nSET FOREIGN_KEY_CHECKS = 1;\n"
	if b.String() != expected {
		t.Errorf("No tables should be dumped, expected:\n%s\ngot:\n%s", expected, b.String())
	}
}

func Test_mySQL_dumpsTriggers(t *testing.T) {
	db, mock := getDB(t)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	// Expect SHOW FULL TABLES query for views
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "Event", "Table", "Statement", "Timing", "Created", "sql_mode", "Definer", "character_set_client", "collation_connection", "Database Collation"}).AddRow(
			"OLD_table", "INSERT", "OLD_table", "BEGIN\n\tINSERT INTO `OLD_table` (`id`, `name`) VALUES (1, 'test');\nEND", "BEFORE", "2019-01-01 00:00:00", "NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION", "root@localhost", "utf8", "utf8_general_ci", "utf8_general_ci",
		),
	)

	mock.ExpectQuery("SHOW CREATE TRIGGER `OLD_table`").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "sql_mode", "Statement", "character_set_client", "Definer", "collation_connection", "Database Collation"}).AddRow(
			"OLD_table", "ONLY_FULL_GROUP_BY", "CREATE DEFINER=`root`@`%` TRIGGER `ins_sum` BEFORE INSERT ON `account` FOR EACH ROW SET @sum = @sum + NEW.amount", "", "", "", "",
		),
	)

	dumper := getInternalMySQLInstance(db)

	b := new(strings.Builder)

	err := dumper.Dump(t.Context(), b)

	if err != nil {
		t.Error(err)
	}

	// DEFINER is always stripped now
	if !strings.Contains(b.String(), "CREATE TRIGGER `ins_sum` BEFORE INSERT ON `account` FOR EACH ROW SET @sum = @sum + NEW.amount;") {
		t.Error("Trigger not dumped")
	}

	// Delimiter is always //
	if !strings.Contains(b.String(), "DELIMITER //") {
		t.Error("Trigger escaping is missing")
	}

	if !strings.Contains(b.String(), "DELIMITER ;") {
		t.Error("Trigger escaping reset is missing")
	}
}

func Test_mySQL_dumpsTriggersIgnoresDefiners(t *testing.T) {
	db, mock := getDB(t)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "Event", "Table", "Statement", "Timing", "Created", "sql_mode", "Definer", "character_set_client", "collation_connection", "Database Collation"}).AddRow(
			"OLD_table", "INSERT", "OLD_table", "BEGIN\n\tINSERT INTO `OLD_table` (`id`, `name`) VALUES (1, 'test');\nEND", "BEFORE", "2019-01-01 00:00:00", "NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION", "root@localhost", "utf8", "utf8_general_ci", "utf8_general_ci",
		),
	)

	mock.ExpectQuery("SHOW CREATE TRIGGER `OLD_table`").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "sql_mode", "Statement", "character_set_client", "Definer", "collation_connection", "Database Collation"}).AddRow(
			"OLD_table", "ONLY_FULL_GROUP_BY", "CREATE DEFINER=`root`@`%` TRIGGER `ins_sum` BEFORE INSERT ON `account` FOR EACH ROW SET @sum = @sum + NEW.amount", "", "", "", "",
		),
	)

	dumper := getInternalMySQLInstance(db)

	b := new(strings.Builder)

	err := dumper.Dump(t.Context(), b)

	if err != nil {
		t.Error(err)
	}

	if !strings.Contains(b.String(), "CREATE TRIGGER `ins_sum` BEFORE INSERT ON `account` FOR EACH ROW SET @sum = @sum + NEW.amount;") {
		t.Error("Trigger not dumped")
	}
}

func TestMySQLGetViews(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).
			AddRow("view1", "VIEW").
			AddRow("table1", "BASE TABLE").
			AddRow("view2", "VIEW"),
	)
	views, err := dumper.getViews(t.Context())
	assert.Equal(t, []string{"view1", "view2"}, views)
	assert.Nil(t, err)
}

func TestMySQLGetViewsHandlingErrorWhenListingViews(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	expectedErr := errors.New("broken")
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnError(expectedErr)
	views, err := dumper.getViews(t.Context())
	assert.Equal(t, []string{}, views)
	assert.Equal(t, expectedErr, err)
}

func TestMySQLGetViewsHandlingErrorWhenScanningRow(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).AddRow(1, nil),
	)
	views, err := dumper.getViews(t.Context())
	assert.Equal(t, []string{}, views)
	assert.NotNil(t, err)
}

func TestMySQLDumpView(t *testing.T) {
	var ddl = "CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `user_view` AS SELECT `id`, `name` FROM `users`"
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW CREATE VIEW `user_view`").WillReturnRows(
		sqlmock.NewRows([]string{"View", "Create View", "character_set_client", "collation_connection"}).
			AddRow("user_view", ddl, "utf8mb4", "utf8mb4_general_ci"),
	)
	str, err := dumper.getView(t.Context(), "user_view")

	assert.Nil(t, err)
	assert.Contains(t, str, "CREATE ALGORITHM=UNDEFINED")
	assert.Contains(t, str, "VIEW `user_view`")
	assert.NotContains(t, str, "DEFINER=") // skipDefiner is true by default
}

func TestMySQLDumpViewHandlingErrorWhenScanningRows(t *testing.T) {
	db, mock := getDB(t)
	dumper := getInternalMySQLInstance(db)
	mock.ExpectQuery("SHOW CREATE VIEW `user_view`").WillReturnRows(
		sqlmock.NewRows([]string{"View", "Create View", "character_set_client", "collation_connection"}).AddRow("user_view", nil, nil, nil),
	)

	_, err := dumper.getView(t.Context(), "user_view")
	assert.NotNil(t, err)
}

func Test_mySQL_dumpsViews(t *testing.T) {
	db, mock := getDB(t)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).
			AddRow("user_view", "VIEW"),
	)

	mock.ExpectQuery("SHOW CREATE VIEW `user_view`").WillReturnRows(
		sqlmock.NewRows([]string{"View", "Create View", "character_set_client", "collation_connection"}).AddRow(
			"user_view", "CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `user_view` AS SELECT `id`, `name` FROM `users`", "utf8mb4", "utf8mb4_general_ci",
		),
	)

	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "Event", "Table", "Statement", "Timing", "Created", "sql_mode", "Definer", "character_set_client", "collation_connection", "Database Collation"}),
	)

	dumper := getInternalMySQLInstance(db)

	b := new(strings.Builder)

	err := dumper.Dump(t.Context(), b)

	if err != nil {
		t.Error(err)
	}

	if !strings.Contains(b.String(), "DROP VIEW IF EXISTS `user_view`") {
		t.Error("View DROP statement not dumped")
	}

	if !strings.Contains(b.String(), "CREATE ALGORITHM=UNDEFINED") {
		t.Error("View not dumped")
	}

	if !strings.Contains(b.String(), "VIEW `user_view`") {
		t.Error("View name not dumped")
	}
}

func Test_mySQL_dumpsViewsIgnoresDefiners(t *testing.T) {
	db, mock := getDB(t)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}),
	)

	mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
		sqlmock.NewRows([]string{"Tables_in_database", "Table_type"}).
			AddRow("user_view", "VIEW"),
	)

	mock.ExpectQuery("SHOW CREATE VIEW `user_view`").WillReturnRows(
		sqlmock.NewRows([]string{"View", "Create View", "character_set_client", "collation_connection"}).AddRow(
			"user_view", "CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `user_view` AS SELECT `id`, `name` FROM `users`", "utf8mb4", "utf8mb4_general_ci",
		),
	)

	mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(
		sqlmock.NewRows([]string{"Trigger", "Event", "Table", "Statement", "Timing", "Created", "sql_mode", "Definer", "character_set_client", "collation_connection", "Database Collation"}),
	)

	dumper := getInternalMySQLInstance(db)

	b := new(strings.Builder)

	err := dumper.Dump(t.Context(), b)

	if err != nil {
		t.Error(err)
	}

	if strings.Contains(b.String(), "DEFINER=") {
		t.Error("DEFINER should be stripped from view")
	}

	if !strings.Contains(b.String(), "VIEW `user_view`") {
		t.Error("View not dumped")
	}
}

func Test_mySQL_parallelDefaultValue(t *testing.T) {
	db, _ := getDB(t)
	dumper := getInternalMySQLInstance(db)

	assert.Equal(t, 0, dumper.Parallel, "Parallel should be disabled by default")
}

func Test_mySQL_parallelCanBeEnabled(t *testing.T) {
	db, _ := getDB(t)
	dumper := getInternalMySQLInstance(db)
	dumper.Parallel = 4

	assert.Equal(t, 4, dumper.Parallel, "Parallel should be configurable")
}
