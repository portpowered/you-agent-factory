---
author: Agent Factory Team
last-modified: 2026-05-21
doc-id: agent-factory/reference/workers
---

# Workers

`you docs workers` stays available as the stable packaged worker quick
reference. Use [`docs/reference/workers.md`](../../../docs/reference/workers.md)
for the maintained worker guide.

Workers are the execution backends that workstations dispatch. A worker can be
model-backed, script-backed, or repository-hosted. Workstations reference
workers by `name`.

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
- `HOSTED_WORKER` binds a built-in provider integration. V1 currently supports
  `provider: LINEAR` for poller-style intake.
- Runner selection is separate from `modelProvider`. Use factory or
  workstation `runner` fields to choose `codex`, `gemini`, `kiro`,
  `cursor-cli`, or `opencode`.

## Common Fields

| Field | Applies to | Description |
|-------|------------|-------------|
| `name` | All | Stable worker identity referenced by `workstations[].worker`. |
| `type` | All | `MODEL_WORKER`, `SCRIPT_WORKER`, or `HOSTED_WORKER`. |
| `timeout` | All | Execution timeout such as `10m` or `1h`. |
| `resources` | All | Worker-scoped resource labels used by runtime integrations. |
| `model` | Model | Provider model name. |
| `modelProvider` | Model | Public model provider identifier such as `claude` or `codex`. |
| `executorProvider` | Model | Executor wrapper identifier such as `SCRIPT_WRAP`. |
| `stopToken` | Model | Accepted-completion marker when configured. |
| `skipPermissions` | Model | Provider-specific local permission shortcut. |
| `command` | Script | Executable name. |
| `args` | Script | Argument list. Values support template rendering. |
| `provider` | Hosted | Built-in provider identifier. V1 supports `LINEAR`. |
| `auth.secretRef` | Hosted | Secret reference for provider authentication. |
| `linear` | Hosted | Provider-specific Linear poller configuration. |

## Authoring Rules

- Use `modelProvider` and `executorProvider` as distinct fields.
- Use `runner` when the operator needs to choose the execution family; keep
  `modelProvider` for worker-local provider compatibility and diagnostics.
- Prefer split `workers/<name>/AGENTS.md` files for long model instructions.
- Keep inline worker runtime config only when portability or generated output
  matters more than hand-authored readability.
- For hosted pollers, keep auth on `auth.secretRef` and keep provider-specific
  poller fields on the worker, not on the workstation.

## Related

- `you docs config`
- `you docs workstations`
- `you docs templates`
