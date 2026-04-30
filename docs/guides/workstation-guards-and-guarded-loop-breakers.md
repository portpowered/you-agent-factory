---
author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/guides/workstation-guards-and-guarded-loop-breakers
---

# Workstation Guards And Guarded Loop Breakers

Use this guide when deciding whether a workflow needs a workstation-level
`visit_count` guard, a guarded `LOGICAL_MOVE` loop breaker, a same-name input
guard, parent-aware fan-in guards, resource limits, or runtime timeouts.

The public authoring pattern for visit-count loop breaking is a guarded
`LOGICAL_MOVE` workstation. It makes the source state, target state, watched
workstation, and visit threshold explicit in normal workstation topology.

## Quick Choice

| Need | Use |
|------|-----|
| Stop a retry or review loop and move work to a failed or terminal state | A guarded `LOGICAL_MOVE` workstation with a `visit_count` guard |
| Allow a workstation only after another workstation has visited the token enough times | `workstations[].guards[]` with `type: "visit_count"` |
| Require all grouped workstation inputs to resolve the same field value before dispatch | `workstations[].guards[]` with `type: "matches_fields"` and `matchConfig.inputKey` |
| Join two normal workstation inputs only when their authored work names match | `workstations[].inputs[].guards[]` with `type: "same_name"` and `matchInput` |
| Wait for spawned children to complete or fail | `workstations[].inputs[].guards[]` with the parent-aware child guard type |
| Limit concurrent dispatches | `resources[]` plus `workstations[].resources[]` |
| Limit one dispatch's runtime duration | workstation `limits.maxExecutionTime` |

## Same-Name Input Guard

Use a same-name input guard when one workstation consumes two normal inputs and
should fire only when those tokens have the same authored work name. This is an
input guard, not a workstation-level `guards[]` entry.

Attach the guard to one input and point `matchInput` at the peer input's
`workType` name:

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
          "type": "same_name",
          "matchInput": "planItem"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "reviewItem", "state": "ready" }]
}
```

In this workstation shape:

- `matchInput: "planItem"` names the peer input on the same workstation.
- The `same_name` guard compares the guarded `taskItem` token's authored work
  name to the bound `planItem` token's authored work name.
- The workstation stays disabled when the names differ or when either token
  does not have a usable authored work name.

Use same-name matching for normal multi-input joins. Do not move this rule to
workstation-level `guards[]`, and do not use it as a substitute for
parent-aware child guards.

## Guard On An Existing Workstation

Use a workstation-level guard when an existing workstation should stay disabled
until the watched workstation has been visited enough times:

```json
{
  "name": "second-pass-review",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "guards": [
    {
      "type": "visit_count",
      "workstation": "execute-story",
      "maxVisits": 2
    }
  ]
}
```

This guard means `second-pass-review` is enabled only when the token's visit
count for `execute-story` is greater than or equal to `2`.

Important behavior:

- A workstation-level guard gates one workstation. It does not create a new
  source-to-target route.
- If the guard is false, the token stays in its current place until another
  workstation can consume it.
- Workstation-level `guards[]` support `visit_count` and `matches_fields`.
- Parent-aware child guards stay on `workstations[].inputs[].guards[]`, not on
  workstation-level `guards[]`.

## Guarded Loop Breaker

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
      "type": "visit_count",
      "workstation": "review-story",
      "maxVisits": 3
    }
  ]
}
```

This loop breaker fires only when both conditions are true:

- The token is waiting in `story:init` after `review-story` rejected it there.
- The token's visit count for `review-story` is greater than or equal to `3`.

`visit_count` passes when the watched workstation's visits are greater than or
equal to `maxVisits`. The threshold is inclusive.

## Match Inputs By Resolved Field

Use a workstation-level matcher guard when the workstation should consume only
candidate input sets whose resolved selector values all match.

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
      "type": "matches_fields",
      "matchConfig": { "inputKey": ".Name" }
    }
  ]
}
```

The same matcher contract also supports nested tag selectors:

```json
{
  "guards": [
    {
      "type": "matches_fields",
      "matchConfig": { "inputKey": ".Tags[\"_last_output\"]" }
    }
  ]
}
```

## Guard Vs Guarded Route

| Behavior | Guard on an existing workstation | Guarded `LOGICAL_MOVE` workstation |
|----------|----------------------------------|------------------------------------|
| Checks visit count | Yes | Yes |
| Uses the watched workstation's visit history | Yes | Yes |
| Defines an explicit source state | Uses the workstation's existing input | Yes |
| Defines an explicit target state | Uses the workstation's existing output | Yes |
| Moves over-limit work to a failed or terminal state | Only if that workstation is already wired that way | Yes |
| Best for | Delayed work, escalation, or gating a later step | Loop breaking and explicit over-limit routing |

Do not replace a loop breaker with only a guard on an unrelated worker
workstation. That changes the route and can leave the token in the wrong place.

## Migration Note

Historical loop-breaker configs used retired top-level fields. New public
authoring should not use or copy those names. If you are translating an older
config, use the retired-note page for the field-by-field mapping:

- [Historical Note: Retired Loop-Breaker Guide](workstation-guards-and-exhaustion-limits.md)

## Retry Loop Example

This workflow executes a story, reviews it, and sends rejected work back to
the initial state. Two guarded loop breakers cap the execution and review
loops:

```json
{
  "workstations": [
    {
      "name": "execute-story",
      "kind": "repeater",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "executor-loop-breaker",
      "type": "LOGICAL_MOVE",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }],
      "guards": [
        {
          "type": "visit_count",
          "workstation": "execute-story",
          "maxVisits": 50
        }
      ]
    },
    {
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }],
      "guards": [
        {
          "type": "visit_count",
          "workstation": "review-story",
          "maxVisits": 3
        }
      ]
    }
  ]
}
```

## What Guarded Loop Breakers Are Not

Guarded loop breakers are not same-name joins. Use
`workstations[].inputs[].guards[]` with `type: "same_name"` and `matchInput`
when two normal inputs should match by authored work name.

Guarded loop breakers are not resource limits. Use `resources[]` and
`workstations[].resources[]` to limit concurrent dispatches.

Guarded loop breakers are not dispatch timeouts. Use workstation
`limits.maxExecutionTime` for the maximum duration of one execution.

Guarded loop breakers are not parent-aware child guards. Use
`workstations[].inputs[].guards[]` for `all_children_complete` and
`any_child_failed`.

`limits.maxRetries` is a runtime retry or failure limit. It is not a
substitute for a visible workflow route to a named failed or terminal state.

## Validation Checklist

- Use a guarded `LOGICAL_MOVE` workstation for retry and review loops that
  need a visible terminal or failed route.
- Keep same-name matching on `workstations[].inputs[].guards[]`, not on
  workstation-level `guards[]`.
- Every same-name guard names a different peer input with `matchInput`.
- Use `workstations[].guards[]` only for workstation-level `visit_count` or
  `matches_fields` gating.
- Keep parent-aware child guards on `workstations[].inputs[].guards[]`.
- Every `visit_count` workstation guard has `type`, `workstation`, and
  positive `maxVisits`.
- Every `matches_fields` workstation guard has `type` and non-empty
  `matchConfig.inputKey`.
- Every guarded loop breaker has one explicit source input and target output.
- Loop-breaker source and target states reference real work types and states.
- The watched `workstation` references an existing workstation.

## Related

- [Workstations](../workstations.md)
- [Work inputs](../work.md)
- [Prompt variables](../prompt-variables.md)
