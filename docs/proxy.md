# Local domains for parallel instances (`project proxy`)

`shopware-cli project proxy` gives every local Shopware instance a stable
hostname with trusted HTTPS — e.g. `https://my-shop.127.0.0.1.sslip.io` —
instead of `localhost:8000`. This makes running multiple instances in
parallel practical: no port juggling, consistent URLs across restarts, and
HTTPS that works for payment provider testing.

## How it works

- A single shared [Traefik](https://traefik.io) container listens on ports 80
  and 443 and routes requests by hostname to the matching instance over a
  shared Docker network (`shopware-cli`). Instances need no published host
  ports for HTTP anymore.
- Hostnames use [sslip.io](https://sslip.io): any `<name>.127.0.0.1.sslip.io`
  resolves to `127.0.0.1` on every OS without touching `/etc/hosts` or local
  DNS. A custom base domain can be configured with `proxy up --domain`.
- Certificates: when [mkcert](https://github.com/FiloSottile/mkcert) is
  installed, the wildcard certificate is issued by your existing mkcert root
  CA — if you ever ran `mkcert -install`, HTTPS is trusted immediately with no
  extra prompts. Without mkcert, shopware-cli generates its own local CA which
  you trust once via `proxy trust` (set `SHOPWARE_CLI_PROXY_DISABLE_MKCERT=1`
  to force this even with mkcert installed).
- Per project, a small `docker-compose.override.yml` attaches the `web`
  service to the proxy network and sets the Traefik routing labels. `APP_URL`
  in `.env.local` and `url` in `.shopware-project.yml` are updated to match.

## Usage

```bash
# once per machine
shopware-cli project proxy up      # start the shared proxy
shopware-cli project proxy trust   # trust the CA (skip if you already ran "mkcert -install")

# once per project (inside the project directory)
shopware-cli project proxy add
make up                            # or: docker compose up -d

# fresh install: the sales channel picks up APP_URL automatically
make setup

# existing database: point the sales channel at the new domain
docker compose exec web bin/console sales-channel:update:domain https://<name>.127.0.0.1.sslip.io
```

Useful commands:

```bash
shopware-cli project proxy status   # proxy state + running instances and their URLs
shopware-cli project proxy remove   # detach the current project from the proxy
shopware-cli project proxy down     # stop the shared proxy
```

## Options

- `proxy up --domain <domain>` — use a different base domain (e.g. an owned
  wildcard domain or `traefik.me`). The certificate is regenerated to match.
- `proxy up --http-port / --https-port` — use different host ports when 80/443
  are taken. Instance URLs then include the port
  (`https://my-shop.127.0.0.1.sslip.io:8443`).
- `proxy add --name <name>` — subdomain to use (defaults to the sanitized
  project folder name).
- `proxy add --host <fqdn>` — full custom hostname; it is added to the
  certificate automatically.
- `proxy add --service / --upstream-port` — for setups whose compose service
  is not called `web` or does not listen on port 8000.

Proxy state (CA, certificates, Traefik configuration, settings) lives in the
user config directory under `shopware-cli/proxy` and can be overridden with
`SHOPWARE_CLI_PROXY_DIR`.

## Notes & limitations

- sslip.io hostnames need a working internet DNS resolver. Some routers or
  corporate resolvers block DNS answers pointing to private IPs ("DNS
  rebinding protection"); use `--domain` with a hosts-file entry or a local
  resolver in that case.
- Firefox and Chromium on Linux use NSS trust stores. `proxy trust` handles
  them when `certutil` (libnss3-tools) is installed — with mkcert this is
  covered by `mkcert -install` directly.
- The storefront/admin watchers still use their dedicated ports (5173, 8080,
  9998, …) and are not routed through the proxy yet.
- Non-Docker setups (Symfony CLI) are not supported yet; the proxy requires a
  Docker Compose based project such as the default `shopware/docker-dev`
  setup.
