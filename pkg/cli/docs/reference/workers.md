---
author: Agent Factory Team
last-modified: 2026-04-22
doc-id: agent-factory/reference/workers
---

# Workers

Workers are the execution backends that workstations dispatch. A worker can be
model-backed or script-backed. Workstations reference workers by `name`.

## Split Worker Example

`factory.json`:

```json
{
  "workers": [{ "name": "executor" }]
}
```

`workers/executor/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are the implementation worker. Follow the workstation instructions and keep
changes scoped to the current work item.
```

## Worker Types

- `MODEL_WORKER` renders prompts and dispatches through a supported model
  provider.
- `SCRIPT_WORKER` runs a local command with optional rendered arguments.

## Common Fields

| Field | Applies to | Description |
|-------|------------|-------------|
| `name` | All | Stable worker identity referenced by `workstations[].worker`. |
| `type` | All | `MODEL_WORKER` or `SCRIPT_WORKER`. |
| `timeout` | All | Execution timeout such as `10m` or `1h`. |
| `resources` | All | Worker-scoped resource labels used by runtime integrations. |
| `model` | Model | Provider model name. |
| `modelProvider` | Model | Public model provider identifier such as `claude` or `codex`. |
| `executorProvider` | Model | Executor wrapper identifier such as `SCRIPT_WRAP`. |
| `stopToken` | Model | Accepted-completion marker when configured. |
| `skipPermissions` | Model | Provider-specific local permission shortcut. |
| `command` | Script | Executable name. |
| `args` | Script | Argument list. Values support template rendering. |

## Authoring Rules

- Use `modelProvider` and `executorProvider` as distinct fields.
- Prefer split `workers/<name>/AGENTS.md` files for long model instructions.
- Keep inline worker runtime config only when portability or generated output
  matters more than hand-authored readability.

## Related

- `agent-factory docs config`
- `agent-factory docs workstation`
- `agent-factory docs templates`
