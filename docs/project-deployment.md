# Project deployment packaging

## Experimental build

All `project deployment` commands are excluded from normal builds. To enable
them, build the CLI with the `deployment` Go build tag:

```shell
go build -tags=deployment -o shopware-cli .
```

For development and testing:

```shell
go run -tags=deployment . project deployment --help
go test -tags=deployment ./cmd/project
```

The tag controls command registration; reusable build and packaging libraries
remain available without it. Release builds do not enable this tag.

## Archive

Build a deployable `tar.gz` without uploading or activating it:

```shell
shopware-cli project deployment package archive -e production
shopware-cli project deployment package archive ./my-shop \
  -e production --output ./artifacts/shopware.tar.gz
```

With no project directory, the CLI finds the closest Shopware project from the
current directory. By default it reads the config from that project directory;
an explicit `--project-config` path is relative to the caller's working directory.
`--env` selects the project environment, but packaging always builds locally.
An SSH connection is not needed.

The command copies the working tree (including uncommitted files) into a private
temporary directory, runs the same build pipeline as `project ci`, and archives
the result. The original sources are not built in place. `--with-dev-dependencies`
includes Composer development dependencies, just as it does for `project ci`.

The default output is a uniquely named file at
`.shopware-cli/deployments/shopware-<unique-id>.tar.gz` in the project directory.
An explicit `--output` path is relative to the caller's working directory.
Existing files are never overwritten, and a reference is published only after
the archive has been fully written. The final path is printed after build output.
The temporary build directory is removed on success, failure, or cancellation.

### Archive contents

The archive contains project files at its root, with no enclosing directory.
Executable permissions and contained relative symlinks are retained.

The following are excluded from the build copy and final archive:

- `.git` and `.shopware-cli` directories/files.
- `.env` and `.env.*`, except `.env.dist` and `.env.example`.
- Local `.shopware-project.local.yml` / `.yaml` configuration.
- `var`, `public/media`, `public/thumbnail`, and `public/sitemap`.

`auth.json` is available during the private build but excluded from the archive.
Project config files are replaced by a deployment-only `.shopware-project.yml`
(including the compatibility date); CLI environment/Admin API credentials are
not distributed. Custom config files, included configs, and their local
overrides are also excluded from the final archive.

Runtime environment variables and persistent data must be supplied by the
deployment environment. Build-machine caches are deliberately not packaged.
These exclusions are not a general secret scanner: project code and deployment
settings must still be reviewed before distributing an artifact.

### Isolation and symlinks

Absolute, broken, or project-escaping source symlinks are rejected instead of
following them into the original checkout or another directory. Composer path
repositories are installed in mirror (copy) mode for archive builds. Relative
path repositories outside the project directory are not copied into the
temporary workspace and must be made available through a suitable Composer
repository instead.

Build hooks and Composer scripts remain trusted project code with the user's
normal permissions. A temporary build directory is not a security sandbox.

## Container

Generate standalone build files to commit, and print a Docker command to run:

```shell
shopware-cli project deployment package container --load -t my-shop:latest
shopware-cli project deployment package container ./my-shop \
  --load -e production --php-version 8.4 --platform linux/amd64 \
  -t my-shop:release -t my-shop:latest
```

To prepare a command that pushes to a registry:

```shell
shopware-cli project deployment package container --push \
  -t ghcr.io/my-org/my-shop:release
```

Project discovery and `--project-config` resolution work the same way as archive
packaging. The command writes `Dockerfile` and `.dockerignore` into the project
root. **It never executes Docker**, even with `--load` or `--push`. No Docker
installation or Composer credentials are needed to generate the files.

Generated file paths go to stderr; stdout contains a copyable POSIX-shell
`docker buildx build` command with the absolute project path as its context.
Review and run it yourself when ready. `--load`, `--push`, repeatable `--tag`,
and `--platform` only customize that command. Both output flags default to false,
leaving output behavior to the selected builder when you run it. Use `--load`
when you want the image loaded into Docker rather than potentially only cached.

If either output already exists (including a symlink), generation fails without
overwriting it or creating the other file. Edit existing files manually and
run Docker directly. There is no separate `--generate` mode.

The multi-stage Dockerfile uses the same template as project scaffolding:

- Build with `ghcr.io/shopware/shopware-cli:latest-php-<version>` and `project ci`.
- Copy the result to `/var/www/html`, owned by UID 82, in
  `ghcr.io/shopware/docker-base:<version>-frankenphp`.
- Use BuildKit Composer and npm caches.

PHP image version selection uses the first available hint:

1. Explicit `--php-version`.
2. `php_version` in the resolved project configuration.
3. `docker.php.version` in that configuration, including local overrides.
4. The highest supported PHP series satisfying the locked `shopware/core` PHP
   requirement in the project's `composer.lock` (`shopware/platform` is the
   fallback package).
5. `8.3` when no PHP hint is available.

Detection is local and does not require PHP, Docker, or network access. An
unreadable/malformed lockfile or an unsatisfied locked Shopware requirement is
reported instead of silently generating a default image. A configuration pin or
explicit `--php-version` takes precedence and bypasses lockfile detection.
`--with-dev-dependencies` includes Composer dev dependencies.
The generated files contain no temporary paths, resolved configuration, embedded
credentials, or wrapper-specific secret requirements.

### Build context and secrets

A manual build uses the project directly as its context and reads the standard
project configuration from there. `--project-config`
selects the generator's settings, but does not copy that configuration or its
includes into the generated files. If the build needs a nonstandard config path,
adapt the generated Dockerfile and ensure it is available in the build context.
Review the context and `.dockerignore` before committing or publishing. Unlike
archive packaging, container file generation does not filter the project or
sanitize its configuration. Keep runtime credentials out of committed project
configuration and exclude local secret files from the Docker context.

Buildx applies `.dockerignore` when transferring the context. The generated
defaults omit dependencies and generated assets. Prebuilt `vendor` or
`public/bundles` are retained when the corresponding Composer installation or
asset-copy step is disabled.

The CLI does not read or merge credentials. For private packages, add BuildKit
secrets to the printed command before running it, for example from the project
root:

```shell
docker buildx build --load -t my-shop:latest \
  --secret id=composer_auth,src=auth.json \
  --secret id=packages_token,env=SHOPWARE_PACKAGES_TOKEN .
```

Alternatively, use `--secret id=composer_auth,env=COMPOSER_AUTH` for environment-
provided Composer authentication. Do not print secrets or copy secret mounts
from custom build hooks.

The generated Dockerfile uses syntax version `1.10`, which supports the
environment-variable secret mount used for `SHOPWARE_PACKAGES_TOKEN`.
Runtime environment variables and persistent data must be supplied when the
image is deployed.

SSH rollout remains a separate follow-up feature.
