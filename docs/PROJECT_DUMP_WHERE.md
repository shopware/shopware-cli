# Combining dump WHERE conditions across includes

`dump.where` sets one WHERE condition per table. When the same table is set in
a file and in one of its `include` files, the included value replaces the
other (unchanged, kept for compatibility).

To add to a condition instead, tag the value with `!append`. It is combined
with `AND` onto the condition of the same table, each side wrapped in
parentheses, so `OR` expressions stay correct.

```yaml
# .shopware-project.yml
include:
  - base.yml
dump:
  where:
    system_config: !append "configuration_key NOT LIKE 'my.plugin.%'"
```

```yaml
# base.yml
dump:
  where:
    system_config: "configuration_key LIKE 'core.%' OR configuration_key LIKE 'my.%'"
```

Resulting condition:

```sql
(configuration_key LIKE 'core.%' OR configuration_key LIKE 'my.%') AND (configuration_key NOT LIKE 'my.plugin.%')
```

Rules:

- Conditions of several includes are added in include order, nested includes
  first, the including file last: `(((a) AND (b)) AND (c)) AND (own)`.
- In `.shopware-project.local.yml`, `!append` is ANDed onto the value of the
  project file.
- Without a `where` for the table, the appended condition is used as is.
- Table names are matched ignoring case. Other tables are not changed.
