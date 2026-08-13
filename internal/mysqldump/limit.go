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
// rows.
type TableLimit struct {
	// Rows is the maximum number of rows to dump.
	Rows int
	// OrderBy decides which rows are kept (e.g. "`created_at` DESC" for the
	// latest rows). When empty and the table has a created_at column,
	// "`created_at` DESC" is used.
	OrderBy string
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

	if limit, isLimited := r.limits[table]; isLimited {
		query := fmt.Sprintf("SELECT %s FROM `%s`", quotedColumns, tableName)
		if where := r.whereMap[table]; where != "" {
			query += " WHERE " + where
		}
		if limit.OrderBy != "" {
			query += " ORDER BY " + limit.OrderBy
		}
		query += fmt.Sprintf(" LIMIT %d", limit.Rows)

		return fmt.Sprintf("SELECT %s FROM (%s) %s", quotedColumns, query, limitDerivedTableAlias)
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
