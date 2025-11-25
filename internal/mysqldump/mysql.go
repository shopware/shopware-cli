package mysqldump

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

type Dumper struct {
	db *sql.DB
	// SelectMap contains column rewrites per table (table -> column -> SQL expression)
	SelectMap map[string]map[string]string
	// WhereMap contains WHERE clauses per table (table -> WHERE clause)
	WhereMap map[string]string
	// NoData contains list of tables to dump structure only (no data)
	NoData []string
	// Ignore contains list of tables to skip entirely
	Ignore    []string
	filterMap map[string]string
	// LockTables controls whether to lock tables during dump (default: true)
	LockTables bool
	// Quick enables quick mode for mysqldump (default: false)
	Quick               bool
	openTx              *sql.Tx
	mapBins             map[string][]string
	mapExclusionColumns map[string][]string
}

const (
	IgnoreMapPlacement = "ignore"
	NoDataMapPlacement = "nodata"
	FakerUsageCheck    = "faker"
)

var skipDefinerRegExp = regexp.MustCompile(`(?m)DEFINER=[^ ]* `)

// NewMySQLDumper creates a new MySQL dumper with default configuration.
func NewMySQLDumper(db *sql.DB) *Dumper {
	return &Dumper{
		db:                  db,
		mapBins:             make(map[string][]string),
		mapExclusionColumns: make(map[string][]string),
		LockTables:          true,
		Quick:               false,
	}
}

// Dump creates a MySQL dump and writes it to an io.Writer
// returns error in the event something gos wrong in the middle of the dump process
func (d *Dumper) Dump(ctx context.Context, w io.Writer) error {
	var dump string
	var tmp string
	dump = "SET NAMES utf8mb4;\n"
	dump += "SET FOREIGN_KEY_CHECKS = 0;\n"

	tables, err := d.getTables(ctx)
	if err != nil {
		return err
	}

	d.filterMap = make(map[string]string)
	for _, table := range d.NoData {
		d.filterMap[strings.ToLower(table)] = NoDataMapPlacement
	}
	for _, table := range d.Ignore {
		d.filterMap[strings.ToLower(table)] = IgnoreMapPlacement
	}

	for _, table := range tables {
		if d.filterMap[strings.ToLower(table)] == IgnoreMapPlacement {
			continue
		}

		skipData := d.filterMap[strings.ToLower(table)] == NoDataMapPlacement
		tmp, err = d.getCreateTableStatement(table)
		if err != nil {
			return err
		}

		tmp = d.excludeGeneratedColumns(table, tmp)

		d.parseBinaryRelations(table, tmp)

		dump += tmp
		if !skipData {
			dump, err = d.dumpData(ctx, w, dump, table)
			if err != nil {
				return err
			}
		}

		if _, err = fmt.Fprintln(w, dump); err != nil {
			logging.FromContext(ctx).Error(err.Error())
		}
	}

	_, err = fmt.Fprintf(w, "SET FOREIGN_KEY_CHECKS = 1;\n")

	if err := d.dumpViews(ctx, w); err != nil {
		return err
	}

	if err := d.dumpTriggers(ctx, w); err != nil {
		return err
	}

	return err
}

func (d *Dumper) parseBinaryRelations(table, createTable string) {
	// no cache, if it is requested, replace existing entry
	d.mapBins[table] = make([]string, 0)

	scanner := bufio.NewScanner(strings.NewReader(createTable))
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), "binary") || strings.Contains(strings.ToLower(scanner.Text()), "blob") {
			r := regexp.MustCompile("`([^(]*)`")
			columnName := r.FindAllStringSubmatch(scanner.Text(), -1)

			if len(columnName) > 0 && len(columnName[0]) > 0 {
				d.mapBins[table] = append(d.mapBins[table], columnName[0][1])
			}
		}
	}
}

func (d *Dumper) excludeGeneratedColumns(table, createTable string) string {
	// Track generated columns for exclusion during data dump
	// but always preserve them in the CREATE TABLE statement
	d.mapExclusionColumns[table] = make([]string, 0)

	scanner := bufio.NewScanner(strings.NewReader(createTable))
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), "generated always") {
			r := regexp.MustCompile("`([^(]*)`")
			columnName := r.FindAllStringSubmatch(scanner.Text(), -1)

			if len(columnName) > 0 && len(columnName[0]) > 0 {
				d.mapExclusionColumns[table] = append(d.mapExclusionColumns[table], columnName[0][1])
			}
		}
	}

	return createTable
}

