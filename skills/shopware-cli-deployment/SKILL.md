---
name: shopware-cli-deployment
description: Configure and troubleshoot Shopware CLI SSH deployments, including separate SSH, deployment, and PHP accounts, shared permissions, and release pruning.
---

# Shopware CLI SSH deployment

Check `shopware-cli project deploy --help` for the available commands. The
experimental deployment commands require a CLI built with the `deployment`
build tag. Use `shopware-cli project deploy init -e production` to initialize
the target and review its preflight checks before rolling out a release.

## Configure deployment and PHP accounts

Set `user` to the SSH login and `http_user` to the account running PHP-FPM or
the web server in `.config/shopware-project.yml`. Set `become` when the SSH
login needs to switch to a separate deployment account.

```yaml
environments:
  production:
    type: ssh
    ssh:
      host: shop.example.com
      user: ci
      become: deploy
      http_user: www-data
      directory: /srv/shop/current
```

The deployment account owns releases, runs the Shopware Deployment Helper and
hooks, and manages private deployment state. With this configuration the CLI
runs remote deployment commands with `sudo -n -H -u deploy`. The login needs
passwordless sudo for that command, and the target account needs access to the
deployment root. For existing deployments, provision access to the existing
files before changing the deployment account. Each named host can override
`user`, `become` and `http_user`.

When the PHP runtime uses a different account, install the Linux `acl` tools
on the target and use a filesystem that supports POSIX ACLs. The deployment
account needs permission to set ACLs on the shared files and directory roots.
The PHP account needs traversal access to the deployment path and read access
to application code. The rollout grants the following permissions.

- Read access to configured shared files, including the default `.env.local`.
- Write access to the release's runtime cache and configured shared directories.
  Default ACLs give both accounts access to new content, subject to the modes
  requested by the application.

For existing shared directory trees, provision access to existing contents
once. Rollouts set ACLs on the shared directory roots without recursively
rewriting live media. On shared storage, provision the ACLs for the numeric
user IDs of all participating hosts. Use a new release name when changing the
PHP runtime user and remove obsolete ACLs when retiring an account.
Use the CacheTool integration for OPcache resets with a separate PHP user.

During rollout the deployment account prepares Shopware's HTMLPurifier cache
directories before the first web request. PHP can then use its usual `0022`
umask. Repeating a rollout or rolling back also works with caches created by
the PHP account. If an older release or a plugin creates private directories,
prune reports the inaccessible cache path before starting cleanup. Grant the
deployment account read, write and traverse access to that directory as its
owner, then retry prune.

Omitting both options keeps the single-account behavior and requires no ACL
tools or sudo.

## Pruning after an activation

`project deploy prune` keeps a recently deactivated release for at least
120 seconds, including after rollback and with `--keep 0`. A higher
`realpath_cache_ttl` on the target's PHP CLI extends the grace period. Both
the preview and the actual prune list releases retained for PHP workers.
Run prune again after the grace period to reclaim them. If the web server
caches paths for longer or serves long-running requests, schedule cleanup
after those workers have drained.
