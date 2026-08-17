package mysqldump

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

// TableLimit restricts how many rows of a table are dumped. All tables
// referencing the limited table via foreign keys (directly or transitively)
// are filtered automatically so they only contain rows attached to the kept
// rows. A second limit on a table that is already filtered this way is rejected.
// When the limited table references itself (e.g. product.parent_id), the
// ancestors of the kept rows are dumped too, so the export may hold slightly
// more than Rows rows but stays importable with foreign key checks enabled.
type TableLimit struct {
	// Maximum amount of rows to export for this table
	Rows int `yaml:"rows" jsonschema:"required,minimum=1"`
	// SQL ORDER BY clause deciding which rows are kept (e.g. "created_at DESC" for the newest rows). Defaults to "created_at DESC" when the table has a created_at column
	OrderBy string `yaml:"order_by,omitempty"`
}

// limitDerivedTableAlias names the derived table wrapping the LIMIT query, as
// MySQL rejects LIMIT directly inside an IN subquery.
const limitDerivedTableAlias = "`_sw_cli_limit`"

// computeLimitFilters translates LimitMap into WHERE conditions stored in
// limitWhere. Must run after prefetchAllSchemas, as it walks the foreign keys
// of the cached schemas.
func (d *Dumper) computeLimitFilters(ctx context.Context) error {
	if len(d.LimitMap) == 0 {
		return nil
	}

	schemas := make(map[string]*TableSchema, len(d.schemaCache))
	for name, schema := range d.schemaCache {
		schemas[strings.ToLower(name)] = schema
	}

	limits := make(map[string]TableLimit, len(d.LimitMap))
	for table, limit := range d.LimitMap {
		table = strings.ToLower(table)

		schema, ok := schemas[table]
		if !ok {
			logging.FromContext(ctx).Warnf("Ignoring row limit for table %s: the table does not exist or is excluded from the dump", table)
			continue
		}

		if limit.Rows <= 0 {
			return fmt.Errorf("row limit for table %s must be greater than zero", table)
		}

		if len(schema.PrimaryKey) == 0 {
			return fmt.Errorf("cannot limit rows of table %s: the table has no primary key", table)
		}

		if limit.OrderBy == "" && slices.ContainsFunc(schema.Columns, func(col ColumnSchema) bool { return strings.EqualFold(col.Name, "created_at") }) {
			limit.OrderBy = "`created_at` DESC"
			logging.FromContext(ctx).Infof("Limiting table %s to the %d newest rows by created_at", table, limit.Rows)
		}

		limits[table] = limit
	}

	if len(limits) == 0 {
		return nil
	}

	if err := rejectOverlappingLimits(schemas, limits); err != nil {
		return err
	}

	resolver := &limitResolver{
		whereMap:  d.WhereMap,
		schemas:   schemas,
		limits:    limits,
		affected:  make(map[string]bool),
		memo:      make(map[string]string),
		resolving: make(map[string]bool),
	}
	resolver.markAffectedTables()

	d.limitWhere = make(map[string]string)

	for _, table := range slices.Sorted(maps.Keys(resolver.affected)) {
		if _, isLimited := limits[table]; isLimited {
			primaryKey := schemas[table].PrimaryKey
			d.limitWhere[table] = fmt.Sprintf("%s IN (%s)", columnTuple(primaryKey), resolver.keptRowsQuery(table, primaryKey))
			continue
		}

		if condition := resolver.conditionFor(table); condition != "" {
			d.limitWhere[table] = condition
		}
	}

	if d.LockTables && len(d.limitWhere) > 0 {
		logging.FromContext(ctx).Infof("Skipping table locks for %d tables filtered by row limits, as their dump queries reference other tables", len(d.limitWhere))
	}

	return nil
}

type limitResolver struct {
	whereMap map[string]string
	schemas  map[string]*TableSchema
	limits   map[string]TableLimit
	// affected contains all tables whose rows must be filtered: the limited
	// tables plus every table referencing an affected table via foreign keys.
	affected map[string]bool
	memo     map[string]string
	// resolving guards against foreign key cycles: edges into a table that is
	// currently being resolved are skipped, which keeps a superset of the
	// strictly attached rows instead of recursing forever.
	resolving map[string]bool
}

func (r *limitResolver) markAffectedTables() {
	for table := range r.limits {
		r.affected[table] = true
	}

	markReferencingTables(r.schemas, r.affected)
}

