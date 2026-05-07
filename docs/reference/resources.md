# Resources Reference

Use this page when you need the current Agent Factory resource contract for
bounded concurrency.

## Current Contract

- Declare shared pools at the top level of `factory.json` under `resources`.
- Use the canonical `{name, capacity}` shape for the top-level pool and for
  any matching requirement.
- Put the scheduling-facing requirement on the workstation that should hold the
  capacity while it runs.
- Keep worker `resources` on the worker only when you need the worker-runtime
  contract to carry the same requirement metadata; do not use worker-only
  `resources` as the canonical explanation for workflow-step concurrency.
- Use canonical camelCase `resources`; older resource aliases are
  compatibility-only inputs.

## Minimal Bounded-Concurrency Example

```json
{
  "resources": [
    { "name": "agent-slot", "capacity": 2 }
  ],
  "workers": [
    { "name": "executor", "type": "MODEL_WORKER" }
  ],
  "workstations": [
    {
      "name": "execute",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    }
  ]
}
```

Read that example as:

1. `resources[0]` declares a pool named `agent-slot` with total capacity `2`.
2. `workstations[0].resources[0]` asks that workstation to hold one slot while
   the dispatch is in flight.
3. Up to two matching dispatches can run at once before later dispatches wait
   for capacity to be released.

## Where Requirements Belong

| Location | Uses the shared shape | What to use it for |
|----------|------------------------|--------------------|
| `resources[]` | yes | Declare the pool name and total available capacity |
| `workstations[].resources[]` | yes | Consume capacity for a workflow step while that workstation runs |
| `workers[].resources[]` | yes | Keep worker-side resource requirement metadata in the worker contract when needed |

For a new factory author, the normal bounded-concurrency path is: declare the
pool at the top level, then reference that pool from the workstation that
should be throttled.

## Validation Rules

- Requirement names should match a declared top-level resource pool.
- Requirement `capacity` should be positive.
- Use the same pool name everywhere you expect one shared concurrency limit.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Factory JSON and work configuration](work.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Workstations and workers](workstations-and-workers.md)
- [Workstation guards and guarded loop breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
