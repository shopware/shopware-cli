---
title: Serve Shopware skills over MCP from the CLI
status: Accepted
tags: [ai, MCP Server, Agent Skill, integration]
---

Accepted on 2026-11-19. Implementation is deferred until its value justifies
the effort, for example when popular agents support SEP-2640. See the
discussion in
[shopware/shopware-cli#1945](https://github.com/shopware/shopware-cli/pull/1945).

- Related:
  [2026-08-06 Handling Shopware AI integrations](2026-08-06-ai-command.md),
  [SEP-2640 Skills Extension](https://modelcontextprotocol.io/seps/2640-skills-extension)

## Context

### How skills reach AI agents today

Shopware skills are spread across several repositories:

| Repository                      | Skills                                             |
| ------------------------------- | -------------------------------------------------- |
| `shopware/shopware-cli`         | `shopware-cli`, `shopware-cli-docker`, and others |
| `shopware/deployment-helper`    | `deployment-helper`                                |
| `shopware/acceptance-test-suite`| `acceptance-test-suite`                            |

Every repository keeps its skills in the same place: `skills/<name>/SKILL.md`.

The CLI has a built-in list of these skills, the `ai` directory. Today
`shopware-cli ai add <name>` uses the skills.sh tool to copy a skill's files
into the agent's folder on disk. This has three costs:

- the user installs each skill separately,
- the user installs it again for each agent,
- the user needs Node.js and npx.

### What SEP-2640 adds

SEP-2640 (status: Final) is an MCP extension that lets an MCP server provide
skills to the agent, without copying files to disk:

1. The agent asks the server which skills it has (`skills/list`).
2. The server answers with each skill's name, description and file list. Every
   file comes with a checksum, so the agent can check that it got the right
   content.
3. When the agent needs a skill, it asks the server for the files and the
   server sends them.

The agent only talks to the server. The server decides where the files come
from. So one server can provide skills from many repositories.

### Why the CLI and not another server

We have looked at the other Shopware MCP servers:

- The **Shopware Core MCP server** runs inside a shop. It can only provide what
  is installed in that shop. Development skills, such as the CLI or Deployment
  Helper skills, are not installed in a shop. Core MCP is also written in PHP,
  and the PHP MCP library does not support SEP-2640.
- The **Meteor MCP server** covers the Meteor design system. Its library does
  not support SEP-2640.
- A **new hosted server** would need servers to run and a team to own them.

The CLI already has the list of all Shopware skills and their repositories. It
is the only place that knows them all.

### What this changes in the earlier decision

ADR 2026-08-06 says the CLI only helps users find and install integrations,
and that it does not run MCP servers itself. This ADR makes one exception to
that rule, described below.

## Decision

The CLI gets a new command, `shopware-cli ai serve`. It is an MCP server that
gives the agent every skill from the `ai` directory, using SEP-2640.

### How it works

1. `shopware-cli ai add shopware-skills-mcp` writes this entry to the project's
   `.mcp.json`:

   ```json
   {
     "mcpServers": {
       "shopware-skills-mcp": {
         "type": "stdio",
         "command": "shopware-cli",
         "args": ["ai", "serve"]
       }
     }
   }
   ```

2. When the agent starts a session, it starts `shopware-cli ai serve` and
   talks to it over stdio. Nothing listens on a network port.
3. The agent asks for the list of skills. The CLI answers with every skill from
   the `ai` directory.
4. When the agent needs a skill, the CLI sends its files.
5. When the session ends, the agent stops the process.

### The `shopware-skills-mcp` directory entry

`shopware-skills-mcp` is an entry of type `mcp` in the `ai` directory. Its
delivery uses a new `delivery.kind` value, `mcp-stdio`, for MCP servers that the
agent starts as a local command:

| Field              | Required | Value for `shopware-skills-mcp` |
| ------------------ | -------- | ------------------------------- |
| `delivery.kind`    | yes      | `mcp-stdio`                     |
| `delivery.command` | yes      | `shopware-cli`                  |
| `delivery.args`    | no       | `["ai", "serve"]`               |

`ai info --format json` returns:

```json
{
  "delivery": {
    "kind": "mcp-stdio",
    "command": "shopware-cli",
    "args": ["ai", "serve"]
  }
}
```

The change to `internal/ai/directory/CONTRACT.md` is additive. The contract
allows new `delivery.kind` values, and entries with `kind: mcp` still require
`url` and `transport`.

### Where the skill files come from

- The CLI downloads every skill from its repository: the `skills/<name>/`
  folder on the default branch. This is the same source that `ai add <name>`
  installs from. Because of the cache described below, `ai serve` can serve an
  older revision than a new `ai add` installs.
- The CLI does not ship its own skills inside the binary. ADR 2026-08-06 was
  updated on 2026-10-13 to remove a bundled skill. If `ai serve` shipped skills
  inside the binary, it could give out a different version of a skill than
  `ai add` does.
- The CLI keeps the downloaded files in its cache folder for 24 hours. When the
  cache is older, the CLI downloads again. If the download fails, the CLI uses
  the old copy.
- If a skill is not in the cache and cannot be downloaded, the CLI leaves it out
  of the list and prints a warning.
- If a skill's `SKILL.md` frontmatter does not parse, the CLI leaves the skill
  out of the list and prints a warning. The skill's owners fix the file in their
  repository.
- While one `ai serve` process runs, the files of a skill do not change, even
  if the repository changes.

### Limits of the exception

`ai serve` only does what is described above. It does not:

- provide anything other than skills from the `ai` directory,
- change files, call APIs, or do anything except send skill files,
- handle logins, tokens or other secrets,
- open a network port or run as a hosted service,
- start or connect to any other MCP server.

The rest of the 2026-08-06 decision stays as it is.

### What does not change

- `ai add`, `ai remove`, `ai list` and `ai info` keep their current behaviour.
  `shopware-skills-mcp` is one more MCP server entry in the `ai` directory. It
  is added and removed the same way as `meteor`.
- Installing skills as files through `ai add` stays the recommended way.
  Agents that do not support SEP-2640 yet still need it.
- Teams that own a skill still own its content.

## Consequences

### Benefits

- One entry in `.mcp.json` gives the agent every Shopware skill from every
  repository.
- No separate install per skill or per agent.
- No Node.js or npx needed for this path.
- Changes to a skill reach agents within 24 hours, without the user doing
  anything. New skills added to the `ai` directory need a CLI update, because
  the directory ships with the binary.

### Costs and risks

- The CLI depends on the Go MCP library
  (`github.com/modelcontextprotocol/go-sdk`).
- The CLI needs new code to download and cache skill files. It only downloads
  from the Shopware repositories listed in the `ai` directory, the same
  repositories `ai add` installs from today.
- The agent starts `shopware-cli` by name, so `shopware-cli` must be on the
  PATH. Users who run the CLI only through Docker need extra setup, and the
  documentation must explain it.
- Popular agents do not support SEP-2640 yet. Until they do, `ai serve` is ready
  but nobody uses it.
- Whatever is on a repository's default branch reaches agents within 24 hours.
  A broken skill reaches users just as fast. This is the same as `ai add`
  without a version today.

### Ownership

The DX Tools team owns `ai serve`: the command, downloading, caching, and
following SEP-2640. The teams that own a skill own its content, as before.

## Next steps when implementation starts

Each step is its own pull request, in this order:

1. **Update ADR 2026-08-06.** Add a note that this ADR makes one exception to
   "the CLI does not run MCP servers".
2. **Add `shopware-cli ai serve` and the `shopware-skills-mcp` entry.** One pull
   request with:
   - the `ai serve` command, downloading and caching, and the SEP-2640
     messages (`skills/list`, `skills/get`, reading files);
   - the `shopware-skills-mcp` entry in the `ai` directory. `ai add` already
     writes MCP server entries to `.mcp.json`, for example for `meteor`, but
     only for remote servers with a URL. This step adds the `mcp-stdio`
     delivery kind described above, writes it to `.mcp.json` as a
     `"type": "stdio"` entry with `command` and `args`, and updates
     `internal/ai/directory/CONTRACT.md`. After this, `ai add
     shopware-skills-mcp` and `ai remove shopware-skills-mcp` work like they do
     for `meteor`;
   - tests, including a test that runs the built binary.
3. **Document it.** `docs/AGENT_SKILLS.md` explains what `ai serve` is and how
   to turn it on with `ai add shopware-skills-mcp`.

Later, not planned yet:

- The first version implements the SEP-2640 messages in the CLI, on top of the
  current Go MCP library. It does not wait for the library. When the library
  adds built-in SEP-2640 support, replace the CLI's implementation with it to
  reduce maintenance.
- Decide whether users can pin a skill version for `ai serve`, as `ai add
  <name>@<tag>` already allows.
