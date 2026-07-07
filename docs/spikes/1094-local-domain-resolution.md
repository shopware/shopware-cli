# Spike: Local domain resolution for parallel instances (#1094)

Status: first implementation landed (`shopware-cli project proxy`, see
[docs/proxy.md](../proxy.md)) — shared Traefik container + sslip.io hostnames +
local CA · Related: [#1094](https://github.com/shopware/shopware-cli/issues/1094), [#939](https://github.com/shopware/shopware-cli/issues/939)

Goal: each local Shopware instance reachable at a stable hostname (e.g.
`https://instance1.<dev-domain>`) instead of shifting ports, without manual
`/etc/hosts` edits, on macOS, Linux and Windows, with HTTPS that browsers trust
(payment providers require it).

This document maps the current state of shopware-cli, analyzes how
[0ploy/zdev](https://github.com/0ploy/zdev) solves the same problem, lays out
the design space (DNS, routing, TLS), and ends with a recommendation and the
open questions the spike needs to answer.

---

## 1. Current state in shopware-cli

The important architectural fact: **shopware-cli does not own the runtime
layer at all.** There is no `project up`, no compose file generation, no port
allocation, no TLS/DNS code anywhere in this repo. The pieces involved today:

| Concern | Where it lives today |
| --- | --- |
| Web server / ports | External: `shopware/docker-dev` package + Symfony Flex recipe (`shopware/recipes`), pulled in at `composer install`. Caddy runs *inside* the container. |
| Docker wiring in `project create` | `internal/packagist/project_composer_json.go:74-76,152-158` (adds `shopware/docker-dev`, sets `extra.symfony.docker`) |
| Instance URL shown to the user | Hardcoded `http://127.0.0.1:8000` in `cmd/project/project_create.go:624-637` |
| Shop URL the CLI talks to | `.shopware-project.yml` → `internal/shop/config.go:25` (`Config.URL`), resolved in `internal/shop/client.go:56-60` |
| `.env` handling | Read-only loading (`internal/envfile/envfile.go`); the CLI never writes `APP_URL`; no sales-channel domain logic exists |
| Proxies the CLI already runs | Plain-HTTP reverse proxies for `project image-proxy` (`:8080`) and `extension admin-watch` (`:8080`), both with `--external-url` escape hatches |
| Port collisions | Not handled anywhere. `project doctor` is the documented future home for port/env checks (`architecture.md:93`) |
| Hostname constraints | Project names are already validated as Docker-Compose-safe (`cmd/project/project_create.go:63-76`) — conveniently, that charset (`[a-z0-9][a-z0-9_-]*`) is also DNS-label-safe except `_` |

Consequence: this feature necessarily spans **three repos** — shopware-cli
(orchestration, UX, DNS/cert bootstrap), `shopware/docker-dev` (compose
service definitions / proxy labels), and `shopware/recipes` (what
`composer install` materializes into a project).

## 2. Case study: how zdev does it

zdev (Go, same stack as shopware-cli) is the closest prior art and already
ships a Shopware template
([zdev-template-shopware](https://github.com/0ploy/zdev-template-shopware):
PHP 8.4, MariaDB 11.4, uses shopware-cli itself for asset builds). Its
mechanism, from `internal/services/router.go`, `internal/ssl/certs.go` and the
docs:

1. **DNS: wildcard public domain.** `*.0ploy.dev` is public DNS that resolves
   to `127.0.0.1`. Nothing is installed on the machine — any
   `<project>.0ploy.dev` hits loopback on every OS. A custom domain can be
   configured globally.
2. **Routing: one shared Traefik container** bound to `80:80`/`443:443`, using
   the Docker provider with `exposedbydefault=false`. Each project lives in
   its own isolated Docker network; a shared network bridges projects to the
   router. Routing is by `Host()` rule derived from the project name, so no
   per-project host ports exist at all — port collisions disappear by
   construction.
3. **TLS: mkcert.** `zdev systemcheck` installs the mkcert local CA into the
   system/browser trust stores once, generates certs, and mounts them into
   Traefik (`/etc/traefik/certs`, read-only).
4. **App wiring.** The Shopware template sets `APP_URL` to the HTTPS zdev URL,
   creates the sales channel against it during
   `bin/console system:install --basic-setup`, and sets
   `SYMFONY_TRUSTED_PROXIES=private_ranges` because Traefik terminates TLS and
   forwards plain HTTP (without it: redirect loops, `Secure` cookie and
   mixed-content breakage — the exact class of bugs #1094's HTTPS requirement
   is about).

All four ingredients are needed regardless of which tools we pick. zdev's
choices (public wildcard DNS + shared Traefik + mkcert) are one coherent
combination; the design space per ingredient is below.

## 3. Design space

### 3.1 Hostname resolution

| Option | macOS | Linux | Windows | Offline | Privileges | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| **A. Public wildcard DNS** (`*.<dev-domain>` → `127.0.0.1`, zdev-style) | ✅ | ✅ | ✅ | ❌ | none | Zero install. Requires Shopware to own/operate a domain (or reuse `0ploy.dev` / `traefik.me` / `localtest.me`). Breaks offline and behind routers/resolvers with DNS-rebinding protection (FritzBox default, some corporate DNS) — needs a documented fallback. |
| **B. `*.localhost`** | ⚠️ browsers only | ✅ | ⚠️ browsers only | ✅ | none | Chrome/Firefox/Edge resolve any `*.localhost` to loopback internally; systemd-resolved does too. But macOS's system resolver (and hence `curl`, PHP CLI, store API scripts) does not, and Windows behavior varies by version. Fine as a browser-only default, frustrating for API/CLI work. |
| **C. Local DNS resolver** (OrbStack/DDEV-style) | ✅ `/etc/resolver/<tld>` + dnsmasq | ✅ systemd-resolved drop-in or dnsmasq | ⚠️ NRPT rule (`Add-DnsClientNrptRule`) or Acrylic DNS | ✅ | admin, once | True wildcard, offline-capable. Highest setup complexity and support surface; Windows is the weak spot. This is what OrbStack does for `*.orb.local` — but OrbStack ships a whole VM and its own resolver. |
| **D. Hosts-file management** (per instance) | ✅ | ✅ | ✅ | ✅ | admin, per change | No wildcards, needs elevation on every add/remove. #1094 explicitly wants to avoid manual edits; automated edits are possible (cf. `hostctl`) but the elevation prompts are poor UX. Viable only as the *fallback* for option A when offline. |

**TLD choice matters:** avoid `.local` — it is reserved for mDNS (RFC 6762)
and actively conflicts with Bonjour on macOS. Sane choices: an owned real
domain (option A), `.localhost` (RFC 6761), or `.internal` (ICANN-reserved for
private use, 2024). The issue's `shopware.local` examples should become
`shopware.internal` or a real domain.

**Subdomain scheme:** Shopware's admin is a path (`/admin`), not a vhost.
`https://instance1.<dev-domain>/admin` works with zero extra config;
`admin.instance1.<dev-domain>` would need extra vhosting/rewrites inside the
app container for no functional gain. Recommend one hostname per instance,
with the instance name derived from the compose project name (already
validated; `_` must be mapped to `-`). Extra subdomains remain available for
auxiliary services (`mail.<instance>.<dev-domain>` → Mailpit, etc.).

### 3.2 Routing

| Option | Assessment |
| --- | --- |
| **Shared reverse-proxy container** (Traefik or Caddy) on `80/443`, projects join a shared Docker network, routed by `Host` | The zdev/DDEV-proven model. Eliminates host-port allocation entirely — the strongest answer to both #939 and #1094. Traefik: label-driven, zdev has working Shopware labels to copy. Caddy: already the web server in `docker-dev`, and its `internal` CA can mint certs for any hostname on the fly (see 3.3). |
| Per-instance host ports + hostnames that only *name* those ports | Doesn't remove port juggling (URLs like `instance1.dev:8001`), fails #1094's "stable URL" goal. Only worth it as the degraded mode when 80/443 are taken. |
| Document OrbStack's built-in `*.orb.local` | Zero work, already solves this for OrbStack users (every container gets a domain). macOS-only and vendor-specific; worth a docs paragraph regardless of what we build. |

Consideration either way: the proxy must handle websockets/HMR
(`admin-watch`, Vite) and HTTP/2; both Traefik and Caddy do. Rootless
Docker/Podman cannot bind 80/443 without `sysctl net.ipv4.ip_unprivileged_port_start`
adjustments — needs a `project doctor` check and a high-port fallback.

### 3.3 TLS

| Option | Trust UX | Wildcard | Notes |
| --- | --- | --- | --- |
| **mkcert** | One-time CA install (admin prompt); handles system stores *and* Firefox/NSS | ✅ (`*.<dev-domain>`) | Battle-tested (zdev, many others). Adds a binary dependency — but shopware-cli could embed the same logic via `filippo.io/mkcert` internals or shell out. |
| **Caddy internal CA** | Caddy auto-generates a local CA; trust install still required once (Caddy can attempt it, or we run `caddy trust`) | ✅ (on-demand per hostname, no pre-generated wildcard needed) | Fewest moving parts if Caddy is the router; certs appear automatically for any new instance. Firefox trust store needs the same NSS handling mkcert already solved. |
| Traefik default self-signed | Browser warning on every instance | — | Not acceptable: the payments-testing requirement is precisely about *trusted* HTTPS. |
| Real certs (Let's Encrypt wildcard for the public dev domain) | Perfect | ✅ | Requires shipping the private key to every dev machine → forbidden by CA policy, key would be revoked. Not viable. |

Either mkcert or Caddy-internal-CA works; the decision is coupled to the
router choice. Both require one privileged trust-store operation per machine —
that belongs in a `project doctor`-style check with a clear explanation.

### 3.4 Shopware wiring (needed under every variant)

- Write `APP_URL=https://<instance>.<dev-domain>` into `.env.local`
  (`project create`, plus a command to re-point an existing project).
- Sales-channel domain must match: on fresh installs `system:install
  --basic-setup` picks up `APP_URL`; for existing shops run
  `bin/console sales-channel:update:domain <host>` (or admin API) — today the
  CLI has **no** sales-channel domain logic at all.
- `TRUSTED_PROXIES`/`SYMFONY_TRUSTED_PROXIES=private_ranges` whenever TLS
  terminates at the proxy.
- Surface the URL: replace the hardcoded `127.0.0.1:8000` next-steps text in
  `project_create.go`, store the URL in `.shopware-project.yml` (`Config.URL`)
  so every existing CLI command (admin API, watchers) picks it up, and report
  it in `project doctor`.
- `docker-dev` / `shopware/recipes`: add the proxy network + Traefik/Caddy
  labels (or a proxy-aware compose override) derived from
  `COMPOSE_PROJECT_NAME`.

## 4. Recommendation

**Adopt the zdev architecture** (it is proven with Shopware specifically —
`zdev-template-shopware` exists and even uses shopware-cli internally), with
these parameters:

1. **DNS:** Option A — a Shopware-owned wildcard dev domain resolving to
   `127.0.0.1` (e.g. `*.shopware.run` or similar), because it is the only
   zero-install, all-OS answer. Ship a documented offline/rebind-protection
   fallback (hosts-file entries via an explicit `--hosts` mode, option D).
2. **Routing:** one shared proxy container managed by shopware-cli
   (`shopware-cli project proxy up|down|status` or implicit on first use);
   instances attach via labels contributed by `docker-dev`/the Flex recipe.
   Router choice Traefik vs. Caddy should be decided in a prototype — Traefik
   copies zdev's known-good config, Caddy matches the existing docker-dev
   stack and gives on-demand local certs.
3. **TLS:** mkcert-style local CA installed once via a guided
   `project doctor` step (or Caddy's `internal` CA if Caddy wins the router
   decision).
4. **Shopware wiring** as in 3.4, landing across shopware-cli + docker-dev +
   recipes in one coordinated change.

An explicit alternative to weigh before building: **integrate with zdev
instead of rebuilding it** — e.g. `project create` offering a zdev-based
setup, since the template already exists and zdev is Go/MIT. Decision factors:
maintenance ownership, Windows support maturity, and how much of zdev's scope
(shared services, Mutagen sync) Shopware wants to take on vs. only the
routing slice.

## 5. Open questions for the spike (per issue comment)

Cross-platform:
- Windows: native Docker Desktop vs. WSL2 — does the shared proxy bind on the
  Windows side or inside WSL2, and does the wildcard domain resolve in both?
- Rootless Docker / Podman: privileged-port binding for 80/443.
- What happens when 80/443 are already taken (IIS, other stacks, DDEV,
  Laravel Valet/Herd)? Detect and fall back to high ports + print URLs?
- Coexistence with OrbStack (which already grabs `*.orb.local` and
  containers-as-hosts) and with DDEV's Traefik on the same machine.

Certificates:
- mkcert binary dependency vs. embedding cert logic in shopware-cli vs. Caddy
  internal CA — who owns the CA lifecycle (rotation, uninstall/cleanup)?
- Firefox/NSS and corporate-managed machines where trust-store writes are
  blocked — what is the degraded mode?

DNS/domain:
- Who registers and operates the wildcard domain, and what is the SLA? (It
  becomes dev-critical infrastructure; cf. the `traefik.me` outage history.)
- DNS-rebinding protection prevalence (FritzBox, corporate resolvers): how
  loud should detection/diagnostics be in `project doctor`?
- Offline development: is the hosts-file fallback automatic or opt-in?

Product:
- Hostname scheme: `<folder-name>.<dev-domain>` with collision handling when
  two folders share a name? Custom names in `.shopware-project.yml`?
- Multi-sales-channel/multi-domain shops locally (e.g. `en.<instance>.<dev-domain>`)?
- Does this become the default for `project create --docker`, or opt-in
  (backward compatibility for existing `127.0.0.1:8000` setups)?
- Scope split: what exactly changes in `shopware/docker-dev` and
  `shopware/recipes`, and how are version mismatches between CLI and recipe
  handled?

## 6. Suggested spike follow-ups

1. Prototype: shared Traefik + mkcert + one `docker-dev` project with labels,
   on all three OSes (validate the matrix in 3.1, esp. Windows/WSL2).
2. Same prototype with Caddy + internal CA for comparison.
3. Decide build-vs-integrate re: zdev.
4. Registrar/ops decision on the wildcard dev domain.
5. Break down implementation issues per repo (shopware-cli, docker-dev,
   recipes, docs).
