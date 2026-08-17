package mysqldump

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func notNullColumns(names ...string) []ColumnSchema {
	cols := make([]ColumnSchema, 0, len(names))
	for _, name := range names {
		cols = append(cols, ColumnSchema{Name: name})
	}
	return cols
}

func limitTestDumper(schemas ...*TableSchema) *Dumper {
	dumper := NewMySQLDumper(nil)
	dumper.schemaCache = make(map[string]*TableSchema, len(schemas))
	for _, schema := range schemas {
		dumper.schemaCache[schema.Name] = schema
	}
	return dumper
}

func orderGraphDumper() *Dumper {
	return limitTestDumper(
		&TableSchema{
			Name:       "order",
			PrimaryKey: []string{"id", "version_id"},
			Columns:    notNullColumns("id", "version_id", "created_at"),
		},
		&TableSchema{
			Name:       "order_line_item",
			PrimaryKey: []string{"id", "version_id"},
			Columns: []ColumnSchema{
				{Name: "id"}, {Name: "version_id"}, {Name: "order_id"}, {Name: "order_version_id"},
				{Name: "parent_id", Nullable: true}, {Name: "product_id", Nullable: true},
			},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.order_line_item.order_id", Columns: []string{"order_id", "order_version_id"}, ReferencedTable: "order", ReferencedColumns: []string{"id", "version_id"}},
				{Name: "fk.order_line_item.parent_id", Columns: []string{"parent_id"}, ReferencedTable: "order_line_item", ReferencedColumns: []string{"id"}},
				{Name: "fk.order_line_item.product_id", Columns: []string{"product_id"}, ReferencedTable: "product", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "order_delivery",
			PrimaryKey: []string{"id", "version_id"},
			Columns:    notNullColumns("id", "version_id", "order_id", "order_version_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.order_delivery.order_id", Columns: []string{"order_id", "order_version_id"}, ReferencedTable: "order", ReferencedColumns: []string{"id", "version_id"}},
			},
		},
		&TableSchema{
			Name:       "order_delivery_position",
			PrimaryKey: []string{"id", "version_id"},
			Columns:    notNullColumns("id", "version_id", "order_delivery_id", "order_delivery_version_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.order_delivery_position.order_delivery_id", Columns: []string{"order_delivery_id", "order_delivery_version_id"}, ReferencedTable: "order_delivery", ReferencedColumns: []string{"id", "version_id"}},
			},
		},
		&TableSchema{
			Name:       "product",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "created_at"),
		},
	)
}

func TestLimitCascadesOverForeignKeys(t *testing.T) {
	dumper := orderGraphDumper()
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 100}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptOrders := "SELECT `id`, `version_id` FROM (SELECT `id`, `version_id` FROM `order` ORDER BY `created_at` DESC LIMIT 100) `_sw_cli_limit`"

	assert.Equal(t, "(`id`, `version_id`) IN ("+keptOrders+")", dumper.limitWhere["order"])
	assert.Equal(t, "((`order_id`, `order_version_id`) IN ("+keptOrders+"))", dumper.limitWhere["order_line_item"])
	assert.Equal(t, "((`order_id`, `order_version_id`) IN ("+keptOrders+"))", dumper.limitWhere["order_delivery"])
	assert.Equal(t,
		"((`order_delivery_id`, `order_delivery_version_id`) IN (SELECT `id`, `version_id` FROM `order_delivery` WHERE ((`order_id`, `order_version_id`) IN ("+keptOrders+"))))",
		dumper.limitWhere["order_delivery_position"])

	// product is only referenced by order_line_item, not referencing it, so it stays complete
	assert.NotContains(t, dumper.limitWhere, "product")
}

func TestLimitKeepsRowsWithNullableForeignKey(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "order",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "created_at"),
		},
		&TableSchema{
			Name:       "document",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "order_id", Nullable: true}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.document.order_id", Columns: []string{"order_id"}, ReferencedTable: "order", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 10}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptOrders := "SELECT `id` FROM (SELECT `id` FROM `order` ORDER BY `created_at` DESC LIMIT 10) `_sw_cli_limit`"
	assert.Equal(t, "(`order_id` IS NULL OR `order_id` IN ("+keptOrders+"))", dumper.limitWhere["document"])
}

