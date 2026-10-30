package mysqldump

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

func (d *Dumper) validateExcludedReferences(ctx context.Context, schemas map[string]*TableSchema) error {
	if err := d.validateOverlappingDeleteActions(schemas); err != nil {
		return err
	}
	if d.db == nil {
		return nil
	}
	for _, table := range slices.Sorted(maps.Keys(schemas)) {
		if d.filterMap[table] == IgnoreMapPlacement || d.filterMap[table] == NoDataMapPlacement {
			continue
		}
		schema := schemas[table]
		for _, fk := range schema.ForeignKeys {
			parent := schemas[strings.ToLower(fk.ReferencedTable)]
			if err := d.validateExcludedReference(ctx, schema, parent, fk); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateExcludedReference checks that the exported child keys still have a
// retained parent, accounting for omitted rows and configured or generated NULLs.
func (d *Dumper) validateExcludedReference(ctx context.Context, schema, parent *TableSchema, fk ForeignKeySchema) error {
	table := strings.ToLower(schema.Name)
	referenced := strings.ToLower(fk.ReferencedTable)
	if parent == nil {
		return fmt.Errorf(
			"cannot validate foreign key %s on table %s: schema for referenced table %s is unavailable",
			fk.Name,
			schema.Name,
			fk.ReferencedTable,
		)
	}
	if err := d.validateKeyRewrites(schema, parent, fk); err != nil {
		return err
	}

	parentKeysChanged := d.hasKeyProjection(referenced, fk.ReferencedColumns)
	childKeysRewritten := d.hasConfiguredKeyRewrite(table, fk.Columns)
	parentRowsOmitted := d.excludeWhere[referenced] != "" ||
		d.filterMap[referenced] == NoDataMapPlacement
	if !parentRowsOmitted && !parentKeysChanged && !childKeysRewritten {
		return nil
	}
	// Skip only when SET NULL was actually planned. A structure-only
	// parent can be omitted without generating a nulling projection.
	if strings.EqualFold(fk.OnDelete, "SET NULL") && d.excludeWhere[referenced] != "" &&
		!childKeysRewritten && !parentKeysChanged {
		return nil
	}

	query := d.foreignKeyValidationQuery(schema, parent, fk)
	var dangling bool
	if err := d.db.QueryRowContext(ctx, query).Scan(&dangling); err != nil {
		return fmt.Errorf(
			"cannot validate foreign key %s on table %s: %w",
			fk.Name,
			schema.Name,
			err,
		)
	}

	if dangling {
		return fmt.Errorf(
			"cannot dump table %s: retained rows reference excluded rows in %s via foreign key %s",
			schema.Name,
			parent.Name,
			fk.Name,
		)
	}
	return nil
}

// validateKeyRewrites limits key changes to nullable NULL values. General SQL
// rewrites can compare differently after import's type and collation conversions.
func (d *Dumper) validateKeyRewrites(child, parent *TableSchema, fk ForeignKeySchema) error {
	var reason string
	switch {
	case d.hasRecomputedKey(child, fk.Columns) ||
		d.hasRecomputedKey(parent, fk.ReferencedColumns):
		reason = "generated key columns on rewritten tables are not supported"
	case d.hasUnsupportedKeyRewrite(child, fk.Columns) ||
		d.hasUnsupportedKeyRewrite(parent, fk.ReferencedColumns):
		reason = "only NULL rewrites of nullable key columns are supported"
	default:
		return nil
	}
	return fmt.Errorf(
		"cannot validate foreign key %s on table %s: %s",
		fk.Name,
		child.Name,
		reason,
	)
}

// foreignKeyValidationQuery checks the exported keys, including NULL projections,
// against retained parent keys. Any NULL component exempts a composite FK.
func (d *Dumper) foreignKeyValidationQuery(child, parent *TableSchema, fk ForeignKeySchema) string {
	// Preserve the indexed exclusion lookup for unchanged primary keys. Other
	// indexes may be nonunique, allowing an excluded key to have retained matches.
	if slices.Equal(parent.PrimaryKey, fk.ReferencedColumns) &&
		!d.hasKeyProjection(child.Name, fk.Columns) &&
		!d.hasKeyProjection(parent.Name, fk.ReferencedColumns) {
		condition := fmt.Sprintf(
			"%s IN (%s)",
			columnTuple(fk.Columns),
			d.excludedRowsQueryForDump(parent, fk.ReferencedColumns),
		)
		if where := d.effectiveWhere(child.Name); where != "" {
			condition = "(" + where + ") AND (" + condition + ")"
		}
		return fmt.Sprintf(
			"SELECT EXISTS (SELECT 1 FROM `%s` WHERE %s)",
			child.Name,
			condition,
		)
	}

	const childAlias, parentAlias = "`_sw_cli_child`", "`_sw_cli_parent`"
	var nonNull, matches []string
	for i, column := range fk.Columns {
		childKey := prefixColumns(childAlias, []string{column})
		parentKey := prefixColumns(parentAlias, []string{fk.ReferencedColumns[i]})
		nonNull = append(nonNull, childKey+" IS NOT NULL")
		matches = append(matches, parentKey+" = "+childKey)
	}
	return fmt.Sprintf(
		"SELECT EXISTS (SELECT 1 FROM (%s) AS %s WHERE %s "+
			"AND NOT EXISTS (SELECT 1 FROM (%s) AS %s WHERE %s))",
		d.projectedKeysQuery(child, fk.Columns),
		childAlias,
		strings.Join(nonNull, " AND "),
		d.projectedKeysQuery(parent, fk.ReferencedColumns),
		parentAlias,
		strings.Join(matches, " AND "),
	)
}

func (d *Dumper) hasConfiguredKeyRewrite(table string, columns []string) bool {
	rewrites := d.SelectMap[strings.ToLower(table)]
	for _, column := range columns {
		if _, ok := rewrites[strings.ToLower(column)]; ok {
			return true
		}
	}
	return false
}

func (d *Dumper) hasUnsupportedKeyRewrite(schema *TableSchema, columns []string) bool {
	rewrites := d.SelectMap[strings.ToLower(schema.Name)]
	for _, column := range schema.Columns {
		if !slices.Contains(columns, column.Name) {
			continue
		}
		replacement, rewritten := rewrites[strings.ToLower(column.Name)]
		if rewritten && (!column.Nullable || !strings.EqualFold(strings.TrimSpace(replacement), "NULL")) {
			return true
		}
	}
	return false
}

func (d *Dumper) hasKeyProjection(table string, columns []string) bool {
	table = strings.ToLower(table)
	if d.hasConfiguredKeyRewrite(table, columns) {
		return true
	}
	for _, column := range columns {
		if len(d.excludeSetNull[table][strings.ToLower(column)]) != 0 {
			return true
		}
	}
	return false
}

// hasRecomputedKey rejects generated keys whose inputs may change on import.
// Source values cannot validate them without interpreting the generated expression.
func (d *Dumper) hasRecomputedKey(schema *TableSchema, columns []string) bool {
	table := strings.ToLower(schema.Name)
	if len(d.SelectMap[table]) == 0 && len(d.excludeSetNull[table]) == 0 {
		return false
	}
	return slices.ContainsFunc(schema.Columns, func(column ColumnSchema) bool {
		return column.IsGenerated() && slices.Contains(columns, column.Name)
	})
}

func (d *Dumper) projectedKeysQuery(schema *TableSchema, columns []string) string {
	projections := make([]string, len(columns))
	for i, column := range columns {
		projections[i] = fmt.Sprintf(
			"%s AS `%s`",
			d.selectColumnExpression(schema.Name, column),
			column,
		)
	}
	query := fmt.Sprintf("SELECT %s FROM `%s`", strings.Join(projections, ", "), schema.Name)
	if d.filterMap[strings.ToLower(schema.Name)] == NoDataMapPlacement {
		return query + " WHERE FALSE"
	}
	if where := d.effectiveWhere(schema.Name); where != "" {
		query += " WHERE " + where
	}
	return query
}
