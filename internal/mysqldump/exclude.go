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
// in excludeWhere. Directly configured exclusions keep only rows whose condition is not true.
// CASCADE edges filter referencing rows, SET NULL edges rewrite their foreign
// key values, and restrictive edges are checked for dangling exported rows.
// Must run after prefetchAllSchemas because it walks the cached foreign keys.
func (d *Dumper) computeExcludeFilters(ctx context.Context) error {
	if len(d.ExcludeMap) == 0 {
		return nil
	}

	resolver := d.newExcludeResolver(ctx)
	if resolver == nil {
		return nil
	}
	resolver.markAffectedTables()

	d.excludeWhere = resolver.keepConditions()
	d.computeExcludeSetNull(resolver.schemas)
	if err := d.validateExcludedReferences(ctx, resolver.schemas); err != nil {
		return err
	}

	if d.LockTables && len(d.excludeWhere) > 0 {
		logging.FromContext(ctx).Infof(
			"Skipping table locks for %d tables filtered by row exclusions, as their dump queries reference other tables",
			len(d.excludeWhere),
		)
	}

	return nil
}

// newExcludeResolver normalizes table names without changing caller-owned rules.
// Exclusions for missing tables are ignored, as schemas differ across shop versions.
func (d *Dumper) newExcludeResolver(ctx context.Context) *excludeResolver {
	schemas := make(map[string]*TableSchema, len(d.schemaCache))
	for name, schema := range d.schemaCache {
		schemas[strings.ToLower(name)] = schema
	}

	directExclusions := make(map[string]string, len(d.ExcludeMap))
	for table, condition := range d.ExcludeMap {
		table = strings.ToLower(table)
		if _, ok := schemas[table]; !ok {
			logging.FromContext(ctx).Warnf("Ignoring row exclusion for table %s: the table does not exist", table)
			continue
		}
		directExclusions[table] = condition
	}
	if len(directExclusions) == 0 {
		return nil
	}

	return &excludeResolver{
		schemas:          schemas,
		directExclusions: directExclusions,
		affected:         make(map[string]bool),
		memo:             make(map[string]string),
		resolving:        make(map[string]bool),
	}
}

// computeExcludeSetNull uses the final parent filters, including user conditions
// and row limits, to disconnect references to rows omitted from the dump.
func (d *Dumper) computeExcludeSetNull(schemas map[string]*TableSchema) {
	d.excludeSetNull = make(map[string]map[string][]string)
	for _, table := range slices.Sorted(maps.Keys(schemas)) {
		for _, fk := range schemas[table].ForeignKeys {
			if !strings.EqualFold(fk.OnDelete, "SET NULL") {
				continue
			}
			condition := d.excludedReferenceCondition(schemas, fk)
			if condition == "" {
				continue
			}
			if d.excludeSetNull[table] == nil {
				d.excludeSetNull[table] = make(map[string][]string)
			}
			for _, column := range fk.Columns {
				column = strings.ToLower(column)
				d.excludeSetNull[table][column] = append(d.excludeSetNull[table][column], condition)
			}
		}
	}
}

// excludedReferenceCondition matches a parent affected by row exclusions.
// Its final filters determine which parent rows are omitted.
func (d *Dumper) excludedReferenceCondition(schemas map[string]*TableSchema, fk ForeignKeySchema) string {
	referenced := strings.ToLower(fk.ReferencedTable)
	if d.excludeWhere[referenced] == "" {
		return ""
	}
	return fmt.Sprintf(
		"%s IN (%s)",
		columnTuple(fk.Columns),
		d.excludedRowsQueryForDump(schemas[referenced], fk.ReferencedColumns),
	)
}

// excludedRowsQueryForDump returns referenced keys for rows omitted by the
// final table filter, including exclusions that cross a foreign key cycle.
func (d *Dumper) excludedRowsQueryForDump(schema *TableSchema, columns []string) string {
	query := fmt.Sprintf("SELECT %s FROM `%s`", quoteColumns(columns), schema.Name)
	if d.filterMap[strings.ToLower(schema.Name)] == NoDataMapPlacement {
		return query
	}
	return fmt.Sprintf("%s WHERE (%s) IS NOT TRUE", query, d.effectiveWhere(schema.Name))
}

type excludeResolver struct {
	schemas map[string]*TableSchema
	// directExclusions maps tables to their configured conditions selecting rows to omit.
	directExclusions map[string]string
	// affected includes tables with direct exclusions and those reached through foreign keys.
	affected map[string]bool
	// memo caches the excluded-row condition per table.
	memo map[string]string
	// resolving guards against foreign key cycles: edges into a table that is
	// currently being resolved are skipped during SQL construction. Validation
	// rejects any resulting retained row that would reference an omitted parent.
	resolving map[string]bool
}

func (r *excludeResolver) markAffectedTables() {
	for table := range r.directExclusions {
		r.affected[table] = true
	}

	markReferencingTables(r.schemas, r.affected)
}

// keepConditions builds filters in a stable order so cycle resolution is deterministic.
func (r *excludeResolver) keepConditions() map[string]string {
	conditions := make(map[string]string)
	for _, table := range slices.Sorted(maps.Keys(r.affected)) {
		if condition := r.keepConditionFor(table); condition != "" {
			conditions[table] = condition
		}
	}
	return conditions
}

// keepConditionFor retains rows that neither match a direct exclusion nor
// reference excluded parents through CASCADE edges.
func (r *excludeResolver) keepConditionFor(table string) string {
	var parts []string

	if condition, hasDirectExclusion := r.directExclusions[table]; hasDirectExclusion {
		parts = append(parts, "("+condition+") IS NOT TRUE")
	}

	for _, condition := range r.cascadeExclusions(table) {
		parts = append(parts, "(("+condition+") IS NOT TRUE)")
	}

	return strings.Join(parts, " AND ")
}

// excludedConditionFor returns the condition selecting the excluded rows of the
// given table: rows matching a direct exclusion or referencing an excluded
// parent through a CASCADE foreign key.
func (r *excludeResolver) excludedConditionFor(table string) string {
	if condition, ok := r.memo[table]; ok {
		return condition
	}

	r.resolving[table] = true
	defer delete(r.resolving, table)

	var parts []string

	if condition, hasDirectExclusion := r.directExclusions[table]; hasDirectExclusion {
		parts = append(parts, "("+condition+")")
	}

	parts = append(parts, r.cascadeExclusions(table)...)

	condition := strings.Join(parts, " OR ")
	r.memo[table] = condition
	return condition
}

// cascadeExclusions follows only CASCADE relationships. Recursive calls skip
// edges already being resolved; the final keep condition still checks those edges.
func (r *excludeResolver) cascadeExclusions(table string) []string {
	var parts []string
	for _, fk := range r.schemas[table].ForeignKeys {
		referenced := strings.ToLower(fk.ReferencedTable)
		if !r.affected[referenced] ||
			!strings.EqualFold(fk.OnDelete, "CASCADE") ||
			r.resolving[referenced] {
			continue
		}

		condition := r.excludedConditionFor(referenced)
		if condition == "" {
			continue
		}

		query := fmt.Sprintf(
			"SELECT %s FROM `%s` WHERE %s",
			quoteColumns(fk.ReferencedColumns),
			r.schemas[referenced].Name,
			condition,
		)
		parts = append(parts, fmt.Sprintf(
			"%s IN (%s)",
			columnTuple(fk.Columns),
			query,
		))
	}

	return parts
}