func TestLimitRespectsWhereMapAndCustomOrderBy(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "order",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "order_number"),
		},
		&TableSchema{
			Name:       "order_tag",
			PrimaryKey: []string{"order_id", "tag_id"},
			Columns:    notNullColumns("order_id", "tag_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.order_tag.order_id", Columns: []string{"order_id"}, ReferencedTable: "order", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.WhereMap = map[string]string{"order": "version_id = 0x1"}
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 5, OrderBy: "`order_number` DESC"}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptOrders := "SELECT `id` FROM (SELECT `id` FROM `order` WHERE version_id = 0x1 ORDER BY `order_number` DESC LIMIT 5) `_sw_cli_limit`"
	assert.Equal(t, "`id` IN ("+keptOrders+")", dumper.limitWhere["order"])
	assert.Equal(t, "(`order_id` IN ("+keptOrders+"))", dumper.limitWhere["order_tag"])
}

func TestLimitWithoutCreatedAtHasNoOrderBy(t *testing.T) {
	dumper := limitTestDumper(&TableSchema{
		Name:       "queue",
		PrimaryKey: []string{"id"},
		Columns:    notNullColumns("id"),
	})
	dumper.LimitMap = map[string]TableLimit{"queue": {Rows: 3}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	assert.Equal(t, "`id` IN (SELECT `id` FROM (SELECT `id` FROM `queue` LIMIT 3) `_sw_cli_limit`)", dumper.limitWhere["queue"])
}

func TestLimitIncludesChildWhereInCascade(t *testing.T) {
	dumper := orderGraphDumper()
	dumper.WhereMap = map[string]string{"order_delivery": "custom_fields IS NULL"}
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 100}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptOrders := "SELECT `id`, `version_id` FROM (SELECT `id`, `version_id` FROM `order` ORDER BY `created_at` DESC LIMIT 100) `_sw_cli_limit`"
	assert.Equal(t,
		"((`order_delivery_id`, `order_delivery_version_id`) IN (SELECT `id`, `version_id` FROM `order_delivery` WHERE (custom_fields IS NULL) AND ((`order_id`, `order_version_id`) IN ("+keptOrders+"))))",
		dumper.limitWhere["order_delivery_position"])
}

func selfReferencingProductDumper() *Dumper {
	return limitTestDumper(
		&TableSchema{
			Name:       "product",
			PrimaryKey: []string{"id", "version_id"},
			Columns: []ColumnSchema{
				{Name: "id"}, {Name: "version_id"},
				{Name: "parent_id", Nullable: true}, {Name: "parent_version_id", Nullable: true},
				{Name: "created_at"},
			},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.product.parent", Columns: []string{"parent_id", "parent_version_id"}, ReferencedTable: "product", ReferencedColumns: []string{"id", "version_id"}},
			},
		},
		&TableSchema{
			Name:       "order_line_item",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "product_id", Nullable: true}, {Name: "product_version_id", Nullable: true}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.oli.product", Columns: []string{"product_id", "product_version_id"}, ReferencedTable: "product", ReferencedColumns: []string{"id", "version_id"}},
			},
		},
	)
}

