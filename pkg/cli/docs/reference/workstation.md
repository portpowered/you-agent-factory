---
author: Agent Factory Team
last-modified: 2026-04-22
doc-id: agent-factory/reference/workstation
---

# Workstation

Workstations are the dispatch steps in `factory.json`. A workstation consumes
input places, optionally dispatches to a worker, and routes the result to its
configured output, continue, rejection, or failure place.

## Minimal Workstation

```json
{
  "name": "review-story",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

## Topology Fields

| Field | Description |
|-------|-------------|
| `name` | Stable workstation and transition name. |
| `kind` | Scheduling kind: `standard`, `repeater`, or `cron`. |
| `worker` | Worker name to dispatch when the workstation executes. |
| `inputs` | Places that must be present before the workstation can fire. |
| `outputs` | Places produced on accepted completion. |
| `onContinue` | Place produced on ordinary partial-progress completion. |
| `onRejection` | Place produced on rejected completion. |
| `onFailure` | Place produced on failure or timeout. |
| `resources` | Resource capacity held while the dispatch is in flight. |
| `guards` | Workstation-level `visit_count` guards. |
| `cron` | Schedule configuration for `kind: "cron"`. |

## Runtime Fields

These can live inline in `factory.json` or in
`workstations/<name>/AGENTS.md`:

| Field | Description |
|-------|-------------|
| `type` | Runtime implementation, typically `MODEL_WORKSTATION` or `LOGICAL_MOVE`. |
| `promptFile` | Prompt template file relative to the workstation directory. |
| `promptTemplate` | Inline prompt template string. |
| `limits.maxExecutionTime` | Per-dispatch timeout. |
| `limits.maxRetries` | Retry budget before the circuit breaker treats the work as failed. |
| `stopWords` | Ordered markers used for accept-or-fail output handling. |
| `workingDirectory` | Rendered execution working directory. |
| `worktree` | Rendered worktree path passed to supported executors. |
| `env` | Rendered environment variables passed into execution. |

## Scheduling Kinds

- `standard` fires once when its inputs are ready.
- `repeater` fires again after continue results and is the normal fit for
  iterative agent loops that keep rejection reserved for true send-back or
  negative outcomes.
- `cron` submits internal time work on a schedule while the runtime stays in
  service mode.

Use a guarded `LOGICAL_MOVE` workstation to cap repeater or review loops.

## Related

- `agent-factory docs config`
- `agent-factory docs workers`
- `agent-factory docs resources`
- `agent-factory docs templates`