// rejectOverlappingLimits forbids independently limiting a table that is already
// filtered by another limit (directly or transitively via foreign keys).
// Example: --limit order=100 already keeps only order_address rows of those
// orders, so --limit order_address=50 would pick an unrelated set and is rejected.
func rejectOverlappingLimits(schemas map[string]*TableSchema, limits map[string]TableLimit) error {
	if len(limits) < 2 {
		return nil
	}

	limited := slices.Sorted(maps.Keys(limits))
	for _, parent := range limited {
		filtered := map[string]bool{parent: true}
		markReferencingTables(schemas, filtered)

		for _, child := range limited {
			if child != parent && filtered[child] {
				return fmt.Errorf("cannot limit table %s: limiting table %s already filters %s via foreign keys", child, parent, child)
			}
		}
	}

	return nil
}

// markReferencingTables adds every table that references a table already in
// affected, walking foreign keys until the set stops growing.
func markReferencingTables(schemas map[string]*TableSchema, affected map[string]bool) {
	for changed := true; changed; {
		changed = false
		for name, schema := range schemas {
			if affected[name] {
				continue
			}

			for _, fk := range schema.ForeignKeys {
				referenced := strings.ToLower(fk.ReferencedTable)
				if referenced != name && affected[referenced] {
					affected[name] = true
					changed = true
					break
				}
			}
		}
	}
}

// conditionFor returns the WHERE condition restricting the given non-limited
// table to rows attached to the kept rows of its referenced affected tables.
func (r *limitResolver) conditionFor(table string) string {
	if condition, ok := r.memo[table]; ok {
		return condition
	}

	r.resolving[table] = true
	defer delete(r.resolving, table)

	var parts []string
	for _, fk := range r.schemas[table].ForeignKeys {
		referenced := strings.ToLower(fk.ReferencedTable)
		if !r.affected[referenced] || referenced == table || r.resolving[referenced] {
			continue
		}

		if _, isLimited := r.limits[referenced]; !isLimited && r.conditionFor(referenced) == "" {
			continue
		}

		condition := fmt.Sprintf("%s IN (%s)", columnTuple(fk.Columns), r.keptRowsQuery(referenced, fk.ReferencedColumns))

		// Rows not referencing the parent at all are kept.
		if nullable := r.nullableColumns(table, fk.Columns); len(nullable) != 0 {
			nullChecks := make([]string, 0, len(nullable))
			for _, column := range nullable {
				nullChecks = append(nullChecks, fmt.Sprintf("`%s` IS NULL", column))
			}

			condition = fmt.Sprintf("%s OR %s", strings.Join(nullChecks, " OR "), condition)
		}

		parts = append(parts, "("+condition+")")
	}

	condition := strings.Join(parts, " AND ")
	r.memo[table] = condition
	return condition
}

