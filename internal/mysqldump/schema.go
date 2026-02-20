package mysqldump

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type TableSchema struct {
	Name             string
	Engine           string
	Charset          string
	Collation        string
	Comment          string
	RowFormat        string
	AutoIncrement    sql.NullInt64
	Columns          []ColumnSchema
	PrimaryKey       []string
	Indexes          []IndexSchema
	ForeignKeys      []ForeignKeySchema
	CheckConstraints []CheckConstraintSchema
}

type ColumnSchema struct {
	Name           string
	Type           string
	CharacterSet   sql.NullString
	Nullable       bool
	Default        sql.NullString
	Extra          string
	Collation      sql.NullString
	Comment        string
	GenerationExpr sql.NullString
	IsVirtual      bool
	CheckExpr      string
}

func (c *ColumnSchema) IsBinary() bool {
	lower := strings.ToLower(c.Type)
	return strings.Contains(lower, "binary") || strings.Contains(lower, "blob")
}

func (c *ColumnSchema) IsGenerated() bool {
	return c.GenerationExpr.Valid && c.GenerationExpr.String != ""
}

func (schema *TableSchema) GetBinaryColumns() []string {
	var cols []string
	for _, col := range schema.Columns {
		if col.IsBinary() {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

func (schema *TableSchema) GetGeneratedColumns() []string {
	var cols []string
	for _, col := range schema.Columns {
		if col.IsGenerated() {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

type IndexSchema struct {
	Name       string
	Columns    []IndexColumnSchema
	IsUnique   bool
	Type       string
	Comment    string
	SeqInTable int
}

type IndexColumnSchema struct {
	Name    string
	SubPart sql.NullInt64
	Order   string
}

type ForeignKeySchema struct {
	Name              string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	OnDelete          string
	OnUpdate          string
}

type CheckConstraintSchema struct {
	Name       string
	Expression string
}

var collationMap = map[string]string{
	"utf8mb4_0900_ai_ci": "utf8mb4_unicode_ci",
	"utf8mb4_0900_as_ci": "utf8mb4_unicode_ci",
	"utf8mb4_0900_as_cs": "utf8mb4_bin",
}

func mapCollation(collation string) string {
	if mapped, ok := collationMap[collation]; ok {
		return mapped
	}
	return collation
}

func (d *Dumper) prefetchAllSchemas(ctx context.Context, tables []string) error {
	d.schemaCache = make(map[string]*TableSchema, len(tables))

	for _, table := range tables {
		d.schemaCache[table] = &TableSchema{Name: table}
	}

	if err := d.fetchAllTableMetadata(ctx); err != nil {
		return fmt.Errorf("fetch table metadata: %w", err)
	}

	if err := d.fetchAllColumns(ctx); err != nil {
		return fmt.Errorf("fetch columns: %w", err)
	}

	if err := d.fetchAllIndexes(ctx); err != nil {
		return fmt.Errorf("fetch indexes: %w", err)
	}

	if err := d.fetchAllForeignKeys(ctx); err != nil {
		return fmt.Errorf("fetch foreign keys: %w", err)
	}

	if err := d.fetchAllCheckConstraints(ctx); err != nil {
		return fmt.Errorf("fetch check constraints: %w", err)
	}

	return nil
}

func (d *Dumper) fetchTableSchema(tableName string) (*TableSchema, error) {
	if schema, ok := d.schemaCache[tableName]; ok {
		return schema, nil
	}
	return nil, fmt.Errorf("table %s not found in schema cache", tableName)
}

func (schema *TableSchema) BuildCreateTableSQL() string {
	var b strings.Builder

	b.WriteString("CREATE TABLE `")
	b.WriteString(schema.Name)
	b.WriteString("` (\n")

	schema.writeColumns(&b)
	schema.writePrimaryKey(&b)
	schema.writeIndexes(&b)
	schema.writeForeignKeys(&b)
	schema.writeCheckConstraints(&b)
	schema.writeTableOptions(&b)

	return b.String()
}

func (schema *TableSchema) hasConstraints() bool {
	return len(schema.PrimaryKey) > 0 || len(schema.Indexes) > 0 || len(schema.ForeignKeys) > 0 || len(schema.CheckConstraints) > 0
}

func (schema *TableSchema) writeColumns(b *strings.Builder) {
	for i, col := range schema.Columns {
		b.WriteString("  `")
		b.WriteString(col.Name)
		b.WriteString("` ")
		b.WriteString(col.Type)

		col.writeCharsetAndCollation(b, schema.Collation)
		col.writeGeneratedOrDefault(b)
		col.writeExtra(b)

		if col.CheckExpr != "" {
			b.WriteString(" CHECK (")
			b.WriteString(col.CheckExpr)
			b.WriteString(")")
		}

		if col.Comment != "" {
			b.WriteString(" COMMENT '")
			b.WriteString(escapeString(col.Comment))
			b.WriteString("'")
		}

		if i < len(schema.Columns)-1 || schema.hasConstraints() {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
}

func (col *ColumnSchema) writeCharsetAndCollation(b *strings.Builder, tableCollation string) {
	if !isStringType(col.Type) || !col.Collation.Valid || col.Collation.String == "" || col.Collation.String == tableCollation {
		return
	}
	if col.CharacterSet.Valid && col.CharacterSet.String != "" {
		b.WriteString(" CHARACTER SET ")
		b.WriteString(col.CharacterSet.String)
	}
	b.WriteString(" COLLATE ")
	b.WriteString(col.Collation.String)
}

func (col *ColumnSchema) writeGeneratedOrDefault(b *strings.Builder) {
	if col.GenerationExpr.Valid && col.GenerationExpr.String != "" {
		b.WriteString(" GENERATED ALWAYS AS (")
		// INFORMATION_SCHEMA can return escaped quotes in generation expressions.
		b.WriteString(unescape(col.GenerationExpr.String))
		b.WriteString(")")
		if col.IsVirtual {
			b.WriteString(" VIRTUAL")
		} else {
			b.WriteString(" STORED")
		}
		return
	}

	if !col.Nullable {
		b.WriteString(" NOT NULL")
	}

	if col.Default.Valid && col.Default.String != sqlNull {
		b.WriteString(" DEFAULT ")
		b.WriteString(formatDefault(col.Default.String, col.Type))
	}
}

func (col *ColumnSchema) writeExtra(b *strings.Builder) {
	if col.Extra == "" || strings.Contains(col.Extra, "GENERATED") {
		return
	}
	extra := col.Extra
	extra = strings.ReplaceAll(extra, "DEFAULT_GENERATED", "")
	extra = strings.TrimSpace(extra)
	if extra != "" {
		b.WriteString(" ")
		extra = strings.ReplaceAll(extra, "auto_increment", "AUTO_INCREMENT")
		b.WriteString(extra)
	}
}

func (schema *TableSchema) writePrimaryKey(b *strings.Builder) {
	if len(schema.PrimaryKey) == 0 {
		return
	}
	b.WriteString("  PRIMARY KEY (")
	for i, col := range schema.PrimaryKey {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("`")
		b.WriteString(col)
		b.WriteString("`")
	}
	b.WriteString(")")
	if len(schema.Indexes) > 0 || len(schema.ForeignKeys) > 0 || len(schema.CheckConstraints) > 0 {
		b.WriteString(",")
	}
	b.WriteString("\n")
}

func (schema *TableSchema) writeIndexes(b *strings.Builder) {
	for i, idx := range schema.Indexes {
		switch {
		case idx.IsUnique:
			b.WriteString("  UNIQUE KEY `")
		case idx.Type == "FULLTEXT":
			b.WriteString("  FULLTEXT KEY `")
		case idx.Type == "SPATIAL":
			b.WriteString("  SPATIAL KEY `")
		default:
			b.WriteString("  KEY `")
		}
		b.WriteString(idx.Name)
		b.WriteString("` (")

		for j, col := range idx.Columns {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("`")
			b.WriteString(col.Name)
			b.WriteString("`")
			if col.SubPart.Valid {
				fmt.Fprintf(b, "(%d)", col.SubPart.Int64)
			}
		}
		b.WriteString(")")

		if idx.Comment != "" {
			b.WriteString(" COMMENT '")
			b.WriteString(escapeString(idx.Comment))
			b.WriteString("'")
		}

		if i < len(schema.Indexes)-1 || len(schema.ForeignKeys) > 0 || len(schema.CheckConstraints) > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
}

func (schema *TableSchema) writeForeignKeys(b *strings.Builder) {
	for i, fk := range schema.ForeignKeys {
		b.WriteString("  CONSTRAINT `")
		b.WriteString(fk.Name)
		b.WriteString("` FOREIGN KEY (")

		for j, col := range fk.Columns {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("`")
			b.WriteString(col)
			b.WriteString("`")
		}

		b.WriteString(") REFERENCES `")
		b.WriteString(fk.ReferencedTable)
		b.WriteString("` (")

		for j, col := range fk.ReferencedColumns {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("`")
			b.WriteString(col)
			b.WriteString("`")
		}
		b.WriteString(")")

		if fk.OnDelete != "" && fk.OnDelete != "RESTRICT" && fk.OnDelete != "NO ACTION" {
			b.WriteString(" ON DELETE ")
			b.WriteString(fk.OnDelete)
		}
		if fk.OnUpdate != "" && fk.OnUpdate != "RESTRICT" && fk.OnUpdate != "NO ACTION" {
			b.WriteString(" ON UPDATE ")
			b.WriteString(fk.OnUpdate)
		}

		if i < len(schema.ForeignKeys)-1 || len(schema.CheckConstraints) > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
}

func (schema *TableSchema) writeCheckConstraints(b *strings.Builder) {
	for i, cc := range schema.CheckConstraints {
		b.WriteString("  CONSTRAINT `")
		b.WriteString(cc.Name)
		b.WriteString("` CHECK (")
		b.WriteString(cc.Expression)
		b.WriteString(")")

		if i < len(schema.CheckConstraints)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
}

func (schema *TableSchema) writeTableOptions(b *strings.Builder) {
	b.WriteString(") ENGINE=")
	b.WriteString(schema.Engine)

	if schema.AutoIncrement.Valid && schema.AutoIncrement.Int64 > 1 {
		fmt.Fprintf(b, " AUTO_INCREMENT=%d", schema.AutoIncrement.Int64)
	}

	if schema.Charset != "" {
		b.WriteString(" DEFAULT CHARSET=")
		b.WriteString(schema.Charset)
	}

	if schema.Collation != "" {
		b.WriteString(" COLLATE=")
		b.WriteString(schema.Collation)
	}

	if schema.RowFormat != "" && schema.RowFormat != "Dynamic" {
		b.WriteString(" ROW_FORMAT=")
		b.WriteString(schema.RowFormat)
	}

	if schema.Comment != "" {
		b.WriteString(" COMMENT='")
		b.WriteString(escapeString(schema.Comment))
		b.WriteString("'")
	}
}

func isStringType(colType string) bool {
	lower := strings.ToLower(colType)
	return strings.HasPrefix(lower, "varchar") ||
		strings.HasPrefix(lower, "char") ||
		strings.HasPrefix(lower, "text") ||
		strings.HasPrefix(lower, "tinytext") ||
		strings.HasPrefix(lower, "mediumtext") ||
		strings.HasPrefix(lower, "longtext") ||
		strings.HasPrefix(lower, "enum") ||
		strings.HasPrefix(lower, "set")
}

var hexLiteralRegex = regexp.MustCompile(`^(?i)(x'[0-9a-f]*'|0x[0-9a-f]+)$`)

const sqlNull = "NULL"

func formatDefault(value string, colType string) string {
	lower := strings.ToLower(colType)

	if strings.ToUpper(value) == sqlNull {
		return sqlNull
	}

	if hexLiteralRegex.MatchString(value) {
		return value
	}

	if isExpression(value) {
		if isSpecialDefault(value) {
			return value
		}
		return "(" + value + ")"
	}

	if isNumericType(lower) {
		return value
	}

	return "'" + escapeString(value) + "'"
}

func isExpression(value string) bool {
	if isSpecialDefault(value) {
		return true
	}
	if strings.Contains(value, "(") && strings.Contains(value, ")") {
		return true
	}
	return false
}

func isSpecialDefault(value string) bool {
	upper := strings.ToUpper(value)
	switch upper {
	case "CURRENT_TIMESTAMP", "CURRENT_DATE", "CURRENT_TIME", "LOCALTIME", "LOCALTIMESTAMP", "NOW()":
		return true
	}
	return false
}

func isNumericType(colType string) bool {
	return strings.HasPrefix(colType, "int") ||
		strings.HasPrefix(colType, "tinyint") ||
		strings.HasPrefix(colType, "smallint") ||
		strings.HasPrefix(colType, "mediumint") ||
		strings.HasPrefix(colType, "bigint") ||
		strings.HasPrefix(colType, "float") ||
		strings.HasPrefix(colType, "double") ||
		strings.HasPrefix(colType, "decimal") ||
		strings.HasPrefix(colType, "numeric") ||
		colType == "bit"
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func (d *Dumper) fetchAllTableMetadata(ctx context.Context) error {
	query := `
		SELECT
			TABLE_NAME,
			ENGINE,
			TABLE_COLLATION,
			TABLE_COMMENT,
			ROW_FORMAT,
			AUTO_INCREMENT
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tableName string
		var engine, collation, comment, rowFormat sql.NullString
		var autoIncrement sql.NullInt64

		if err := rows.Scan(&tableName, &engine, &collation, &comment, &rowFormat, &autoIncrement); err != nil {
			return err
		}

		schema, ok := d.schemaCache[tableName]
		if !ok {
			continue
		}

		schema.Engine = engine.String
		schema.AutoIncrement = autoIncrement
		if collation.Valid {
			schema.Collation = mapCollation(collation.String)
			if idx := strings.Index(collation.String, "_"); idx > 0 {
				schema.Charset = collation.String[:idx]
			}
		}
		schema.Comment = comment.String
		schema.RowFormat = rowFormat.String
	}

	return rows.Err()
}

func (d *Dumper) fetchAllColumns(ctx context.Context) error {
	query := `
		SELECT
			TABLE_NAME,
			COLUMN_NAME,
			COLUMN_TYPE,
			CHARACTER_SET_NAME,
			IS_NULLABLE,
			COLUMN_DEFAULT,
			EXTRA,
			COLLATION_NAME,
			COLUMN_COMMENT,
			GENERATION_EXPRESSION
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, ORDINAL_POSITION`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tableName string
		var col ColumnSchema
		var nullable string
		var generationExpr sql.NullString

		err := rows.Scan(
			&tableName,
			&col.Name,
			&col.Type,
			&col.CharacterSet,
			&nullable,
			&col.Default,
			&col.Extra,
			&col.Collation,
			&col.Comment,
			&generationExpr,
		)
		if err != nil {
			return err
		}

		schema, ok := d.schemaCache[tableName]
		if !ok {
			continue
		}

		col.Nullable = nullable == "YES"
		col.GenerationExpr = generationExpr

		if generationExpr.Valid && generationExpr.String != "" {
			col.IsVirtual = strings.Contains(strings.ToUpper(col.Extra), "VIRTUAL")
		}

		if col.Collation.Valid {
			col.Collation.String = mapCollation(col.Collation.String)
		}

		if col.Type == "int" {
			col.Type = "int(11)"
		}

		schema.Columns = append(schema.Columns, col)
	}

	return rows.Err()
}

func (d *Dumper) fetchAllIndexes(ctx context.Context) error {
	query := `
		SELECT
			TABLE_NAME,
			INDEX_NAME,
			COLUMN_NAME,
			NON_UNIQUE,
			INDEX_TYPE,
			SUB_PART,
			COLLATION,
			INDEX_COMMENT,
			SEQ_IN_INDEX
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	indexMaps := make(map[string]map[string]*IndexSchema)
	indexFirstSeen := make(map[string]map[string]int)
	seqCounters := make(map[string]int)

	for rows.Next() {
		var tableName, indexName, columnName, indexType string
		var nonUnique int
		var subPart sql.NullInt64
		var collation sql.NullString
		var comment string
		var seqInIndex int

		err := rows.Scan(&tableName, &indexName, &columnName, &nonUnique, &indexType, &subPart, &collation, &comment, &seqInIndex)
		if err != nil {
			return err
		}

		schema, ok := d.schemaCache[tableName]
		if !ok {
			continue
		}

		if indexName == "PRIMARY" {
			schema.PrimaryKey = append(schema.PrimaryKey, columnName)
			continue
		}

		if indexMaps[tableName] == nil {
			indexMaps[tableName] = make(map[string]*IndexSchema)
			indexFirstSeen[tableName] = make(map[string]int)
			seqCounters[tableName] = 0
		}

		idx, exists := indexMaps[tableName][indexName]
		if !exists {
			idx = &IndexSchema{
				Name:       indexName,
				IsUnique:   nonUnique == 0,
				Type:       indexType,
				Comment:    comment,
				SeqInTable: seqCounters[tableName],
			}
			indexMaps[tableName][indexName] = idx
			indexFirstSeen[tableName][indexName] = seqCounters[tableName]
			seqCounters[tableName]++
		}

		order := "ASC"
		if collation.Valid && collation.String == "D" {
			order = "DESC"
		}

		idx.Columns = append(idx.Columns, IndexColumnSchema{
			Name:    columnName,
			SubPart: subPart,
			Order:   order,
		})
	}

	for tableName, idxMap := range indexMaps {
		schema := d.schemaCache[tableName]
		schema.Indexes = make([]IndexSchema, 0, len(idxMap))
		for name, idx := range idxMap {
			idx.SeqInTable = indexFirstSeen[tableName][name]
			schema.Indexes = append(schema.Indexes, *idx)
		}
		for i := 0; i < len(schema.Indexes); i++ {
			for j := i + 1; j < len(schema.Indexes); j++ {
				if schema.Indexes[i].SeqInTable > schema.Indexes[j].SeqInTable {
					schema.Indexes[i], schema.Indexes[j] = schema.Indexes[j], schema.Indexes[i]
				}
			}
		}
	}

	return rows.Err()
}

func (d *Dumper) fetchAllForeignKeys(ctx context.Context) error {
	query := `
		SELECT DISTINCT
			kcu.TABLE_NAME,
			kcu.CONSTRAINT_NAME,
			kcu.COLUMN_NAME,
			kcu.REFERENCED_TABLE_NAME,
			kcu.REFERENCED_COLUMN_NAME,
			rc.UPDATE_RULE,
			rc.DELETE_RULE,
			kcu.ORDINAL_POSITION
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc
			ON kcu.CONSTRAINT_NAME = rc.CONSTRAINT_NAME
			AND kcu.TABLE_SCHEMA = rc.CONSTRAINT_SCHEMA
		WHERE kcu.TABLE_SCHEMA = DATABASE()
			AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY kcu.TABLE_NAME, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	fkMaps := make(map[string]map[string]*ForeignKeySchema)
	fkOrders := make(map[string][]string)
	seenCols := make(map[string]map[string]map[string]bool)

	for rows.Next() {
		var tableName, constraintName, columnName, refTable, refColumn, onUpdate, onDelete string
		var ordinalPosition int

		err := rows.Scan(&tableName, &constraintName, &columnName, &refTable, &refColumn, &onUpdate, &onDelete, &ordinalPosition)
		if err != nil {
			return err
		}

		_, ok := d.schemaCache[tableName]
		if !ok {
			continue
		}

		if fkMaps[tableName] == nil {
			fkMaps[tableName] = make(map[string]*ForeignKeySchema)
			fkOrders[tableName] = []string{}
			seenCols[tableName] = make(map[string]map[string]bool)
		}

		fk, exists := fkMaps[tableName][constraintName]
		if !exists {
			fk = &ForeignKeySchema{
				Name:            constraintName,
				ReferencedTable: refTable,
				OnUpdate:        onUpdate,
				OnDelete:        onDelete,
			}
			fkMaps[tableName][constraintName] = fk
			fkOrders[tableName] = append(fkOrders[tableName], constraintName)
			seenCols[tableName][constraintName] = make(map[string]bool)
		}

		if seenCols[tableName][constraintName][columnName] {
			continue
		}
		seenCols[tableName][constraintName][columnName] = true

		fk.Columns = append(fk.Columns, columnName)
		fk.ReferencedColumns = append(fk.ReferencedColumns, refColumn)
	}

	for tableName, fkMap := range fkMaps {
		schema := d.schemaCache[tableName]
		for _, name := range fkOrders[tableName] {
			fk := *fkMap[name]
			// Fixes for unnamed foreign keys with numeric names
			if _, err := strconv.Atoi(fk.Name); err == nil && len(fk.Columns) > 0 {
				fk.Name = "fk." + tableName + "." + fk.Columns[0]
			}
			schema.ForeignKeys = append(schema.ForeignKeys, fk)
		}
	}

	return rows.Err()
}

func (d *Dumper) fetchAllCheckConstraints(ctx context.Context) error {
	query := `
		SELECT DISTINCT
			tc.TABLE_NAME,
			tc.CONSTRAINT_NAME,
			cc.CHECK_CLAUSE
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS cc
			ON tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
			AND tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
		WHERE tc.TABLE_SCHEMA = DATABASE()
			AND tc.CONSTRAINT_TYPE = 'CHECK'
		ORDER BY tc.TABLE_NAME, LENGTH(tc.CONSTRAINT_NAME) DESC`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		// CHECK_CONSTRAINTS table doesn't exist in MySQL < 8.0.16, so ignore errors
		return nil
	}
	defer func() { _ = rows.Close() }()

	seenExpr := make(map[string]map[string]bool)

	for rows.Next() {
		var tableName, name, expr string
		if err := rows.Scan(&tableName, &name, &expr); err != nil {
			return err
		}

		schema, ok := d.schemaCache[tableName]
		if !ok {
			continue
		}

		// The INFORMATION_SCHEMA.CHECK_CONSTRAINTS.CHECK_CLAUSE column contains escaped strings which we must unescape.
		//
		// For example a check like (('A' = 'A')) is stored as (_utf8mb4\'A\' = _utf8mb4\'A\') in CHECK_CLAUSE.
		expr = unescape(expr)

		if seenExpr[tableName] == nil {
			seenExpr[tableName] = make(map[string]bool)
		}
		if seenExpr[tableName][expr] {
			continue
		}
		seenExpr[tableName][expr] = true

		// If constraint name matches a column name, store as inline CHECK on the column
		isColumnName := false
		for i, col := range schema.Columns {
			if col.Name == name {
				schema.Columns[i].CheckExpr = expr
				isColumnName = true
				break
			}
		}
		if isColumnName {
			continue
		}

		schema.CheckConstraints = append(schema.CheckConstraints, CheckConstraintSchema{
			Name:       name,
			Expression: expr,
		})
	}

	return rows.Err()
}
