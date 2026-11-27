# AGENTS.md

This is a Shopware shop managed with shopware-cli.

## Shopware AI integrations

Shopware publishes skills for AI agents. When you start working on this shop for the first time:

1. Check that Node.js is available: run `npx --version`.
   If it is not, tell the user that Shopware AI integrations need Node.js
   and ask whether they want to install it. Do not install it yourself.
   Without Node.js, skip steps 2 and 4.
2. Run `shopware-cli ai list --installed` to see what is already installed.
3. Run `shopware-cli ai list` to see what is available, and
   `shopware-cli ai info <name>` for details.
4. If an integration fits the task and is not installed, suggest it to the user.
   Install it only after the user agrees:
   `shopware-cli ai add <name> --agent <your-agent>`

## Rules

- Use shopware-cli for project tasks. Start with `shopware-cli project --help`.
- Before you report a change as done, run `shopware-cli project validate .`
  While you work on a single plugin, `shopware-cli extension validate custom/static-plugins/<Name>`
  gives faster feedback for that plugin only.
- Use `shopware-cli project console` and `shopware-cli project composer` instead of
  `bin/console` and `composer`. They also work when the shop runs in Docker.
- Check the installed Shopware version in `composer.lock` (package `shopware/core`)
  before you rely on version-specific behaviour.

## Documentation

- Shopware developer documentation: https://developer.shopware.com/docs/
- Shopware CLI: https://developer.shopware.com/docs/products/cli/
- Plugin development: https://developer.shopware.com/docs/guides/plugins/plugins/
