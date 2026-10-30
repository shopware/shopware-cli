package shop

// EnableSkipCustomerData excludes customer and order records, their dependent
// records, and their history, including transient data and promotion redemptions.
// Explicit names preserve configuration tables such as customer_group and document_type.
// Plugin tables can be added through dump.skip_customer_data_tables.
func (c *ConfigDump) EnableSkipCustomerData() {
	c.EnableClean()
	c.NoData = appendMissing(c.NoData, c.SkipCustomerDataTables)
	c.NoData = appendMissing(c.NoData, []string{
		"customer",
		"customer_address",
		"customer_tag",
		"customer_wishlist",
		"customer_wishlist_product",
		"document",
		"document_file",
		"newsletter_recipient",
		"newsletter_recipient_tag",
		"order",
		"order_address",
		"order_customer",
		"order_delivery",
		"order_delivery_position",
		"order_line_item",
		"order_line_item_download",
		"order_tag",
		"order_transaction",
		"order_transaction_capture",
		"order_transaction_capture_refund",
		"order_transaction_capture_refund_position",
		"product_review",
		"promotion_persona_customer",
		"sales_channel_api_context",
		"sales_channel_tracking_customer",
		"sales_channel_tracking_order",
		"state_machine_history",
	})

	c.forceRewrite("promotion", "orders_per_customer_count", "NULL")
	c.forceRewrite("promotion", "order_count", "0")
	c.forceRewrite("promotion_individual_code", "payload", "NULL")
	c.forceRewrite("mail_template_type", "template_data", "NULL")
}
