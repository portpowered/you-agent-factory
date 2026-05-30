# Resources Reference

Use this page when you need the current you-agent-factory resource contract for
bounded concurrency.

Use [Config](config.md) for the overall
`factory.json` topology and field ownership. This page owns the bounded
concurrency behavior of `resources` pools and resource requirements.

## Current Contract

- Declare shared pools at the top level of `factory.json` under `resources`.
- Use the canonical `{name, capacity}` shape for the top-level pool and for
  any matching requirement. Add uppercase `type` plus typed metadata when the
  resource describes a model cache, cloud quota, or invocation slot boundary.
- Put the scheduling-facing requirement on the workstation that should hold the
  capacity while it runs.
- Keep worker `resources` on the worker only when you need the worker-runtime
  contract to carry the same requirement metadata; do not use worker-only
  `resources` as the canonical explanation for workflow-step concurrency.
- Use canonical camelCase `resources`; older resource aliases are
  compatibility-only inputs.

## Typed Model Resources

Use typed resources when model-backed execution needs more than a generic
capacity pool:

- `MODEL` for local managed model assets and loaded-handle capacity.
- `PROVIDER_QUOTA` for provider-wide cloud quota or request budget.
- `INVOCATION_SLOT` for per-model or per-provider concurrency.

```json
{
  "resources": [
    {
      "name": "omnivoice-cache",
      "type": "MODEL",
      "capacity": 1,
      "model": "OMNIVOICE_Q4_K_M",
      "backend": "LLAMACPP",
      "loadPolicy": "ON_DEMAND"
    },
    {
      "name": "cloud-tts-quota",
      "type": "PROVIDER_QUOTA",
      "capacity": 8,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    },
    {
      "name": "cloud-tts-slot",
      "type": "INVOCATION_SLOT",
      "capacity": 2,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    }
  ]
}
```

Typed metadata rules:

- `MODEL` resources require `model`, `backend`, and `loadPolicy`.
- `PROVIDER_QUOTA` resources require `provider` and `model`.
- `INVOCATION_SLOT` resources should carry the provider or model identity that
  the scheduler is supposed to throttle.

For local models, the running service also enforces a process-level shared
capacity boundary keyed by canonical model metadata. Two different factories
that point at the same local model still share that cross-factory limit.

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
- [Config](config.md)
- [Submitted work](work.md)
- [Models and model operations](models.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Workstation guards and guarded loop breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
