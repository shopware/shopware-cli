package shop

// EnableSkipCustomerData excludes customer and order records, their dependent
// records, and their history. Use explicit names so
// configuration tables such as customer_group and document_type keep their data.
// Plugin tables can be added through dump.skip_customer_data_tables.
func (c *ConfigDump) EnableSkipCustomerData() {
	c.NoData = appendMissing(c.NoData, c.SkipCustomerDataTables)
	c.NoData = appendMissing(c.NoData, []string{
		"cart",
		"customer",
		"customer_address",
		"customer_recovery",
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
		"version_commit_data",
		"webhook_event_log",
	})
}
