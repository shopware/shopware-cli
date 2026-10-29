# Deployment backends

The `deployment` package defines backend capabilities and initialization inputs.
`backend` resolves project configuration and selects an implementation; `ssh`
owns SSH transport, release layout, initialization and multi-host coordination.
Deployment commands are experimental and require the `deployment` build tag.

## Initialize an SSH target

Run `shopware-cli project deploy init -e production` for guided setup. For CI or
wrapper scripts, supply a JSON file with `--init-config` (no prompts or additional
confirmation). Use `-` to read stdin, keeping credentials out of process arguments:

```sh
shopware-cli -n project deploy init -e production --init-config - < init.json
```

Example `init.json`:

```json
{
  "runtime_values": {
    "APP_URL": "https://shop.example.com",
    "DATABASE_URL": "mysql://shopware:encoded-password@db:3306/shopware"
  },
  "install_values": {
    "INSTALL_ADMIN_PASSWORD": "replace-with-a-strong-password",
    "INSTALL_ADMIN_EMAIL": "admin@example.com"
  }
}
```

Protect this file (for example, `chmod 600 init.json`), do not commit it, and remove
it when no longer needed. URL-encode database credentials in `DATABASE_URL`.

- `APP_URL` and `DATABASE_URL` are required. `APP_ENV` defaults to `prod` and
  `APP_DEBUG` to `0`. An existing `APP_SECRET` is preserved; otherwise one is
  generated unless explicitly supplied.
- Omit `install_values` to skip first-install setup. When provided, a non-empty
  map requires `INSTALL_ADMIN_PASSWORD`. `INSTALL_LOCALE`, `INSTALL_CURRENCY` and
  `INSTALL_ADMIN_USERNAME` default to `en-GB`, `EUR` and `admin`.
  `INSTALL_ADMIN_EMAIL` and `SALES_CHANNEL_URL` are optional; an omitted sales
  channel URL uses the application URL through Deployment Helper.
- Invalid or unknown values fail before remote inspection or writes. Supplying
  the configuration explicitly authorizes writing it; there is no final prompt.
- Repeated initialization preserves completed configuration. Named-host groups
  write only on the migration host and verify matching shared runtime files on
  every host, just like interactive setup.

Secrets are sent over SSH stdin, not remote command arguments. Successful setup
prints the remaining database, web server and deployment steps; it does not
install Shopware or provision shared storage for a host group.
