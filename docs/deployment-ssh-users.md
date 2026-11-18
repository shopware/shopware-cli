# SSH deployment and PHP users

Set `user` to the SSH login and `http_user` to the account running PHP-FPM or
the web server in `.config/shopware-project.yml`.

```yaml
environments:
  production:
    type: ssh
    ssh:
      host: shop.example.com
      user: deploy
      http_user: www-data
      directory: /srv/shop/current
```

The deployment account owns releases, runs the Shopware Deployment Helper and
hooks, and manages private deployment state. When the SSH login needs to switch
to that account, add `become: shop` under `ssh`. The CLI then runs remote
deployment commands with `sudo -n -H -u shop`. The login needs passwordless
sudo for that command, and the target account needs access to the deployment
root. For existing deployments, provision access to the existing files before
changing the deployment account.
Each named host can override `user`, `become` and `http_user`.

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

Omitting both options keeps the single-account behavior and requires no ACL
tools or sudo.