func (d *Dumper) isColumnBinary(table, columnName string) bool {
	columnName = strings.Trim(columnName, "`")
	if val, ok := d.mapBins[table]; ok {
		for _, b := range val {
			if b == columnName {
				return true
			}
		}
	}

	return false
}

func (d *Dumper) isColumnExcluded(table, columnName string) bool {
	if val, ok := d.mapExclusionColumns[table]; ok {
		for _, b := range val {
			if b == columnName {
				return true
			}
		}
	}

	return false
}

func (d *Dumper) dumpData(ctx context.Context, w io.Writer, dump, table string) (string, error) {
	var cnt uint64
	var tmp string
	var err error
	if d.LockTables {
		_, err = d.mysqlFlushTable(table)
		if err != nil {
			return "", err
		}
	}

	tmp, cnt, err = d.getTableHeader(table)
	if err != nil {
		return "", err
	}
	dump += tmp
	if cnt > 0 {
		dump += d.getLockTableWriteStatement(table)

		// before the data dump, we need to flush everything to file
		if _, err = fmt.Fprintln(w, dump); err != nil {
			return "", err
		}
		// and after flush we need to clear the variable
		dump = ""

		if dErr := d.dumpTableData(ctx, w, table); dErr != nil {
			return "", dErr
		}

		dump += d.getUnlockTablesStatement()
	}

	if d.LockTables {
		if _, dErr := d.mysqlUnlockTables(); err != nil {
			return "", dErr
		}
	}

	return dump, nil
}


func (d *Dumper) getTables(ctx context.Context) ([]string, error) {
	tables := make([]string, 0)

	rows, err := d.db.Query("SHOW FULL TABLES")
	if a := d.evaluateErrors(err, rows); a != nil {
		return tables, a
	}

	defer func(rows *sql.Rows) {
		dErr := rows.Close()
		if dErr != nil {
			logging.FromContext(ctx).Errorf("failed to close rows while getting tables: %s", err.Error())
		}
	}(rows)

	for rows.Next() {
		var tableName, tableType string

		if dErr := rows.Scan(&tableName, &tableType); dErr != nil {
			return tables, dErr
		}

		if tableType == "BASE TABLE" {
			tables = append(tables, tableName)
		}
	}

	return tables, nil
}

func (d *Dumper) getViews(ctx context.Context) ([]string, error) {
	views := make([]string, 0)

	rows, err := d.db.Query("SHOW FULL TABLES")
	if a := d.evaluateErrors(err, rows); a != nil {
		return views, a
	}

	defer func(rows *sql.Rows) {
		dErr := rows.Close()
		if dErr != nil {
			logging.FromContext(ctx).Errorf("failed to close rows while getting views: %s", dErr.Error())
		}
	}(rows)

	for rows.Next() {
		var viewName, tableType string

		if dErr := rows.Scan(&viewName, &tableType); dErr != nil {
			return views, dErr
		}

		if tableType == "VIEW" {
			views = append(views, viewName)
		}
	}

	return views, nil
}

