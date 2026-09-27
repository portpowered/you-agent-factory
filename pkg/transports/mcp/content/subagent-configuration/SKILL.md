---
name: subagent-configuration
description: Configure default worker provider and model settings, inspect the operator configuration, and add or update ACP subagent providers.
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

The `defaults` object can set `workerModelProvider` and `workerModel`. For
example:

```json
{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-6-luna"
  }
}
```

Defaults are fallbacks. Each default resolves independently in this order:
operator file, environment, then explicit invocation flag. Thus environment
settings override the file, and `you run --provider` or `--model` override both.
Authored worker values take precedence over operator defaults. Agent and worker
selections also take precedence for their own execution.

Use the read-only MCP resource `you://operator/config/current` to inspect the
current file. It returns the raw JSON configuration, or `{}` when no file is
present. Use `you://operator/config/schema` for the configuration schema.
These resources are authoritative for the running server's current values and
supported fields. Do not expose credentials or other sensitive values when
reporting configuration to a customer.

## Discover provider names and models

Use MCP tool `you.provider.list_providers` or run `you providers list` (or `you providers list --json`) to inspect provider
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

Prefer MCP tool `you.operator_settings.add_acp_provider` when available. It
accepts `name`, `command`, optional `id`, and optional `transport` (defaults to
`stdio`), validates the candidate, and persists it at the canonical config
path and activates the provider in the current server. The result reports
`requiresRestart: false`; call `you.provider.list_providers` to verify the new
provider, then invoke it. The equivalent supported CLI is:

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

To select the custom provider as the default, call
`you.operator_settings.set_subagent_defaults` with `provider` set to its
canonical `name` (and optionally `model`). The tool validates and persists the
operator defaults at the canonical config path. To set it for one worker,
configure that worker's `modelProvider` to the same name. Choose a
model only when the provider catalog reports an exact supported model ID.
Custom ACP providers may not publish model metadata, so do not guess one.

The CLI is preferred to hand-editing the file. When direct editing is needed,
preserve existing keys and use this shape:

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
invocation's supported permission setting for permission behavior.

## Existing MCP capabilities

Use `you.provider.list_providers` to inspect the live catalog, and read
`you://operator/config/current` before changing defaults. Apply changes with
`you.operator_settings.set_subagent_defaults` or
`you.operator_settings.add_acp_provider`, then read the current configuration
and provider catalog again to verify. The MCP tool activates custom ACP
providers without a server restart. Do not claim that a change succeeded
unless the MCP tool or resource confirms it.

Long-running subagent invocations are synchronous. Do not assume a one-hour
invocation will complete: the MCP client, server transport, and provider may
each enforce a timeout. For long tasks, inspect the tool's timeout behavior
and report the configured limit. A timeout should be surfaced as an execution
failure rather than treated as a successful result.
