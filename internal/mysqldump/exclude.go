package mysqldump

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

// computeExcludeFilters translates ExcludeMap into the WHERE conditions stored
// in excludeWhere: the seeded tables keep only rows not matching their exclude
// condition, and every table referencing an affected table via foreign keys
// (directly or transitively) keeps only rows that do not reference an excluded
// row. Must run after prefetchAllSchemas, as it walks the foreign keys of the
// cached schemas.
func (d *Dumper) computeExcludeFilters(ctx context.Context) error {
	if len(d.ExcludeMap) == 0 {
		return nil
	}

	schemas := make(map[string]*TableSchema, len(d.schemaCache))
	for name, schema := range d.schemaCache {
		schemas[strings.ToLower(name)] = schema
	}

	seeds := make(map[string]string, len(d.ExcludeMap))
	for table, condition := range d.ExcludeMap {
		table = strings.ToLower(table)

		if _, ok := schemas[table]; !ok {
			logging.FromContext(ctx).Warnf("Ignoring row exclusion for table %s: the table does not exist or is excluded from the dump", table)
			continue
		}

		seeds[table] = condition
	}

	if len(seeds) == 0 {
		return nil
	}

	resolver := &excludeResolver{
		schemas:   schemas,
		seeds:     seeds,
		affected:  make(map[string]bool),
		memo:      make(map[string]string),
		resolving: make(map[string]bool),
	}
	resolver.markAffectedTables()

	d.excludeWhere = make(map[string]string)
	for _, table := range slices.Sorted(maps.Keys(resolver.affected)) {
		if condition := resolver.keepConditionFor(table); condition != "" {
			d.excludeWhere[table] = condition
		}
	}

	if d.LockTables && len(d.excludeWhere) > 0 {
		logging.FromContext(ctx).Infof("Skipping table locks for %d tables filtered by row exclusions, as their dump queries reference other tables", len(d.excludeWhere))
	}

	return nil
}

type excludeResolver struct {
	schemas map[string]*TableSchema
	// seeds contains the tables with a configured exclude condition, mapping
	// the table name to the condition selecting the rows to exclude.
	seeds map[string]string
	// affected contains all tables whose rows must be filtered: the seeded
	// tables plus every table referencing an affected table via foreign keys.
	affected map[string]bool
	// memo caches the excluded-row condition per table.
	memo map[string]string
	// resolving guards against foreign key cycles: edges into a table that is
	// currently being resolved are skipped, which keeps a superset of the
	// strictly unreferenced rows instead of recursing forever.
	resolving map[string]bool
}

func (r *excludeResolver) markAffectedTables() {
	for table := range r.seeds {
		r.affected[table] = true
	}

	markReferencingTables(r.schemas, r.affected)
}

// keepConditionFor returns the WHERE condition restricting the given table to
// the rows that are not excluded: rows not matching the table's own exclude
// condition and not referencing an excluded row of an affected table.
func (r *excludeResolver) keepConditionFor(table string) string {
	var parts []string

	if condition, isSeed := r.seeds[table]; isSeed {
		parts = append(parts, "NOT ("+condition+")")
	}

	for _, fk := range r.schemas[table].ForeignKeys {
		referenced := strings.ToLower(fk.ReferencedTable)
		if !r.affected[referenced] || referenced == table || r.excludedConditionFor(referenced) == "" {
			continue
		}

		condition := fmt.Sprintf("%s NOT IN (%s)", columnTuple(fk.Columns), r.excludedRowsQuery(referenced, fk.ReferencedColumns))

		// Rows not referencing the parent at all are kept.
		if nullable := nullableColumns(r.schemas[table], fk.Columns); len(nullable) != 0 {
			nullChecks := make([]string, 0, len(nullable))
			for _, column := range nullable {
				nullChecks = append(nullChecks, fmt.Sprintf("`%s` IS NULL", column))
			}

			condition = fmt.Sprintf("%s OR %s", strings.Join(nullChecks, " OR "), condition)
		}

		parts = append(parts, "("+condition+")")
	}

	return strings.Join(parts, " AND ")
}

// excludedConditionFor returns the condition selecting the excluded rows of the
// given table: its seeded rows plus the rows referencing an excluded row of an
// affected table.
func (r *excludeResolver) excludedConditionFor(table string) string {
	if condition, ok := r.memo[table]; ok {
		return condition
	}

	r.resolving[table] = true
	defer delete(r.resolving, table)

	var parts []string

	if condition, isSeed := r.seeds[table]; isSeed {
		parts = append(parts, "("+condition+")")
	}

	for _, fk := range r.schemas[table].ForeignKeys {
		referenced := strings.ToLower(fk.ReferencedTable)
		if !r.affected[referenced] || referenced == table || r.resolving[referenced] || r.excludedConditionFor(referenced) == "" {
			continue
		}

		parts = append(parts, fmt.Sprintf("%s IN (%s)", columnTuple(fk.Columns), r.excludedRowsQuery(referenced, fk.ReferencedColumns)))
	}

	condition := strings.Join(parts, " OR ")
	r.memo[table] = condition
	return condition
}

// excludedRowsQuery returns a SELECT yielding the given columns of all
// excluded rows of the given affected table.
func (r *excludeResolver) excludedRowsQuery(table string, columns []string) string {
	return fmt.Sprintf("SELECT %s FROM `%s` WHERE %s", quoteColumns(columns), r.schemas[table].Name, r.excludedConditionFor(table))
}
