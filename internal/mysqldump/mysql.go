package mysqldump

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"

	"crawshaw.io/iox"

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
	Quick bool
	// InsertIntoLimit controls how many rows are included in each INSERT statement (default: 100).
	// When set to a positive value, it takes priority over Quick mode.
	// When not set (0 or negative) and Quick is true, the batch size is 1.
	InsertIntoLimit int
	// Parallel controls how many tables to dump concurrently (default: 0 = disabled)
	Parallel            int
	mapBins             map[string][]string
	mapExclusionColumns map[string][]string
	mapMu               sync.RWMutex
	// schemaCache stores prefetched table schemas
	schemaCache map[string]*TableSchema
}

const (
	IgnoreMapPlacement = "ignore"
	NoDataMapPlacement = "nodata"
	defaultInsertLimit = 100
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
		InsertIntoLimit:     0,
		Parallel:            0,
	}
}

// Dump creates a MySQL dump and writes it to an io.Writer
// returns error in the event something gos wrong in the middle of the dump process
func (d *Dumper) Dump(ctx context.Context, w io.Writer) error {
	var dump string
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

	tablesToDump := make([]string, 0, len(tables))
	for _, table := range tables {
		if d.filterMap[strings.ToLower(table)] != IgnoreMapPlacement {
			tablesToDump = append(tablesToDump, table)
		}
	}

	if err := d.prefetchAllSchemas(ctx, tablesToDump); err != nil {
		return err
	}

	if _, err = fmt.Fprintln(w, dump); err != nil {
		return err
	}

	if d.Parallel > 1 {
		if err := d.dumpTablesParallel(ctx, w, tables); err != nil {
			return err
		}
	} else {
		if err := d.dumpTablesSequential(ctx, w, tables); err != nil {
			return err
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

func (d *Dumper) dumpTablesSequential(ctx context.Context, w io.Writer, tables []string) error {
	for _, table := range tables {
		if d.filterMap[strings.ToLower(table)] == IgnoreMapPlacement {
			continue
		}

		skipData := d.filterMap[strings.ToLower(table)] == NoDataMapPlacement
		createStmt, err := d.getCreateTableStatement(table)
		if err != nil {
			return err
		}

		if _, err = fmt.Fprintln(w, createStmt); err != nil {
			return err
		}

		if !skipData {
			if err = d.dumpTableDataDirect(ctx, w, table); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Dumper) dumpTablesParallel(ctx context.Context, w io.Writer, tables []string) error {
	type tableResult struct {
		done  chan struct{}
		table string
		file  *iox.BufferFile
		err   error
	}

	tablesToDump := make([]string, 0, len(tables))
	for _, table := range tables {
		if d.filterMap[strings.ToLower(table)] != IgnoreMapPlacement {
			tablesToDump = append(tablesToDump, table)
		}
	}

	filer := iox.NewFiler(0)

	results := make([]*tableResult, len(tablesToDump))
	for i := range results {
		results[i] = &tableResult{
			done:  make(chan struct{}),
			table: tablesToDump[i],
			file:  filer.BufferFile(0),
		}
	}
	defer func() {
		for _, result := range results {
			_ = result.file.Close()
		}
	}()

	go func() {
		semaphore := make(chan struct{}, d.Parallel)

		for _, result := range results {
			semaphore <- struct{}{}

			go func() {
				defer close(result.done)
				defer func() { <-semaphore }()

				table := result.table

				skipData := d.filterMap[strings.ToLower(table)] == NoDataMapPlacement
				createStmt, err := d.getCreateTableStatement(table)
				if err != nil {
					result.err = err
					return
				}

				if _, err := io.WriteString(result.file, createStmt); err != nil {
					result.err = err
					return
				}

				if !skipData {
					if err := d.dumpTableDataToWriter(ctx, result.file, table); err != nil {
						result.err = err
						return
					}
				}
			}()
		}
	}()

	for _, result := range results {
		<-result.done
		if result.err != nil {
			return result.err
		}
		if _, err := result.file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.Copy(w, result.file); err != nil {
			return err
		}
		if err := result.file.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dumper) dumpTableDataDirect(ctx context.Context, w io.Writer, table string) error {
	var cnt uint64
	var tmp string
	var err error

	if d.LockTables {
		_, err = d.mysqlFlushTable(ctx, table)
		if err != nil {
			return err
		}
	}

	tmp, cnt, err = d.getTableHeader(ctx, table)
	if err != nil {
		return err
	}

	if _, err = fmt.Fprint(w, tmp); err != nil {
		return err
	}

	if cnt > 0 {
		lockStmt := d.getLockTableWriteStatement(table)
		if _, err = fmt.Fprint(w, lockStmt); err != nil {
			return err
		}

		if dErr := d.dumpTableData(ctx, w, table); dErr != nil {
			return dErr
		}

		unlockStmt := d.getUnlockTablesStatement()
		if _, err = fmt.Fprint(w, unlockStmt); err != nil {
			return err
		}
	}

	if d.LockTables {
		if _, dErr := d.mysqlUnlockTables(ctx); dErr != nil {
			return dErr
		}
	}

	return nil
}

func (d *Dumper) dumpTableDataToWriter(ctx context.Context, w io.Writer, table string) error {
	var cnt uint64
	var tmp string
	var err error

	tmp, cnt, err = d.getTableHeader(ctx, table)
	if err != nil {
		return err
	}

	if _, err = fmt.Fprint(w, tmp); err != nil {
		return err
	}

	if cnt > 0 {
		lockStmt := d.getLockTableWriteStatement(table)
		if _, err = fmt.Fprint(w, lockStmt); err != nil {
			return err
		}

		if dErr := d.dumpTableData(ctx, w, table); dErr != nil {
			return dErr
		}

		unlockStmt := d.getUnlockTablesStatement()
		if _, err = fmt.Fprint(w, unlockStmt); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dumper) parseBinaryRelations(table, createTable string) {
	// no cache, if it is requested, replace existing entry
	binaryCols := make([]string, 0)

	scanner := bufio.NewScanner(strings.NewReader(createTable))
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), "binary") || strings.Contains(strings.ToLower(scanner.Text()), "blob") {
			r := regexp.MustCompile("`([^(]*)`")
			columnName := r.FindAllStringSubmatch(scanner.Text(), -1)

			if len(columnName) > 0 && len(columnName[0]) > 0 {
				binaryCols = append(binaryCols, columnName[0][1])
			}
		}
	}

	d.mapMu.Lock()
	d.mapBins[table] = binaryCols
	d.mapMu.Unlock()
}

func (d *Dumper) excludeGeneratedColumns(table, createTable string) string {
	// Track generated columns for exclusion during data dump
	// but always preserve them in the CREATE TABLE statement
	excludedCols := make([]string, 0)

	scanner := bufio.NewScanner(strings.NewReader(createTable))
	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), "generated always") {
			r := regexp.MustCompile("`([^(]*)`")
			columnName := r.FindAllStringSubmatch(scanner.Text(), -1)

			if len(columnName) > 0 && len(columnName[0]) > 0 {
				excludedCols = append(excludedCols, columnName[0][1])
			}
		}
	}

	d.mapMu.Lock()
	d.mapExclusionColumns[table] = excludedCols
	d.mapMu.Unlock()

	return createTable
}

func (d *Dumper) isColumnBinary(table, columnName string) bool {
	columnName = strings.Trim(columnName, "`")
	d.mapMu.RLock()
	val, ok := d.mapBins[table]
	d.mapMu.RUnlock()
	if ok {
		return slices.Contains(val, columnName)
	}

	return false
}

func (d *Dumper) isColumnExcluded(table, columnName string) bool {
	d.mapMu.RLock()
	val, ok := d.mapExclusionColumns[table]
	d.mapMu.RUnlock()
	if ok {
		return slices.Contains(val, columnName)
	}

	return false
}

func (d *Dumper) getTables(ctx context.Context) ([]string, error) {
	tables := make([]string, 0)

	rows, err := d.db.QueryContext(ctx, "SHOW FULL TABLES")
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

	rows, err := d.db.QueryContext(ctx, "SHOW FULL TABLES")
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

	numRows := d.InsertIntoLimit
	if numRows <= 0 {
		if d.Quick {
			numRows = 1
		} else {
			numRows = defaultInsertLimit
		}
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
			if _, err := fmt.Fprintf(w, "%s\n%s;\n", query, strings.Join(data, ",\n")); err != nil {
				return err
			}
			data = make([]string, 0)
		}
	}

	if len(data) > 0 {
		if _, err := fmt.Fprintf(w, "%s\n%s;\n", query, strings.Join(data, ",\n")); err != nil {
			return err
		}
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

			if strings.Contains(val, "faker.") {
				val = replaceStringWithFakerWhenRequested(val)
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

func (d *Dumper) getTableHeader(ctx context.Context, table string) (str string, count uint64, err error) {
	str = fmt.Sprintf("\n--\n-- Data for table `%s`", table)
	count, err = d.rowCount(ctx, table)

	if err != nil {
		return "", 0, err
	}

	str += fmt.Sprintf(" -- %d rows\n--\n\n", count)
	return str, count, nil
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
		return nil, nil, err
	}
	if rows, err = d.db.QueryContext(ctx, selectQuery); err != nil {
		return nil, nil, err
	}

	return rows, columns, nil
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
		return nil, err
	}

	for _, column := range tmp {
		if d.isColumnExcluded(table, column) {
			continue
		}

		replacement, ok := d.SelectMap[strings.ToLower(table)][strings.ToLower(column)]
		if ok && considerRewriteMap {
			if strings.Contains(replacement, "faker.") {
				replacement = fmt.Sprintf("'%s'", replacement)
			}

			columns = append(columns, fmt.Sprintf("%s AS `%s`", replacement, column))
		} else {
			columns = append(columns, fmt.Sprintf("`%s`", column))
		}
	}

	return columns, nil
}

func (d *Dumper) rowCount(ctx context.Context, table string) (count uint64, err error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if where, ok := d.WhereMap[strings.ToLower(table)]; ok {
		query = fmt.Sprintf("%s WHERE %s", query, where)
	}
	row := d.useTransactionOrDBQueryRow(ctx, query)
	if err = row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *Dumper) getCreateTableStatement(table string) (string, error) {
	s := fmt.Sprintf("\n--\n-- Structure for table `%s`\n--\n\n", table)
	s += fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", table)

	schema, err := d.fetchTableSchema(table)
	if err != nil {
		return "", fmt.Errorf("fetch table schema: %w", err)
	}

	// Populate binary and generated column maps for data export
	d.mapMu.Lock()
	d.mapBins[table] = schema.GetBinaryColumns()
	d.mapExclusionColumns[table] = schema.GetGeneratedColumns()
	d.mapMu.Unlock()

	s += schema.BuildCreateTableSQL() + ";\n"
	return s, nil
}

func (d *Dumper) mysqlFlushTable(ctx context.Context, table string) (sql.Result, error) {
	return d.useTransactionOrDBExec(ctx, fmt.Sprintf("FLUSH TABLES `%s` WITH READ LOCK", table))
}

// Release the global read locks
func (d *Dumper) mysqlUnlockTables(ctx context.Context) (sql.Result, error) {
	return d.useTransactionOrDBExec(ctx, "UNLOCK TABLES")
}

func (d *Dumper) useTransactionOrDBQueryRow(ctx context.Context, query string) *sql.Row {
	return d.db.QueryRowContext(ctx, query)
}

func (d *Dumper) useTransactionOrDBExec(ctx context.Context, query string) (sql.Result, error) {
	return d.db.ExecContext(ctx, query)
}

func (d *Dumper) dumpTriggers(ctx context.Context, w io.Writer) error {
	triggers, err := d.getTriggers(ctx)
	if err != nil {
		return err
	}

	for _, trigger := range triggers {
		ddl, err := d.getTrigger(ctx, trigger)

		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "\n--\n-- Trigger `%s`\n--\n\n", trigger); err != nil {
			return err
		}

		// Always use // as delimiter for triggers
		if _, err := fmt.Fprintf(w, "DELIMITER //\n"); err != nil {
			return err
		}

		if _, err := w.Write([]byte(ddl)); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "//\nDELIMITER ;\n"); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dumper) getTriggers(ctx context.Context) ([]string, error) {
	triggers := make([]string, 0)

	rows, err := d.db.QueryContext(ctx, "SHOW TRIGGERS")
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

func (d *Dumper) getTrigger(ctx context.Context, triggerName string) (string, error) {
	var ddl, unknown string

	row := d.useTransactionOrDBQueryRow(ctx, fmt.Sprintf("SHOW CREATE TRIGGER `%s`", triggerName))
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
		ddl, err := d.getView(ctx, view)

		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "\n--\n-- Structure for view `%s`\n--\n\n", view); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "DROP VIEW IF EXISTS `%s`;\n", view); err != nil {
			return err
		}

		if _, err := w.Write([]byte(ddl)); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dumper) getView(ctx context.Context, viewName string) (string, error) {
	var ddl, unknown string

	row := d.useTransactionOrDBQueryRow(ctx, fmt.Sprintf("SHOW CREATE VIEW `%s`", viewName))
	if err := row.Scan(&unknown, &ddl, &unknown, &unknown); err != nil {
		return "", err
	}

	// Always skip definer for portability
	ddl = skipDefinerRegExp.ReplaceAllString(ddl, "")

	return ddl + ";\n", nil
}
