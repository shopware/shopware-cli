---
title: Project Commands
weight: 30
---

## shopware-cli project create [folder] [version]

Create a new Shopware 6 project from the chosen version

Arguments:

* `folder` - **Required**: Folder name for the installation
* `version` - Version to install

## shopware-cli project admin-build

Builds the Administration with all installed extensions

Parameters:

* `--skip-assets-install` - Skips the assets installation
* `--force-install-dependencies` - Forces the installation of NPM dependencies

## shopware-cli project admin-watch

Starts the Administration watcher

Parameters:

* `--only-extensions` - Only consider given list of extensions for the watcher (comma-separated list)
* `--skip-extensions` - Skip given list of extensions for the watcher (comma-separated list)

## shopware-cli project storefront-build

Builds the Storefront with all installed extensions

Parameters:

* `--skip-theme-compile` - Skips the theme compilation
* `--force-install-dependencies` - Forces the installation of NPM dependencies

## shopware-cli project storefront-watch

Starts the Storefront watcher

Parameters:

* `--only-extensions` - Only consider given list of extensions for the watcher (comma-separated list)
* `--skip-extensions` - Skip given list of extensions for the watcher (comma-separated list)

## shopware-cli project worker

Starts the Shopware worker in the background and tails the log

Parameters:

* `--queue`: Queue names to start. F.e: `--queue "default,high,low"`
* `--time-limit`: Limit the execution time of each worker in seconds
* `--memory-limit`: Limit the max memory usage of each worker before restart

Arguments:

* Worker amount - `shopware-cli project worker 5` starts 5 workers

## shopware-cli project dump [database]

Dumps the MySQL database as SQL. Additional configuration can be done with a `.shopware-project.yml` like

```yaml
dump:
  # Rewrite column content new value
  rewrite:
    table:
      column: "'new-value'"
      column2: "faker.Internet().Email()" # Uses faker data. See https://github.com/jaswdr/faker
  # Ignore table content
  nodata:
    - table
  # Ignore entire table
  ignore:
    - table
  # Add a where to the export
  where:
    table: 'id > 5'
```

Parameters:

* `--host` - MySQL Host (default: 127.0.0.1)
* `--port` - MySQL Port (default: 3306)
* `--username` - MySQL Username (default: root)
* `--password` - MySQL Password (default: root)
* `--output` - Output file (default: `dump.sql`)
* `--clean` - Ignores content of following tables: `cart`, `customer_recovery`, `dead_message`, `enqueue`, `increment`, `elasticsearch_index_task`, `log_entry`, `message_queue_stats`, `notification`, `payment_token`, `refresh_token`, `version`, `version_commit`, `version_commit_data`, `webhook_event_log`
* `--skip-lock-tables` - Skips locking of tables
* `--anonymize` - Additionally to the configurated `dump.rewrite`, this parameter will anonymize known user data tables. [See](https://github.com/shopware/shopware-cli/blob/main/cmd/project/project_dump.go#L73) for the list
* `--gzip` - Create a gzip compressed file
* `--zstd` - Create a zstd compressed file

Examples:

- `shopware-cli project dump sw6 --host 127.0.0.1 --username root --password root --clean --anonymize`

## shopware-cli project admin-api [method] [path]

Run authentificated curl against the admin api

Arguments:

* `method` - **Required:** HTTP method
* `path` - **Required:** HTTP path

Parameters:

* `--output-token` - Outputs only the access token

Examples:

- `shopware-cli project admin-api POST "/search/tax" -- -d '{"limit": 1}' -H 'Accept: application/json' -H 'Content-Type: application/json'`


## shopware-cli project clear-cache

Clears the cache of the shop

## shopware-cli project extension list

Lists all extensions of the shop

Parameters:

* `--json` - Outputs as JSON

## shopware-cli project extension outdated

Shows only extensions which are updateable. Exists with exit code 1 when updates are found

Parameters:

* `--json` - Outputs as JSON


## shopware-cli project extension install

Install one or more extensions

Arguments:

- The extension name


## shopware-cli project extension uninstall

Uninstall one or more extensions

Arguments:

- The extension name


## shopware-cli project extension activate

Activate one or more extensions

Arguments:

- The extension name


## shopware-cli project extension deactivate

Deactivates one or more extensions

Arguments:

- The extension name


## shopware-cli project extension update

Updates one or more extensions

Use `all` as argument to update all possible extensions

Arguments:

- The extension name


## shopware-cli project extension upload [folder|zip]

Uploads one local extension zip or folder to shop

Arguments:

- zip or folder path

Parameters:

- `--activate` - Installs, Activates or updates the extension after upload

## shopware-cli project config pull

Downloads the current external shop config to the local `.shopware-project.yml`. Use `shopware-cli project config init` to create the basic config file first

## shopware-cli project config push

Pushes the local configuration to the external system

Parameters:

* `--auto-approve` - Skips the manual confirmation

## shopware-cli project ci

Builds a Shopware project with assets, composer etc

Arguments:

- project path

Flags:

- `--with-dev-dependencies` - Install dev dependencies

You can set `SHOPWARE_PACKAGES_TOKEN` as environment variable with the Shopware Composer Registry token,
to pass it to the composer command.

What that command does:

- Installs all composer dependencies
- Builds all storefront and admin assets of all extensions
- Strips unused files from the vendor folder

The steps can be configured using a `.shopware-project.yaml` see [Schema](../shopware-project-yml-schema.md) for more information.

## shopware-cli project generate-jwt

Generates a JWT token for the given path

Arguments:

- project path (optional)

Parameters:

* `--env` - Print the JWT key as environment variable
