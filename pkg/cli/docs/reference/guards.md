# Guards

Use guards when a workstation should stay disabled until a condition is true,
when two inputs must match before dispatch, when parent work must wait for
children, or when a retry loop needs an explicit terminal route.

`you docs guards` is the canonical guide for guard types, attachment levels,
and guarded `LOGICAL_MOVE` loop breakers. See [Workstations](workstations.md)
for workstation kinds and route fields, and [Relationships](relationships.md)
for how `PARENT_CHILD` batch relations enable parent-aware input guards.

## Quick Choice

| Need | Attachment | Guard type |
|------|------------|------------|
| Stop a retry or review loop and move work to a failed or terminal state | Guarded `LOGICAL_MOVE` workstation | `VISIT_COUNT` on the loop breaker |
| Allow a workstation only after another workstation has visited the token enough times | `workstations[].guards[]` | `VISIT_COUNT` |
| Require grouped inputs to share the same resolved field value before dispatch | `workstations[].guards[]` | `MATCHES_FIELDS` |
| Join two normal inputs only when their authored work names match | `workstations[].inputs[].guards[]` | `SAME_NAME` |
| Wait for all spawned children to complete | `workstations[].inputs[].guards[]` | `ALL_CHILDREN_COMPLETE` |
| Fail a parent when any spawned child fails | `workstations[].inputs[].guards[]` | `ANY_CHILD_FAILED` |
| Pause inference dispatches after provider throttle failures | Top-level `guards[]` on `factory.json` | `INFERENCE_THROTTLE_GUARD` |
| Limit concurrent dispatches | `resources[]` and `workstations[].resources[]` | Not a guard |
| Limit one dispatch duration | `workstations[].limits.maxExecutionTime` | Not a guard |

## Attachment Levels

| Level | JSON path | Supported guard types |
|-------|-----------|------------------------|
| Factory | `guards[]` on `factory.json` | `INFERENCE_THROTTLE_GUARD` only |
| Workstation | `workstations[].guards[]` | `VISIT_COUNT`, `MATCHES_FIELDS` |
| Input | `workstations[].inputs[].guards[]` | `SAME_NAME`, `ALL_CHILDREN_COMPLETE`, `ANY_CHILD_FAILED` |

Workstation-level guards gate whether a workstation may fire. They do not create
a new source-to-target route. Per-input guards apply to one consumed input.
Parent-aware child guards belong on inputs, not on workstation-level `guards[]`.

## Workstation-Level `VISIT_COUNT`

Use a workstation-level `VISIT_COUNT` guard when an existing workstation should
stay disabled until the watched workstation has been visited enough times:

```json
{
  "name": "second-pass-review",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "guards": [
    {
      "type": "VISIT_COUNT",
      "workstation": "execute-story",
      "maxVisits": 2
    }
  ]
}
```

`second-pass-review` is enabled only when the token's visit count for
`execute-story` is greater than or equal to `2`. The threshold is inclusive:
`maxVisits` passes when visits are greater than or equal to the limit.

If the guard is false, the token stays in its current place until another
workstation can consume it.

## Workstation-Level `MATCHES_FIELDS`

Use `MATCHES_FIELDS` when the workstation should consume only candidate input
sets whose resolved selector values all match:

```json
{
  "name": "pair-same-flavor-assets",
  "worker": "matcher",
  "inputs": [
    { "workType": "asset", "state": "ready" },
    { "workType": "asset", "state": "ready" }
  ],
  "outputs": [{ "workType": "asset", "state": "matched" }],
  "guards": [
    {
      "type": "MATCHES_FIELDS",
      "matchConfig": { "inputKey": ".Name" }
    }
  ]
}
```

Nested tag selectors are supported:

```json
{
  "guards": [
    {
      "type": "MATCHES_FIELDS",
      "matchConfig": { "inputKey": ".Tags[\"_last_output\"]" }
    }
  ]
}
```

## Per-Input `SAME_NAME`

Use `SAME_NAME` when one workstation consumes two normal inputs and should fire
only when those tokens share the same authored work name. Attach the guard to
one input and set `matchInput` to the peer input's `workType` name:

```json
{
  "name": "join-plan-and-task",
  "worker": "planner-reviewer",
  "inputs": [
    { "workType": "planItem", "state": "ready" },
    {
      "workType": "taskItem",
      "state": "ready",
      "guards": [
        {
          "type": "SAME_NAME",
          "matchInput": "planItem"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "reviewItem", "state": "ready" }]
}
```

`matchInput: "planItem"` names the peer input on the same workstation. The
workstation stays disabled when the names differ or when either token lacks a
usable authored work name. Do not move same-name joins to workstation-level
`guards[]`.

## Per-Input `ALL_CHILDREN_COMPLETE`

Use `ALL_CHILDREN_COMPLETE` when a parent should finish only after the expected
child set reaches a completion state. The child input carries the guard:

