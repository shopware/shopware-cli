package mysqldump

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// validateOverlappingDeleteActions rejects affected FK actions that share columns
// and can produce different results depending on which parent is deleted first.
func (d *Dumper) validateOverlappingDeleteActions(schemas map[string]*TableSchema) error {
	for _, table := range slices.Sorted(maps.Keys(schemas)) {
		if len(d.excludeSetNull[table]) == 0 ||
			d.filterMap[table] == IgnoreMapPlacement || d.filterMap[table] == NoDataMapPlacement {
			continue
		}
		schema := schemas[table]
		for i, left := range schema.ForeignKeys {
			if d.excludeWhere[strings.ToLower(left.ReferencedTable)] == "" {
				continue
			}
			for _, right := range schema.ForeignKeys[i+1:] {
				if d.excludeWhere[strings.ToLower(right.ReferencedTable)] == "" ||
					!deleteActionsCanInterfere(left, right) {
					continue
				}
				return fmt.Errorf(
					"cannot dump table %s: overlapping foreign keys %s and %s require an explicit delete order",
					schema.Name, left.Name, right.Name,
				)
			}
		}
	}
	return nil
}

func deleteActionsCanInterfere(left, right ForeignKeySchema) bool {
	leftNull := strings.EqualFold(left.OnDelete, "SET NULL")
	rightNull := strings.EqualFold(right.OnDelete, "SET NULL")
	if !leftNull && !rightNull {
		return false
	}
	shared, identical := false, len(left.Columns) == len(right.Columns)
	for _, column := range left.Columns {
		found := slices.Contains(right.Columns, column)
		shared = shared || found
		identical = identical && found
	}
	// Two actions nulling exactly the same columns have the same result.
	if leftNull && rightNull && identical {
		return false
	}
	return shared
}
