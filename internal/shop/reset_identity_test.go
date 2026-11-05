package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/mysqldump"
)

func TestResetIdentityExclusionsUseAppSchema(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "legacy"
		if pending {
			name = "pending registrations supported"
		}
		t.Run(name, func(t *testing.T) {
			app := &mysqldump.TableSchema{Name: "app", Columns: []mysqldump.ColumnSchema{{Name: "app_secret"}}}
			condition := "`app_secret` IS NOT NULL"
			if pending {
				app.Columns = append(app.Columns, mysqldump.ColumnSchema{Name: "unconfirmed_app_secrets"})
				condition += " OR JSON_LENGTH(`unconfirmed_app_secrets`) > 0"
			}
			assert.Equal(t, map[string]string{
				"app":         condition,
				"integration": "`id` IN (SELECT `integration_id` FROM `app` WHERE " + condition + ")",
			}, resetIdentityExclusions(map[string]*mysqldump.TableSchema{"app": app}))
		})
	}
}