```json
{
  "name": "complete-story",
  "worker": "story-merger",
  "inputs": [
    { "workType": "story", "state": "waiting" },
    {
      "workType": "task",
      "state": "complete",
      "guards": [
        {
          "type": "ALL_CHILDREN_COMPLETE",
          "parentInput": "story",
          "spawnedBy": "split-story"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "story", "state": "complete" }]
}
```

`parentInput` names the parent input on the same workstation. `spawnedBy`
names the workstation that created the children when the child count is known
at runtime. For submitted batches without a splitter, see
[Relationships](relationships.md) for when `PARENT_CHILD` lineage is enough.

## Per-Input `ANY_CHILD_FAILED`

Use `ANY_CHILD_FAILED` when one failed child should fail the parent:

```json
{
  "name": "fail-story-from-child",
  "worker": "story-failure-handler",
  "inputs": [
    { "workType": "story", "state": "waiting" },
    {
      "workType": "task",
      "state": "failed",
      "guards": [
        {
          "type": "ANY_CHILD_FAILED",
          "parentInput": "story"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "story", "state": "failed" }]
}
```

Submitted `PARENT_CHILD` batches can create the parent lineage these guards
match. See [Relationships](relationships.md) and [Batch Inputs](batch-inputs.md).

## Factory `INFERENCE_THROTTLE_GUARD`

Factory-level guards live in top-level `guards[]` on `factory.json`. Only
`INFERENCE_THROTTLE_GUARD` is supported there. It pauses inference dispatches
for a model lane after throttle failures until `refreshWindow` elapses:

```json
{
  "name": "throttled-factory",
  "guards": [
    {
      "type": "INFERENCE_THROTTLE_GUARD",
      "modelProvider": "CLAUDE",
      "model": "claude-sonnet-4-20250514",
      "refreshWindow": "15m"
    }
  ],
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" }
      ]
    }
  ],
  "workers": [{ "name": "executor" }],
  "workstations": [
    {
      "name": "execute-story",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }]
    }
  ]
}
```

Required fields:

| Field | Requirement |
|-------|-------------|
| `modelProvider` | Provider lane to throttle (for example `CLAUDE`) |
| `model` | Model name on that lane |
| `refreshWindow` | Positive duration such as `15m` or `3s` |

Do not attach `INFERENCE_THROTTLE_GUARD` to workstation-level or input-level
`guards[]`. Do not mix workstation guard fields such as `workstation` or
`maxVisits` on factory guards.

## Guarded `LOGICAL_MOVE` Loop Breakers

Use a guarded `LOGICAL_MOVE` workstation when the workflow should move
over-limit work from one state to another after the watched workstation reaches
its visit threshold:

```json
{
  "name": "review-loop-breaker",
  "type": "LOGICAL_MOVE",
  "inputs": [{ "workType": "story", "state": "init" }],
  "outputs": [{ "workType": "story", "state": "failed" }],
  "guards": [
    {
      "type": "VISIT_COUNT",
      "workstation": "review-story",
      "maxVisits": 3
    }
  ]
}
```

This loop breaker fires only when both are true:

- The token is waiting in `story:init` after `review-story` rejected it there.
- The token's visit count for `review-story` is greater than or equal to `3`.

| Behavior | Guard on an existing workstation | Guarded `LOGICAL_MOVE` |
|----------|----------------------------------|-------------------------|
| Checks visit count | Yes | Yes |
| Defines an explicit source state | Uses the workstation's input | Yes |
| Defines an explicit target failed/terminal state | Only if already wired that way | Yes |
| Best for | Delayed work or gating a later step | Loop breaking and over-limit routing |

Do not replace a loop breaker with only a guard on an unrelated worker
workstation. That changes the route and can leave the token in the wrong place.

## Guards Are Not Resources Or Runtime Limits

| Mechanism | Purpose |
|-----------|---------|
| Guards | Gate dispatch or route over-limit work through guarded `LOGICAL_MOVE` |
| `resources[]` / `workstations[].resources[]` | Limit concurrent dispatches |
| `limits.maxExecutionTime` | Cap duration of one dispatch |
| `limits.maxRetries` | Runtime retry or failure limit; not a visible workflow route |

`limits.maxRetries` is not a substitute for a guarded `LOGICAL_MOVE` route to a
named failed or terminal state.

## Common Mistakes

- Using workstation-level `guards[]` for `SAME_NAME` or parent-aware child
  guards — keep those on `workstations[].inputs[].guards[]`.
- Replacing a guarded `LOGICAL_MOVE` loop breaker with only a `VISIT_COUNT`
  guard on a normal worker workstation.
- Omitting `matchInput` on `SAME_NAME` guards or pointing it at the same input.
- Putting `INFERENCE_THROTTLE_GUARD` on a workstation or input.
- Expecting `DEPENDS_ON` batch relations to create parent lineage for
  `ALL_CHILDREN_COMPLETE` — use `PARENT_CHILD` instead (see
  [Relationships](relationships.md)).

## Related

- [Workstations](workstations.md)
- [Relationships](relationships.md)
- [Batch Inputs](batch-inputs.md)
- [Work configuration](work.md)
- [Templates](templates.md)