func TestLimitClosesSelfReferencingTableOverParents(t *testing.T) {
	dumper := selfReferencingProductDumper()
	dumper.LimitMap = map[string]TableLimit{"product": {Rows: 500}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptProducts := "WITH RECURSIVE `_sw_cli_kept` (`id`, `version_id`) AS (" +
		"SELECT `id`, `version_id` FROM (SELECT `id`, `version_id` FROM `product` ORDER BY `created_at` DESC LIMIT 500) `_sw_cli_limit`" +
		" UNION " +
		"SELECT `_sw_parent`.`id`, `_sw_parent`.`version_id` FROM `product` `_sw_child`" +
		" JOIN `_sw_cli_kept` ON `_sw_child`.`id` = `_sw_cli_kept`.`id` AND `_sw_child`.`version_id` = `_sw_cli_kept`.`version_id`" +
		" JOIN `product` `_sw_parent` ON `_sw_parent`.`id` = `_sw_child`.`parent_id` AND `_sw_parent`.`version_id` = `_sw_child`.`parent_version_id`" +
		") SELECT `id`, `version_id` FROM `_sw_cli_kept`"

	// The limited product table keeps the LIMIT rows plus all their ancestors.
	assert.Equal(t, "(`id`, `version_id`) IN ("+keptProducts+")", dumper.limitWhere["product"])

	// A table referencing product is filtered against the same closed set, so a
	// kept line item always finds its product and that product finds its parent.
	assert.Equal(t,
		"(`product_id` IS NULL OR `product_version_id` IS NULL OR (`product_id`, `product_version_id`) IN ("+keptProducts+"))",
		dumper.limitWhere["order_line_item"])
}

func TestLimitClosesSelfReferencingTableWithSingleColumnKey(t *testing.T) {
	dumper := limitTestDumper(&TableSchema{
		Name:       "category",
		PrimaryKey: []string{"id"},
		Columns: []ColumnSchema{
			{Name: "id"}, {Name: "parent_id", Nullable: true}, {Name: "created_at"},
		},
		ForeignKeys: []ForeignKeySchema{
			{Name: "fk.category.parent", Columns: []string{"parent_id"}, ReferencedTable: "category", ReferencedColumns: []string{"id"}},
		},
	})
	dumper.LimitMap = map[string]TableLimit{"category": {Rows: 20}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptCategories := "WITH RECURSIVE `_sw_cli_kept` (`id`) AS (" +
		"SELECT `id` FROM (SELECT `id` FROM `category` ORDER BY `created_at` DESC LIMIT 20) `_sw_cli_limit`" +
		" UNION " +
		"SELECT `_sw_parent`.`id` FROM `category` `_sw_child`" +
		" JOIN `_sw_cli_kept` ON `_sw_child`.`id` = `_sw_cli_kept`.`id`" +
		" JOIN `category` `_sw_parent` ON `_sw_parent`.`id` = `_sw_child`.`parent_id`" +
		") SELECT `id` FROM `_sw_cli_kept`"

	assert.Equal(t, "`id` IN ("+keptCategories+")", dumper.limitWhere["category"])
}

func TestLimitClosureRespectsWhereMap(t *testing.T) {
	dumper := selfReferencingProductDumper()
	dumper.WhereMap = map[string]string{"product": "active = 1"}
	dumper.LimitMap = map[string]TableLimit{"product": {Rows: 10}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	// The user WHERE only constrains the LIMIT seed; ancestors are still pulled
	// in unconditionally so the kept set stays closed under the self-reference.
	assert.Contains(t, dumper.limitWhere["product"],
		"SELECT `id`, `version_id` FROM `product` WHERE active = 1 ORDER BY `created_at` DESC LIMIT 10")
}

func TestLimitTerminatesOnForeignKeyCycles(t *testing.T) {
	dumper := limitTestDumper(
		&TableSchema{
			Name:       "s",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id"),
		},
		&TableSchema{
			Name:       "a",
			PrimaryKey: []string{"id"},
			Columns:    notNullColumns("id", "b_id"),
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.a.b_id", Columns: []string{"b_id"}, ReferencedTable: "b", ReferencedColumns: []string{"id"}},
			},
		},
		&TableSchema{
			Name:       "b",
			PrimaryKey: []string{"id"},
			Columns:    []ColumnSchema{{Name: "id"}, {Name: "a_id", Nullable: true}, {Name: "s_id"}},
			ForeignKeys: []ForeignKeySchema{
				{Name: "fk.b.a_id", Columns: []string{"a_id"}, ReferencedTable: "a", ReferencedColumns: []string{"id"}},
				{Name: "fk.b.s_id", Columns: []string{"s_id"}, ReferencedTable: "s", ReferencedColumns: []string{"id"}},
			},
		},
	)
	dumper.LimitMap = map[string]TableLimit{"s": {Rows: 5}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))

	keptS := "SELECT `id` FROM (SELECT `id` FROM `s` LIMIT 5) `_sw_cli_limit`"
	assert.Contains(t, dumper.limitWhere["b"], "(`s_id` IN ("+keptS+"))")
	assert.Contains(t, dumper.limitWhere["a"], "`b_id` IN (SELECT `id` FROM `b` WHERE")
}

func TestLimitUnknownTableIsIgnored(t *testing.T) {
	dumper := limitTestDumper(&TableSchema{
		Name:       "order",
		PrimaryKey: []string{"id"},
		Columns:    notNullColumns("id"),
	})
	dumper.LimitMap = map[string]TableLimit{"missing": {Rows: 5}}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))
	assert.Empty(t, dumper.limitWhere)
}