// keptRowsQuery returns a SELECT yielding the given columns of all rows kept
// for the given affected table.
func (r *limitResolver) keptRowsQuery(table string, columns []string) string {
	quotedColumns := quoteColumns(columns)
	tableName := r.schemas[table].Name

	if _, isLimited := r.limits[table]; isLimited {
		// A self-referencing limited table (e.g. product.parent_id -> product.id)
		// must keep the ancestors of every kept row, otherwise a kept variant
		// would reference a parent excluded by the LIMIT and break the foreign
		// key on import. A recursive CTE seeded by the LIMIT walks the parents.
		if selfRefs := r.selfReferences(table); len(selfRefs) != 0 {
			return r.selfReferenceClosureQuery(table, columns, selfRefs)
		}

		return fmt.Sprintf("SELECT %s FROM (%s) %s", quotedColumns, r.limitSeedQuery(table, columns), limitDerivedTableAlias)
	}

	query := fmt.Sprintf("SELECT %s FROM `%s`", quotedColumns, tableName)

	conditions := make([]string, 0, 2)
	if where := r.whereMap[table]; where != "" {
		conditions = append(conditions, "("+where+")")
	}
	if condition := r.conditionFor(table); condition != "" {
		conditions = append(conditions, condition)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	return query
}

// limitSeedQuery returns the plain "SELECT columns FROM table [WHERE] [ORDER BY]
// LIMIT n" query selecting the rows kept by the limit of the given table.
func (r *limitResolver) limitSeedQuery(table string, columns []string) string {
	limit := r.limits[table]

	query := fmt.Sprintf("SELECT %s FROM `%s`", quoteColumns(columns), r.schemas[table].Name)
	if where := r.whereMap[table]; where != "" {
		query += " WHERE " + where
	}
	if limit.OrderBy != "" {
		query += " ORDER BY " + limit.OrderBy
	}
	query += fmt.Sprintf(" LIMIT %d", limit.Rows)

	return query
}

// selfReferences returns the foreign keys of the given table that point back at
// the same table (e.g. product.parent_id -> product.id).
func (r *limitResolver) selfReferences(table string) []ForeignKeySchema {
	var selfRefs []ForeignKeySchema
	for _, fk := range r.schemas[table].ForeignKeys {
		if strings.EqualFold(fk.ReferencedTable, table) {
			selfRefs = append(selfRefs, fk)
		}
	}

	return selfRefs
}

// selfReferenceCTEAlias names the recursive CTE that closes a limited table over
// its self-references.
const selfReferenceCTEAlias = "`_sw_cli_kept`"

// selfReferenceClosureQuery returns a SELECT yielding the given columns of all
// rows kept for a limited, self-referencing table: the rows selected by the
// LIMIT plus, recursively, the parent rows they reference through selfRefs. The
// recursive CTE guarantees the kept set is closed under the self-references, so
// the dump imports without dangling foreign keys.
func (r *limitResolver) selfReferenceClosureQuery(table string, columns []string, selfRefs []ForeignKeySchema) string {
	primaryKey := r.schemas[table].PrimaryKey
	tableName := r.schemas[table].Name
	pkColumns := quoteColumns(primaryKey)

	// Each recursive term walks one self-reference upwards: it joins the kept
	// rows back to the base table to recover their foreign key columns, then
	// joins to the referenced parent rows and keeps their primary keys.
	const childAlias, parentAlias = "`_sw_child`", "`_sw_parent`"
	recursiveTerms := make([]string, 0, len(selfRefs))
	for _, fk := range selfRefs {
		keptJoin := make([]string, len(primaryKey))
		for i, column := range primaryKey {
			keptJoin[i] = fmt.Sprintf("%s.`%s` = %s.`%s`", childAlias, column, selfReferenceCTEAlias, column)
		}

		parentJoin := make([]string, len(fk.Columns))
		for i, column := range fk.Columns {
			parentJoin[i] = fmt.Sprintf("%s.`%s` = %s.`%s`", parentAlias, fk.ReferencedColumns[i], childAlias, column)
		}

		recursiveTerms = append(recursiveTerms, fmt.Sprintf(
			"SELECT %s FROM `%s` %s JOIN %s ON %s JOIN `%s` %s ON %s",
			prefixColumns(parentAlias, primaryKey),
			tableName, childAlias,
			selfReferenceCTEAlias, strings.Join(keptJoin, " AND "),
			tableName, parentAlias, strings.Join(parentJoin, " AND "),
		))
	}

	seed := fmt.Sprintf("SELECT %s FROM (%s) %s", pkColumns, r.limitSeedQuery(table, primaryKey), limitDerivedTableAlias)

	// The CTE carries only the primary key of the kept rows. When the caller
	// wants exactly the primary key (the common case) it is selected directly;
	// otherwise the requested columns are fetched by joining the kept keys back
	// to the base table, which also covers foreign keys referencing non-primary
	// key columns.
	projection := fmt.Sprintf("%s FROM %s", quoteColumns(columns), selfReferenceCTEAlias)
	if !slices.Equal(columns, primaryKey) {
		keptJoin := make([]string, len(primaryKey))
		for i, column := range primaryKey {
			keptJoin[i] = fmt.Sprintf("`%s`.`%s` = %s.`%s`", tableName, column, selfReferenceCTEAlias, column)
		}

		projection = fmt.Sprintf("%s FROM `%s` JOIN %s ON %s", prefixColumns("`"+tableName+"`", columns), tableName, selfReferenceCTEAlias, strings.Join(keptJoin, " AND "))
	}

	return fmt.Sprintf(
		"WITH RECURSIVE %s (%s) AS (%s UNION %s) SELECT %s",
		selfReferenceCTEAlias, pkColumns, seed, strings.Join(recursiveTerms, " UNION "), projection,
	)
}

// prefixColumns quotes each column and prefixes it with the given already-quoted
// table reference, e.g. `_sw_parent`.`id`, `_sw_parent`.`version_id`.
func prefixColumns(tableRef string, columns []string) string {
	prefixed := make([]string, len(columns))
	for i, column := range columns {
		prefixed[i] = fmt.Sprintf("%s.`%s`", tableRef, column)
	}

	return strings.Join(prefixed, ", ")
}

func (r *limitResolver) nullableColumns(table string, columns []string) []string {
	var nullable []string
	for _, column := range columns {
		for _, col := range r.schemas[table].Columns {
			if col.Name == column && col.Nullable {
				nullable = append(nullable, column)
				break
			}
		}
	}

	return nullable
}

func columnTuple(columns []string) string {
	if len(columns) == 1 {
		return "`" + columns[0] + "`"
	}

	return "(" + quoteColumns(columns) + ")"
}

func quoteColumns(columns []string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = "`" + column + "`"
	}

	return strings.Join(quoted, ", ")
}
