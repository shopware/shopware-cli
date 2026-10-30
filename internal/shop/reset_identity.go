package shop

import (
	"slices"

	"github.com/shopware/shopware-cli/internal/mysqldump"
)

// resetIdentitySystemConfigWhere drops the shop ID from system_config. Shopware
// stores it under core.app.shopId (legacy) and core.app.shopIdV2 (current);
// removing both matches ShopIdProvider::deleteShopId, so the cloned shop
// generates a new shop ID on first use.
const resetIdentitySystemConfigWhere = "`configuration_key` NOT IN ('core.app.shopId', 'core.app.shopIdV2')"

// EnableResetIdentity removes the source shop ID and retained app registration secrets.
func (c *ConfigDump) EnableResetIdentity() {
	c.mergeWhere("system_config", resetIdentitySystemConfigWhere)
	if !slices.Contains(c.NoData, "deleted_apps") {
		c.NoData = append(c.NoData, "deleted_apps")
	}
}

// resetIdentityExclusions removes completed and pending app registrations tied to the
// source shop, together with their integrations so the clone cannot reuse their API credentials.
func resetIdentityExclusions(schemas map[string]*mysqldump.TableSchema) map[string]string {
	registeredApp := "`app_secret` IS NOT NULL"
	if app := schemas["app"]; app != nil && slices.ContainsFunc(app.Columns, func(column mysqldump.ColumnSchema) bool {
		return column.Name == "unconfirmed_app_secrets"
	}) {
		registeredApp += " OR JSON_LENGTH(`unconfirmed_app_secrets`) > 0"
	}
	return map[string]string{
		"app":         registeredApp,
		"integration": "`id` IN (SELECT `integration_id` FROM `app` WHERE " + registeredApp + ")",
	}
}