func TestLimitRejectsOverlappingTables(t *testing.T) {
	t.Run("child of a limited table", func(t *testing.T) {
		dumper := orderGraphDumper()
		dumper.LimitMap = map[string]TableLimit{
			"order":           {Rows: 100},
			"order_line_item": {Rows: 50},
		}

		err := dumper.computeLimitFilters(t.Context())
		assert.ErrorContains(t, err, "cannot limit table order_line_item")
		assert.ErrorContains(t, err, "limiting table order already filters order_line_item")
	})

	t.Run("transitive child of a limited table", func(t *testing.T) {
		dumper := orderGraphDumper()
		dumper.LimitMap = map[string]TableLimit{
			"order":                   {Rows: 100},
			"order_delivery_position": {Rows: 10},
		}

		err := dumper.computeLimitFilters(t.Context())
		assert.ErrorContains(t, err, "cannot limit table order_delivery_position")
		assert.ErrorContains(t, err, "limiting table order already filters order_delivery_position")
	})

	t.Run("parent and child limited independently", func(t *testing.T) {
		dumper := limitTestDumper(
			&TableSchema{
				Name:       "customer",
				PrimaryKey: []string{"id"},
				Columns:    notNullColumns("id", "created_at"),
			},
			&TableSchema{
				Name:       "order",
				PrimaryKey: []string{"id"},
				Columns:    notNullColumns("id", "customer_id", "created_at"),
				ForeignKeys: []ForeignKeySchema{
					{Name: "fk.order.customer_id", Columns: []string{"customer_id"}, ReferencedTable: "customer", ReferencedColumns: []string{"id"}},
				},
			},
		)
		dumper.LimitMap = map[string]TableLimit{
			"order":    {Rows: 100},
			"customer": {Rows: 50},
		}

		err := dumper.computeLimitFilters(t.Context())
		assert.ErrorContains(t, err, "cannot limit table order")
		assert.ErrorContains(t, err, "limiting table customer already filters order")
	})

	t.Run("foreign key cycle", func(t *testing.T) {
		dumper := limitTestDumper(
			&TableSchema{
				Name:       "a",
				PrimaryKey: []string{"id"},
				Columns:    notNullColumns("id", "b_id"),
				ForeignKeys: []ForeignKeySchema{
					{Name: "fk.a.b_id", Columns: []string{"b_id"}, ReferencedTable: "b", ReferencedColumns: []string{"id"}},
				},
			},
			&TableSchema{
				Name:       "b",
				PrimaryKey: []string{"id"},
				Columns:    notNullColumns("id", "a_id"),
				ForeignKeys: []ForeignKeySchema{
					{Name: "fk.b.a_id", Columns: []string{"a_id"}, ReferencedTable: "a", ReferencedColumns: []string{"id"}},
				},
			},
		)
		dumper.LimitMap = map[string]TableLimit{
			"a": {Rows: 10},
			"b": {Rows: 10},
		}

		err := dumper.computeLimitFilters(t.Context())
		assert.ErrorContains(t, err, "cannot limit table b")
		assert.ErrorContains(t, err, "limiting table a already filters b")
	})
}

func TestLimitAllowsUnrelatedTables(t *testing.T) {
	dumper := orderGraphDumper()
	dumper.LimitMap = map[string]TableLimit{
		"order":   {Rows: 100},
		"product": {Rows: 50},
	}

	assert.NoError(t, dumper.computeLimitFilters(t.Context()))
	assert.Contains(t, dumper.limitWhere, "order")
	assert.Contains(t, dumper.limitWhere, "product")
}

func TestLimitValidation(t *testing.T) {
	dumper := limitTestDumper(&TableSchema{
		Name:       "order",
		PrimaryKey: []string{"id"},
		Columns:    notNullColumns("id"),
	})
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 0}}
	assert.ErrorContains(t, dumper.computeLimitFilters(t.Context()), "must be greater than zero")

	dumper = limitTestDumper(&TableSchema{
		Name:    "order",
		Columns: notNullColumns("id"),
	})
	dumper.LimitMap = map[string]TableLimit{"order": {Rows: 5}}
	assert.ErrorContains(t, dumper.computeLimitFilters(t.Context()), "no primary key")
}

func TestEffectiveWhereCombinesUserAndLimitConditions(t *testing.T) {
	dumper := NewMySQLDumper(nil)
	dumper.WhereMap = map[string]string{"t": "a = 1"}
	dumper.limitWhere = map[string]string{"t": "`b` IN (SELECT 1)", "u": "`c` IN (SELECT 2)"}

	assert.Equal(t, "(a = 1) AND (`b` IN (SELECT 1))", dumper.effectiveWhere("T"))
	assert.Equal(t, "`c` IN (SELECT 2)", dumper.effectiveWhere("u"))
	assert.Equal(t, "", dumper.effectiveWhere("other"))
}
