---
title: Generating MySQL dumps
weight: 11
---

shopware-cli has built-in support for generating MySQL dumps. The dump command is native implementation and does not use existing tools like `mysqldump`.

Creating a MySQL dump is as simple as running the following command:

```bash
shopware-cli project dump sw6 --host 127.0.0.1 --username root --password root
```

It's possible to use `--skip-lock-tables` to skip the lock tables command. This is useful for large databases.

## Anonymizing data

The `--anonymize` flag will anonymize known user data tables. The following tables are anonymized:

[See here for the complete list](https://github.com/shopware/shopware-cli/blob/main/cmd/project/project_dump.go#L74)

It's possible to customize the anonymization process by using the `dump.rewrite` configuration in the `shopware-cli.yml` file.

```yaml
# .shopware-project.yml
dump:
  rewrite:
    <table-name>:
      # Rewrite column content new value
      <column-name>: "'new-value'"
      # Use go-faker to generate data
      <column-name>: "faker.Internet().Email()" # See https://github.com/jaswdr/faker
```

## Ignoreing table content

Some tables are not relevant for dumps, like log tables. To ignore some default tables, use the `--clean` flag. This will ignore the content of the following tables:

- `cart`
- `customer_recovery`
- `dead_message`
- `enqueue`
- `increment`
- `elasticsearch_index_task`
- `log_entry`
- `message_queue_stats`
- `notification`
- `payment_token`
- `refresh_token`
- `version`
- `version_commit`
- `version_commit_data`
- `webhook_event_log`

To ignore additional tables, use the `dump.ignore` configuration in the `shopware-cli.yml` file.

```yaml
# .shopware-project.yml
dump:
  nodata:
    - <table-name>
```

## Ignoring entire tables

It's also possible to completely ignore a table **not only the content**.

```yaml
# .shopware-project.yml
dump:
  ignore:
    - <table-name>
```

## Adding a where clause

It's possible to add a where clause to the export of a table. So only rows matching the where clause will be exported.

```yaml
# .shopware-project.yml
dump:
  where:
    <table-name>: 'id > 5'
```

