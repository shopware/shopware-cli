# 1. Shopware AI integrations in `shopware-cli`

- Status: Proposed
- Date: 2026-08-06

## Context

Add a top-level `ai` command to Shopware CLI that gives developers and automation agents a way to discover and connect Shopware-provided AI integrations. The first phase would cover Shopware-provided integrations only. Support for user-supplied and third-party integrations would be considered separately.

Shopware's portfolio of AI integrations will grow over time. Users should not need to know the internal structure of the company to find them. A user should be able to ask, "Does Shopware provide something that helps with this?" and get a useful answer from the CLI.

## Decision

The CLI would:

1. Show the Shopware AI integrations it knows about.
2. Briefly explain what each integration is for and how it is delivered.
3. Configure or install it in a supported AI client.

The CLI would act as the front door, not as the runtime. It would not host MCP servers, run agents, perform authentication, monitor availability, or take over maintenance of the integrations it exposes.

## Consequences

### Ownership and maintenance

Being listed means that the CLI team has accepted the integration into the Shopware directory. It does not mean that the CLI team has taken over its operation or subject-matter support. The CLI team may deprecate or remove an entry when it becomes unavailable, unsafe, misleading, incompatible, or clearly unmaintained. Removal from the directory must not silently uninstall an existing integration from users' projects. Existing installations can instead be shown as deprecated or no longer available. When an active directory entry is removed, tombstone metadata (including lifecycle status, removal reason, and replacement information) must be retained in the manifest so that existing installations remain discoverable. Removing an active directory entry must not remove the corresponding tombstone metadata from later CLI releases.

The CLI team owns the `ai` command, the directory format, supported client adapters, safe configuration changes, the bundled CLI skill; and reviewing, presenting, deprecating, and removing directory entries.

The contributing team owns the integration itself, its content and behavior, its service availability, its compatibility, its documentation, its authentication system, its ongoing maintenance.

## Additional context

### Initial Shopware integrations

#### `shopware-cli` skill

The CLI skill is bundled with the binary and maintained as part of `shopware-cli`: `shopware-cli ai add shopware-cli --client <client>`. Its version follows the installed CLI version. It does not need a separate download, registry entry, or release process.

#### Deployment Helper skill

The Deployment Helper skill covers a workflow that spans both the CLI and Deployment Helper. It is strongly connected to the CLI, but is released separately. The CLI will make the skill discoverable and install it into a supported client.

#### Shopware Core MCP Server

As part of the installed Shopware platform, it can be detected by the CLI whether the current project supports it. The CLI only helps connect the client to the server already provided by the project.

#### Planned, non-platform MCP Servers, skills, etc.

If an MCP Server is a remote server, users can connect an MCP-capable assistant to its endpoint and authenticate through their Shopware account. The client handles the browser-based OAuth flow; the CLI only writes the connection configuration. Support can be added once the MCP Server provides a stable, publicly usable endpoint and documentation. The CLI design should be ready for it without depending on its delivery timeline.

#### Future Shopware skills

Other teams may contribute integrations such as browser-testing or accessibility skills. These will appear in the same list and use the same installation flow. From the user's perspective, they are simply Shopware-provided integrations.

### A curated, baked-in directory and self-service contributions

For now, we do not plan on a new registry service or central platform. Instead, Shopware CLI will contain a small, curated directory of integrations it knows how to handle. Another Shopware team will contribute their AI tool/"entry" via a pull request to the CLI repository. An entry must include the minimum information needed by the CLI:

- stable name;
- short (75 characters or less) public description;
- integration type (MCP Server, skill);
- installation or connection details;
- public documentation link;
- compatibility requirements;
- current lifecycle status;
- an internal maintainer-team contact (GitHub team name, added to a file maintained in the CLI repository).

Additional integration types such as "agent" may be added once their delivery target, adapter, versioning requirements, and safety rules are defined.

The internal contact is used by the CLI team when an entry needs attention. The contribution process should remain lightweight. It exists to ensure that an entry:

- is genuinely provided by Shopware;
- has enough documentation to be useful;
- can be installed or configured safely;
- does not require the CLI to store secrets;
- has a named internal maintainer;
- fits the supported integration model.

This is a curated directory, not a promise that every AI-related project created inside Shopware will automatically be listed.

The directory will be stored as a YAML manifest in the `shopware-cli` repository, embedded into CLI releases, and validated in CI against a generated JSON Schema. One manifest file should be sufficient initially.

### Safety and automation

All commands must work for both people and automation agents.

Writing commands must:

- support `--dry-run`;
- show the target file and the exact planned change;
- require an explicit client when the destination is ambiguous in non-interactive use;
- structurally redact sensitive keys and values (OAuth tokens, client secrets, connection credentials) in all output, including dry-run previews, diffs, and written files, while preserving the exact planned-change structure for non-sensitive data;
- report every file they change;
- remove only configuration previously created by `shopware-cli`;
- support stable machine-readable output: JSON format on stdout, versioned schema, errors on stderr with structured JSON error shape, and consistent exit-code semantics (0 for success, non-zero for failure).

For remote services using OAuth, login and token storage remain the responsibility of the AI client and the remote service. The CLI writes only the connection information.

#### Supported AI clients

The initial client adapters will support:

- Claude;
- Codex;
- GitHub Copilot;
- Cursor.

Support for additional clients can be added later through the same adapter model.

#### Shopware Core MCP detection

The CLI derives the Core MCP endpoint from the current Shopware project and checks whether the installed Shopware version provides it.

Compatibility information is supplied and maintained by the team responsible for Shopware Core. The CLI consumes that information; it does not define or independently guarantee Core MCP compatibility.

#### Separately maintained skills

Separately maintained skills are published and versioned by their owning team.

The CLI resolves and installs the latest compatible release by default (not just the latest available release) and records the tag or immutable revision that was installed as the current version. This allows the CLI to report what is installed and, later, determine whether an update is available. Version precedence is deterministic (semantic versioning order); pre-releases are excluded unless explicitly requested; unknown compatibility is treated as incompatible; and installation must fail with a clear error when no compatible release is available.

The owning team is responsible for:

- publishing releases or maintaining the relevant release tag;
- declaring supported CLI, Deployment Helper, Shopware, or client versions;
- providing and maintaining compatibility checks;
- updating the skill when its dependencies change.

The CLI team is responsible only for resolving the published version, installing it safely, and reporting compatibility information supplied by the owner.

#### Contribution requirements

A Shopware-provided integration must include:

- a stable name and clear public description;
- public documentation;
- installation or connection metadata;
- a named internal maintainer;
- declared compatibility requirements;
- compatibility checks maintained by the owning team.

The CLI team reviews whether the entry can be presented and installed safely. It does not take ownership of the integration's behavior, content, availability, or compatibility.
