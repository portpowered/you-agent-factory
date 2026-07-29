---
author: Agent Factory Team
last-modified: 2026-07-29
doc-id: agent-factory/guides/providers
---

# Providers and ACP Agents

The `executorProvider` on an agent worker selects the execution mechanism;
`ACP` selects Agent Client Protocol execution. The separate `modelProvider`
names the configured integration. ACP integrations let `you` start a compatible
agent over local stdio while Factory Sessions, work routing, event recording,
and final-result selection remain owned by the Factory runtime.

Use `you workers list` to discover all workers, `you workers acp` to manage
custom ACP integrations, and `you run` to prove that an
agent can complete real work. Factory validation checks configuration shape; it
does not launch an agent or prove that a custom provider is still installed.

## Install an ACP agent

Install and authenticate the agent before configuring a worker. `you` includes
a broad, data-backed stdio catalog. Representative entries are:

| Provider identity | Launch command |
|-------------------|----------------|
| `cursor-acp` | `cursor-agent acp` |
| `kiro-acp` | `kiro-cli-chat acp` |
| `opencode-acp` | `npx -y opencode-ai acp` |
| `codex-acp` | `npx -y @agentclientprotocol/codex-acp` |
| `claude-acp` | `npx -y @agentclientprotocol/claude-agent-acp` |
| `gemini-acp` | `gemini --acp` |

For Cursor, confirm that the command is installed and that the account is
authenticated:

```bash
cursor-agent --version
cursor-agent acp --help
```

List the built-in presets and any operator-added ACP integrations:

```bash
you workers list
```

`selectable` means the identity is present in the Providers catalog. The
definitive readiness check is a small `you run`, because the agent executable,
authentication, and negotiated ACP session are checked when execution starts.

## Add a custom ACP integration

Use a stable lowercase provider identity and pass the complete launch command
as one `--argument` value:

```bash
you workers acp add --name company-cursor --transport stdio --argument "cursor-agent acp"
you workers list
```

P0 supports stdio only. The command may include arguments; `you` parses it when
the Providers service is constructed. Adding `cursor-acp`, `kiro-acp`, or
`opencode-acp` overrides that preset's launch command. Deleting such an override
restores the built-in preset.

ACP integrations are operator settings in:

- macOS/Linux: `~/.you-agent-factory/config.json`
- Windows: `%USERPROFILE%\.you-agent-factory\config.json`

The persisted section has this shape:

```json
{
  "workers": {
    "acp": {
      "integrations": [
        {
          "id": "generated-settings-entry-id",
          "name": "company-cursor",
          "transport": "stdio",
          "command": "cursor-agent acp"
        }
      ]
    }
  }
}
```

Prefer the CLI over editing this file. The generated `id` identifies the
settings entry; workers select the integration by `name`. Timeout and permission
policy do not belong in this operator section. Use normal Factory Session and
worker limits for timeouts, and `skipPermissions` for the existing invocation
permission behavior.

## Use an ACP agent in a Factory

In a split Factory, select the provider in `workers/<name>/AGENTS.md`:

```yaml
---
type: AGENT_WORKER
executorProvider: ACP
modelProvider: cursor-acp
skipPermissions: true
---

You are a focused software engineer. Make the requested change and run focused
verification before reporting the result.
```

The workstation remains a normal `AGENT_RUN`; ACP is not a separate
workstation or worker type:

```yaml
---
type: AGENT_RUN
---

{{ (index .Inputs 0).Payload }}
```

Validate the Factory, then run a small end-to-end task and keep the response
stream visible until it reaches a terminal result:

```bash
you factory config validate ./factory
you run --factory ./factory/factory.json --provider cursor-acp --model auto --skip-permissions "Add one table-driven test and run the focused test command."
```

`--skip-permissions` is invocation-only. Omit it when the ACP agent should ask
through its supported permission flow.

For a portable JSON or YAML Factory, set the same identity on its worker:

```json
{
  "name": "executor",
  "type": "AGENT_WORKER",
  "executorProvider": "ACP",
  "modelProvider": "cursor-acp",
  "skipPermissions": true
}
```

## Use ACP with a packaged Factory

Packaged Factories materialize lazily under
`~/.you-agent-factory/factories` and remain editable. Inspect the catalog, ask
for the named Factory's generated help to materialize it without executing
work, set `executorProvider` on the agent worker, validate it, and run it by
name:

```bash
you factory list
you run --named @you/goal --help
you factory config validate ~/.you-agent-factory/factories/@you/goal
you run --named @you/goal --provider cursor-acp --model auto --skip-permissions "Add a simple unit test, run it, and finish the goal."
```

For `@you/goal`, set `"executorProvider": "ACP"` and
`"modelProvider": "cursor-acp"` on the `goal-executor` worker in its
materialized `factory.json`. On Windows, the same
Factory is under `%USERPROFILE%\.you-agent-factory\factories\@you\goal`.

## Use ACP from JavaScript

JavaScript workflows select the same Providers catalog identity on
`agent.run`:

```javascript
return (async function () {
  phase("test");
  return await agent.run({
    label: "cursor-test-author",
    executorProvider: "ACP",
    modelProvider: "cursor-acp",
    prompt: "Add one table-driven unit test and run the focused test command."
  });
})();
```

Run it from the project directory whose files the agent should inspect:

```bash
you run --factory ./test.workflow.js --skip-permissions
```

JavaScript receives the structured child result. Use `you session show`,
`you session dispatches`, and the printed Factory Session links when deeper
inspection is needed. See `you docs javascript-workflows` for the complete host
API and lifecycle contract.

## Remove a custom integration

Delete by provider name and confirm it no longer appears:

```bash
you workers acp delete --name company-cursor
you workers list
```

Factories that still name the deleted custom identity can pass offline Factory
validation because the identity is syntactically valid. Their next real run
fails provider selection. Update `modelProvider`, reinstall the integration,
or use a built-in preset.

## Troubleshooting

- `ACP command is unavailable` means the configured launch command could not be
  created or found. Run its executable directly and check `PATH`.
- `ACP authentication required` means the agent advertised authentication but
  could not open a session. Complete the agent's own login flow and retry.
- An unknown-provider or failed-state result after deleting a custom integration
  means a worker still references its old `modelProvider`.
- A permission cancellation means the agent requested an operation that was not
  approved. Retry with the intended permission flow or deliberately add
  `--skip-permissions` for that invocation.
- `you factory config validate` proves Factory structure only. A terminal
  `you run` proves command startup, ACP negotiation, agent execution, routing,
  and result projection together.

## Related

- `you docs workers` for worker and workstation field ownership
- `you docs run` for terminal output modes and run selection
- `you docs javascript-workflows` for JavaScript child execution
- `you docs sessions` for Factory Session and dispatch inspection