func (d *Dumper) dumpTableData(ctx context.Context, w io.Writer, table string) error {
	columns, err := d.getColumnsForSelect(ctx, table, false)

	if err != nil {
		return err
	}

	rows, _, err := d.selectAllDataFor(ctx, table)
	if a := d.evaluateErrors(err, rows); a != nil {
		return a
	}

	defer func(rows *sql.Rows) {
		dErr := rows.Close()
		if dErr != nil {
			logging.FromContext(ctx).Errorf("dumping data for table %s failed, closing rows failed: %s", table, dErr.Error())
		}
	}(rows)

	numRows := 100
	if d.Quick {
		numRows = 1
	}

	values := make([]*sql.RawBytes, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	query := d.generateInsertStatement(columns, table)
	var data []string
	for rows.Next() {
		if dErr := rows.Scan(scanArgs...); err != nil {
			return dErr
		}
		var vals []string
		for i, col := range values {
			vals = append(vals, d.getProperEscapedValue(col, table, columns[i]))
		}

		data = append(data, fmt.Sprintf("( %s )", strings.Join(vals, ", ")))
		if len(data) >= numRows {
			fmt.Fprintf(w, "%s\n%s;\n", query, strings.Join(data, ",\n"))
			data = make([]string, 0)
		}
	}

	if len(data) > 0 {
		fmt.Fprintf(w, "%s\n%s;\n", query, strings.Join(data, ",\n"))
	}

	return nil
}

func (d *Dumper) getProperEscapedValue(col *sql.RawBytes, table, columnName string) string {
	val := "NULL"

	if col != nil {
		// Always use hex encoding for binary columns
		if d.isColumnBinary(table, columnName) {
			encodedVal := hex.EncodeToString(*col)

			if encodedVal != "" {
				val = "0x" + encodedVal
			} else {
				val = "NULL"
			}
		} else {
			val = string(*col)

			if len(val) >= 5 && val[0:5] == FakerUsageCheck {
				val, _ = replaceStringWithFakerWhenRequested(val)
			}

			val = fmt.Sprintf("'%s'", escape(val))
		}
	}

	return val
}

func (d *Dumper) generateInsertStatement(cols []string, table string) string {
	s := fmt.Sprintf("INSERT INTO `%s` (", table)
	for _, col := range cols {
		s += fmt.Sprintf("%s, ", col)
	}

	return s[:len(s)-2] + ") VALUES"
}

func (d *Dumper) getTableHeader(table string) (str string, count uint64, err error) {
	str = fmt.Sprintf("\n--\n-- Data for table `%s`", table)
	count, err = d.rowCount(table)

	if err != nil {
		return "", 0, err
	}

	str += fmt.Sprintf(" -- %d rows\n--\n\n", count)
	return
}

func (d *Dumper) evaluateErrors(base error, rows *sql.Rows) error {
	if base != nil {
		return base
	}

	if rows != nil && rows.Err() != nil {
		return rows.Err()
	}

	return nil
}

func (d *Dumper) selectAllDataFor(ctx context.Context, table string) (rows *sql.Rows, columns []string, err error) {
	var selectQuery string
	if columns, selectQuery, err = d.getSelectQueryFor(ctx, table); err != nil {
		return
	}
	if rows, err = d.db.Query(selectQuery); err != nil {
		return
	}

	return
}

func (d *Dumper) getSelectQueryFor(ctx context.Context, table string) (cols []string, query string, err error) {
	cols, err = d.getColumnsForSelect(ctx, table, true)
	if err != nil {
		return cols, "", err
	}
	query = fmt.Sprintf("SELECT %s FROM `%s`", strings.Join(cols, ", "), table)
	if where, ok := d.WhereMap[strings.ToLower(table)]; ok {
		query = fmt.Sprintf("%s WHERE %s", query, where)
	}
	return
}

func (d *Dumper) getLockTableWriteStatement(table string) string {
	return fmt.Sprintf("LOCK TABLES `%s` WRITE;\n", table)
}

func (d *Dumper) getUnlockTablesStatement() string {
	return "UNLOCK TABLES;\n"
}

func (d *Dumper) getColumnsForSelect(ctx context.Context, table string, considerRewriteMap bool) (columns []string, err error) {
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM `%s` LIMIT 1", table))
	if a := d.evaluateErrors(err, rows); a != nil {
		return columns, a
	}

	defer func(rows *sql.Rows) {
		dErr := rows.Close()
		if dErr != nil {
			logging.FromContext(ctx).Warnf("getting columns for select on table %s failed: %s", table, dErr.Error())
		}
	}(rows)
	var tmp []string
	if tmp, err = rows.Columns(); err != nil {
		return
	}

	for _, column := range tmp {
		if d.isColumnExcluded(table, column) {
			continue
		}

		replacement, ok := d.SelectMap[strings.ToLower(table)][strings.ToLower(column)]
		if ok && considerRewriteMap {
			if len(replacement) >= 5 && replacement[0:5] == FakerUsageCheck {
				replacement = fmt.Sprintf("'%s'", replacement)
			}

			columns = append(columns, fmt.Sprintf("%s AS `%s`", replacement, column))
		} else {
			columns = append(columns, fmt.Sprintf("`%s`", column))
		}
	}

	return columns, nil
}

func (d *Dumper) rowCount(table string) (count uint64, err error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if where, ok := d.WhereMap[strings.ToLower(table)]; ok {
		query = fmt.Sprintf("%s WHERE %s", query, where)
	}
	row := d.useTransactionOrDBQueryRow(query)
	if err = row.Scan(&count); err != nil {
		return
	}
	return
}

func (d *Dumper) getCreateTableStatement(table string) (string, error) {
	s := fmt.Sprintf("\n--\n-- Structure for table `%s`\n--\n\n", table)
	s += fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", table)
	row := d.useTransactionOrDBQueryRow(fmt.Sprintf("SHOW CREATE TABLE `%s`", table))
	var tname, ddl string
	if err := row.Scan(&tname, &ddl); err != nil {
		return "", err
	}
	s += fmt.Sprintf("%s;\n", ddl)
	return s, nil
}

func (d *Dumper) mysqlFlushTable(table string) (sql.Result, error) {
	return d.useTransactionOrDBExec(fmt.Sprintf("FLUSH TABLES `%s` WITH READ LOCK", table))
}

// Release the global read locks
func (d *Dumper) mysqlUnlockTables() (sql.Result, error) {
	return d.useTransactionOrDBExec("UNLOCK TABLES")
}

func (d *Dumper) useTransactionOrDBQueryRow(query string) *sql.Row {
	return d.db.QueryRow(query)
}

func (d *Dumper) useTransactionOrDBExec(query string) (sql.Result, error) {
	return d.db.Exec(query)
}

func (d *Dumper) getTransaction() *sql.Tx {
	if d.openTx == nil {
		var err error
		d.openTx, err = d.db.Begin()
		if err != nil {
			panic("could not start a transaction")
		}
	}

	return d.openTx
}

func (d *Dumper) dumpTriggers(ctx context.Context, w io.Writer) error {
	triggers, err := d.getTriggers(ctx)
	if err != nil {
		return err
	}

	for _, trigger := range triggers {
		ddl, err := d.getTrigger(trigger)

		if err != nil {
			return err
		}

		fmt.Fprintf(w, "\n--\n-- Trigger `%s`\n--\n\n", trigger)

		// Always use // as delimiter for triggers
		fmt.Fprintf(w, "DELIMITER //\n")

		if _, err := w.Write([]byte(ddl)); err != nil {
			return err
		}

		fmt.Fprintf(w, "//\nDELIMITER ;\n")
	}

	return nil
}

func (d *Dumper) getTriggers(ctx context.Context) ([]string, error) {
	triggers := make([]string, 0)

	rows, err := d.db.Query("SHOW TRIGGERS")
	if a := d.evaluateErrors(err, rows); a != nil {
		return triggers, a
	}

	defer func(rows *sql.Rows) {
		dErr := rows.Close()
		if dErr != nil {
			logging.FromContext(ctx).Errorf("failed to close rows while getting triggers: %s", dErr.Error())
		}
	}(rows)

	for rows.Next() {
		var triggerName, unknown string

		if dErr := rows.Scan(&triggerName, &unknown, &unknown, &unknown, &unknown, &unknown, &unknown, &unknown, &unknown, &unknown, &unknown); dErr != nil {
			return triggers, dErr
		}

		triggers = append(triggers, triggerName)
	}

	return triggers, nil
}

func (d *Dumper) getTrigger(triggerName string) (string, error) {
	var ddl, unknown string

	row := d.useTransactionOrDBQueryRow(fmt.Sprintf("SHOW CREATE TRIGGER `%s`", triggerName))
	if err := row.Scan(&unknown, &unknown, &ddl, &unknown, &unknown, &unknown, &unknown); err != nil {
		return "", err
	}

	// Always skip definer for portability
	ddl = skipDefinerRegExp.ReplaceAllString(ddl, "")

	return ddl + ";\n", nil
}

func (d *Dumper) dumpViews(ctx context.Context, w io.Writer) error {
	views, err := d.getViews(ctx)
	if err != nil {
		return err
	}

	for _, view := range views {
		ddl, err := d.getView(view)

		if err != nil {
			return err
		}

		fmt.Fprintf(w, "\n--\n-- Structure for view `%s`\n--\n\n", view)
		fmt.Fprintf(w, "DROP VIEW IF EXISTS `%s`;\n", view)

		if _, err := w.Write([]byte(ddl)); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dumper) getView(viewName string) (string, error) {
	var ddl, unknown string

	row := d.useTransactionOrDBQueryRow(fmt.Sprintf("SHOW CREATE VIEW `%s`", viewName))
	if err := row.Scan(&unknown, &ddl, &unknown, &unknown); err != nil {
		return "", err
	}

	// Always skip definer for portability
	ddl = skipDefinerRegExp.ReplaceAllString(ddl, "")

	return ddl + ";\n", nil
}
