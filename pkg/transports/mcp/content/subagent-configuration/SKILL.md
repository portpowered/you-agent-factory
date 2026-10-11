---
name: subagent-configuration
description: Inspect subagent providers and configure operator defaults or custom ACP providers using the operator configuration file.
---

# Configure subagent defaults and providers

Use this skill when a customer asks which subagent runs by default, where the
operator configuration lives, how to change defaults, or how to add a custom
ACP agent such as OpenCode.

The MCP skill URI is `skill://subagent-configuration/SKILL.md`.

## Configuration file and precedence

The shared operator configuration is `.you-agent-factory/config.json` in the
user's home directory:

- Windows: `%USERPROFILE%\.you-agent-factory\config.json`
- macOS and Linux: `~/.you-agent-factory/config.json`

Direct START uses `defaults.subagent`, independently of generic worker defaults:

```json
{
  "defaults": {
    "subagent": {
      "provider": "opencode",
      "model": "explicitly-advertised-free-model"
    }
  }
}
```

Select an exact model advertised by the provider catalog; the model above is
a placeholder. Edit `defaults.subagent` in the selected host's operator
configuration file. Preserve the other configuration fields. Read the file
again to confirm the dedicated route and unchanged generic worker defaults.

Per-call provider, model and reasoning effort override the dedicated route.
A different provider requires an explicit model. Missing or unavailable routes
fail without fallback. Generic worker defaults and environment values do not
select the direct subagent route.

Use the read-only MCP resource `you://operator/config/current` to inspect the
current file. It returns the raw JSON configuration, or `{}` when no file is
present. Use `you://operator/config/schema` for the configuration schema.
These resources are authoritative for the running server's current values and
supported fields. Do not expose credentials or other sensitive values when
reporting configuration to a customer.

## Discover provider names and models

Read MCP resource `you://providers/catalog` or run `you providers list` (or `you providers list --json`) to inspect provider
identities, aliases, exact model IDs, effort values, capabilities, and
readiness. Use the canonical provider `id` in configuration. Do not invent
provider aliases or model IDs. Readiness can be unverified until a request-time
probe or a real invocation checks the local executable and account.

First-party provider identities include `codex`, `claude`, and
`antigravity` (AGY). ACP identities include the built-in presets listed by
`you providers list`, such as `cursor-acp`, `kiro-acp`, `opencode-acp`, and
`gemini-acp`. The catalog output is the source of truth for the complete set
and aliases available on this installation. Provider/model catalogs can differ
with the installed software and account.

The detailed provider guide is `docs/reference/providers.md` in the source
repository. The MCP server's `you://operator/config/schema` resource documents
the shape of the operator configuration.

## Add or change an ACP subagent

ACP means Agent Client Protocol. Install and authenticate the agent first, then
confirm its ACP launch command. The built-in OpenCode preset uses
`npx -y opencode-ai acp`; inspect `you providers list` for the preset identity
and current availability.

Use the CLI to add an ACP provider and validate the resulting configuration:

```text
you workers acp add --name company-opencode --transport stdio --argument "npx -y opencode-ai acp"
you workers list
you providers list
```

Use a stable lowercase provider name. `--argument` carries the full launch
command as one value. The supported transport is `stdio`. The integration is
stored under `workers.acp.integrations`. Its `id` is a settings-entry ID;
workers select it by its `name` (the provider identity). Adding an existing
built-in name overrides that preset's launch command; deleting that override
restores the built-in preset.

To select the custom provider for START, set `defaults.subagent.provider`
in the operator configuration to its canonical `name` and set
`defaults.subagent.model` to an explicit supported model. To set it for one worker,
configure that worker's `modelProvider` to the same name. Choose a
model only when the provider catalog reports an exact supported model ID.
Custom ACP providers may not publish model metadata, so do not guess one.

When editing the file, preserve existing keys and use this shape:

```json
{
  "workers": {
    "acp": {
      "integrations": [
        {
          "id": "generated-settings-entry-id",
          "name": "company-opencode",
          "transport": "stdio",
          "command": "npx -y opencode-ai acp"
        }
      ]
    }
  }
}
```

The schema resource `you://operator/config/schema` is the contract for all
supported fields and constraints. Configuration validation confirms shape;
it does not confirm that the executable is installed, authenticated, or able
to complete work. Check the provider catalog, then run a small real task to
verify the integration.

ACP integration entries do not contain timeout or permission policy. Use the
normal worker or Factory Session limits for execution timeouts and the
invocation's supported permission setting for permission behavior. The
packaged `@you/subagent` worker sets `skipPermissions: true`. Built-in OpenCode
ACP can select an allow option when its peer requests permission. Custom ACP
integrations without a declared bypass capability retain normal permission
handling.

## Existing MCP capabilities

Read `you://providers/catalog` and `you://operator/config/current` before
changing defaults. Change the configuration using the CLI or by editing the
file, then read both resources again to verify the values visible to the MCP
server. The MCP server exposes `you.subagent` for invocation; configuration
and catalog data are resources. Do not claim that a change is active until a
small real invocation confirms it.

Long-running subagent invocations are synchronous. Do not assume a one-hour
invocation will complete: the MCP client, server transport, and provider may
each enforce a timeout. For long tasks, inspect the tool's timeout behavior
and report the configured limit. A timeout should be surfaced as an execution
failure rather than treated as a successful result.

## Message and continue Worker Sessions

Use the existing `you.subagent` MESSAGE action to ask your own lead or requester,
then pull replies with `LIST` and `messages: true`. Use `inReplyTo` for replies.
The selected host verifies execution credentials and recipient authority.
Never include caller tokens in tool arguments or output.

Use explicit REVIVE with a stable request and successor identity to continue an
ended Worker Session without creating an Agent Message. Admission does not
confirm delivery. Read `you docs mcp` and `you docs messaging` for the complete
request shapes, inherited HOLD policy, finite paging, and delivery facts.
Messages remain an independent side channel. Keep the mailbox as the fallback
when a factory scope question cannot reach its lead through messaging.
