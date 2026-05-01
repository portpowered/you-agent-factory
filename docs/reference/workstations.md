# Workstations Reference

Use this page when you need the current workstation authoring contract:
topology fields, scheduling kinds, runtime `type`, and outcome routing.

## Current Contract

- Use `behavior` for scheduling behavior: `STANDARD`, `REPEATER`, or `CRON`.
- Use `type` for the runtime implementation: `MODEL_WORKSTATION` or
  `LOGICAL_MOVE`.
- Use `worker` for the bound worker name. Omit it only for logical routing
  workstations such as `LOGICAL_MOVE`.
- Route accepted results through `outputs`, ordinary partial-progress results
  through `onContinue`, rejected results through `onRejection`, and failed or
  timed-out results through `onFailure`.
- Use workstation-level `guards` only for `VISIT_COUNT` gating. Use a guarded
  `LOGICAL_MOVE` workstation when you need an explicit loop-breaker route.

## `behavior` Versus `type`

`behavior` answers "when should this workstation run?"

- `STANDARD` is the default fire-once step.
- `REPEATER` re-runs after continue results until the work is accepted or
  fails.
- `CRON` runs on a schedule in service mode.

`type` answers "what runtime implementation handles the step?"

- `MODEL_WORKSTATION` renders a prompt and dispatches to the bound worker.
- `LOGICAL_MOVE` moves tokens without invoking a worker.

Do not use `type` to express schedule semantics, and do not use `behavior` to
replace runtime implementation.

## Minimal Standard Step

```json
{
  "name": "review-story",
  "behavior": "STANDARD",
  "type": "MODEL_WORKSTATION",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

For a basic workflow step:

- `outputs` handles accepted completion.
- `onContinue` handles ordinary "keep iterating" routing when configured.
- `onRejection` handles true negative outcomes or review send-back.
- `onFailure` handles execution failure or timeout.

Use `REPEATER` when continue should keep the same workstation active instead of
routing to a different review state. Pair long-running review loops with a
guarded `LOGICAL_MOVE` loop breaker so repeated true rejection has an explicit
terminal path.

## When To Use Each Kind

- Use `STANDARD` for normal pipeline stages.
- Use `REPEATER` for iterative agent loops.
- Use `CRON` only when the step should submit scheduled time work in service
  mode; keep the schedule under `cron.schedule`.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Workstations and workers](../workstations.md)
- [Factory JSON and work configuration](../work.md)
- [Workstation guards and guarded loop breakers](../guides/workstation-guards-and-guarded-loop-breakers.md)
- [Parent-aware fan-in](../guides/parent-aware-fan-in.md)
