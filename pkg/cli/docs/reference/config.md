---
author: Agent Factory Team
last-modified: 2026-04-22
doc-id: agent-factory/reference/config
---

# Config

Use `factory.json` as the canonical topology file for an Agent Factory run.
It declares the work types, states, workers, workstations, resources, and
routes that the runtime turns into a Petri-net execution graph.

## Minimal Shape

The smallest useful config has one work type, one worker, and one workstation:

```json
{
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [{ "name": "executor" }],
  "workstations": [
    {
      "name": "process-task",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

## Split Layout

Keep prompt-heavy runtime details beside the config when you want a readable
working tree:

```text
factory/
  factory.json
  workers/executor/AGENTS.md
  workstations/process-task/AGENTS.md
  inputs/task/default/
```

`factory.json` still owns the topology. Split `AGENTS.md` files own worker and
workstation runtime content such as system prompts, prompt templates, timeout
limits, and executor settings.

## Core Fields

| Field | Description |
|-------|-------------|
| `project` | Optional factory-wide project name used when submitted work does not provide one. |
| `workTypes` | Declares work categories and lifecycle states. |
| `resources` | Declares shared concurrency pools. |
| `workers` | Declares worker identities and optional inline worker runtime config. |
| `workstations` | Declares dispatch steps, routing, and optional inline runtime fields. |

## Authoring Rules

- Use camelCase public fields such as `workTypes`, `modelProvider`,
  `executorProvider`, `stopWords`, and `maxExecutionTime`.
- Use `behavior` for workstation scheduling behavior and `type` for runtime
  implementation details.
- Keep guarded `LOGICAL_MOVE` workstations explicit instead of relying on
  retired top-level loop-breaking fields.
- Prefer split `AGENTS.md` files for long prompts and inline runtime fields for
  portable or recorded single-file configs.

## Related

- `agent-factory docs workstation`
- `agent-factory docs workers`
- `agent-factory docs resources`
- `agent-factory docs batch-work`
- `agent-factory docs templates`
