---
author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/guides/parent-aware-fan-in
---

# Parent-Aware Fan-In

Parent-aware fan-in lets a parent work item wait for child work items that were
spawned from the same parent. Use it when a workflow splits one item into
children, processes the children independently, and then needs to finish or
fail the parent based on the child results.

This guide covers the guard authoring shape in `factory.json`. For the
canonical watched-file and API request body used to submit parent-child
batches, see [Batch Inputs](batch-inputs.md).

## When To Use It

Use parent-aware fan-in for workflows like:

- split a story into tasks, then complete the story after all tasks complete
- split a document into pages, then merge the document after all pages process
- fail a parent as soon as any spawned child reaches a failed state

Do not use parent-aware fan-in for ordinary batch dependencies between sibling
work items. Batch dependencies use `DEPENDS_ON` relations in a
`FACTORY_REQUEST_BATCH`. Parent-aware fan-in uses workstation input guards and
parent-child token metadata created when work is spawned by a workstation or
submitted through `PARENT_CHILD` batch relations.

Choose the submitted relation this way:

| If you need | Use | Why |
|-------------|-----|-----|
| Child membership under one parent for parent-aware guards such as `ANY_CHILD_FAILED` or `ALL_CHILDREN_COMPLETE` | `PARENT_CHILD` | It records parent lineage on the child token. |
| One sibling work item to wait for another sibling work item | `DEPENDS_ON` | It only blocks dispatch ordering; it does not create parent lineage. |

`DEPENDS_ON` and `PARENT_CHILD` can appear in the same submitted batch, but
they solve different problems. Use `PARENT_CHILD` for parent-aware routing and
`DEPENDS_ON` for prerequisite ordering between siblings.

## Mental Model

A fan-in workstation has at least two inputs:

- the parent input, such as `story:waiting`
- the child input to observe, such as `task:complete` or `task:failed`

The child input carries a guard. The guard tells the scheduler which parent
input it should match against:

```json
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
```

Read this as: for the bound `story` parent, observe matching `task:complete`
children that were spawned by `split-story`.

The parent token is consumed by the fan-in workstation. Matching child tokens
are observed, so completing or failing the parent does not remove child result
tokens from their places.

## How The Splitter Creates Children

The splitter is a normal workstation execution that produces child work at
runtime. For parent-aware fan-in, those child tokens must carry the parent
work ID and the splitter must produce the fanout count used by `spawnedBy`.

Today that parent-scoped fanout path is a runtime executor capability: the
splitter returns spawned work in its `WorkResult.SpawnedWork`, and each child
token has `ParentID` set to the parent input token's work ID. The runtime then
creates:

- one child token in the child work type's initial state
- one fanout-count token for the `spawnedBy` workstation
- normal parent output tokens, such as moving the parent from `story:init` to
  `story:waiting`

Conceptually, the splitter result looks like this:

```json
{
  "outcome": "ACCEPTED",
  "spawned_work": [
    {
      "work_type_id": "task",
      "work_id": "task-1",
      "parent_id": "story-123"
    },
    {
      "work_type_id": "task",
      "work_id": "task-2",
      "parent_id": "story-123"
    }
  ]
}
```

The exact shape above is the internal `WorkResult` shape, not a public input
file. Custom executors and runtime integrations can populate it directly. A
plain model worker response or script stdout is not automatically converted
into `SpawnedWork`.

## Can A File Or API Call Create The Children?

Yes. A public `FACTORY_REQUEST_BATCH` can create submitted parent-child work
for fan-in when the batch uses `PARENT_CHILD` relations and places the parent
in the waiting `state` consumed by the guard. See
[Batch Inputs](batch-inputs.md) for the canonical request shape.

The splitter runtime path is still useful when child count is discovered during
execution. That path creates the same parent-child token metadata plus the
fanout-count token used by `spawnedBy`.

Use these surfaces this way:

| Surface | Good fit | Not a fit |
|---------|----------|-----------|
| `factory/inputs/BATCH/default/*.json` | Submit mixed-work-type parent-child batches with `PARENT_CHILD`. | Relying on folder inference for mixed work types. |
| `PUT /work-requests/{request_id}` | Submit the same canonical `FACTORY_REQUEST_BATCH` body through the API. | Replacing the `factory.json` guard authoring described in this guide. |
| Splitter workstation runtime | Create parent-scoped child work when the child set is only known at execution time. | Replacing public batch submission when the full child set is already known up front. |

Submitted `PARENT_CHILD` batches are enough to create parent lineage for
parent-aware matching. That is the key requirement for routes such as
`ANY_CHILD_FAILED`.

`ALL_CHILDREN_COMPLETE` needs one more detail: the runtime must know how many
children count toward completion. Use `spawnedBy` when a splitter workstation
discovers children at runtime and emits the fanout-count token. For submitted
batches with no splitter, use `ALL_CHILDREN_COMPLETE` only when some other part
of the topology already makes the expected child set explicit; otherwise the
guard cannot tell "all known children are complete" from "more children have
not arrived yet."

## Submitted Batch Example At A Glance

Use the canonical request body from [Batch Inputs](batch-inputs.md#quick-start)
for the actual watched-file or API payload. In that example:

| Batch element | Example value | Why it matters |
|---------------|---------------|----------------|
| Parent work item | `story-set` with `work_type_name: "story-set"` and `state: "waiting"` | The parent starts in the waiting state consumed by fan-in. |
| Child work items | `story-auth` and `story-billing` with `work_type_name: "story"` | These are the children observed by the parent-aware guards. |
| Submitted relations | `PARENT_CHILD` from each child name to `story-set` | The relations create the parent lineage the guard matches. |

One matching guard route can then consume the parent and observe its related
children:

```json
{
  "name": "fail-story-set-from-child",
  "worker": "story-set-failure-handler",
  "inputs": [
    { "workType": "story-set", "state": "waiting" },
    {
      "workType": "story",
      "state": "failed",
      "guards": [
        {
          "type": "ANY_CHILD_FAILED",
          "parentInput": "story-set"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "story-set", "state": "failed" }]
}
```

Read that as: when a submitted child story under the waiting `story-set`
parent reaches `failed`, fail that parent.

## Complete Parent After All Children Complete

This pattern finishes the parent only after the splitter's expected child count
has reached the child complete state:

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
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onFailure": { "workType": "story", "state": "failed" }
}
```

Use `spawnedBy` when the child count is determined at runtime. The runtime
creates a fanout-count token for that spawning workstation and uses it to know
how many children must complete before fan-in can fire.

## Fail Parent When Any Child Fails

Use a separate fan-in route when one failed child should fail the parent:

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
          "parentInput": "story",
          "spawnedBy": "split-story"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "story", "state": "failed" }]
}
```

Use `ANY_CHILD_FAILED` on the failed child input. The workstation fires when at
least one failed child token matches the parent.

## Guard Fields

Parent-aware fan-in belongs on `workstations[].inputs[].guards[]`.

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | `ALL_CHILDREN_COMPLETE` or `ANY_CHILD_FAILED`. |
| `parentInput` | Yes | Work type name for another input on the same workstation. |
| `spawnedBy` | Recommended | Workstation that spawned the child work. Required when the runtime must track the exact dynamic fanout count. |

The `parentInput` value matches a work type name, not a state name. In this
example, `parentInput: "story"` points at the workstation's
`story:waiting` input.

## Dynamic Versus Static Fan-In

Use `spawnedBy` for normal dynamic fanout. It ties the fan-in guard to the
number of children produced by a specific spawning workstation.

Omitting `spawnedBy` creates a static parent-match guard. That form observes
matching child tokens already present in the guarded child state, but it does
not know how many children should eventually arrive. Avoid it for generated
child work unless another part of the topology makes the expected child set
explicit. Submitted `PARENT_CHILD` batches often pair well with
`ANY_CHILD_FAILED`; use extra care before documenting or relying on a static
`ALL_CHILDREN_COMPLETE` route.

## Troubleshooting Submitted Fan-In

| Symptom | Likely cause | What to change |
|---------|--------------|----------------|
| The guard never matches the parent-child batch you submitted. | `PARENT_CHILD` direction is reversed. | Keep `source_work_name` on the child and `target_work_name` on the parent. |
| The parent never enters the fan-in workstation. | The submitted parent is missing the waiting `state` expected by the parent input. | Set the parent `works[].state` to the exact state consumed by the workstation, such as `waiting`. |
| Validation passes but the guard still does not match the intended parent input. | `parentInput` names the wrong work type. | Set `parentInput` to the workstation input work type name for the parent, not to a work item name or state name. |
| Sibling ordering works, but parent-aware guards never see a parent-child relationship. | The batch used `DEPENDS_ON` where `PARENT_CHILD` was required. | Use `PARENT_CHILD` for parent membership and keep `DEPENDS_ON` only for prerequisite ordering between siblings. |
| An `ALL_CHILDREN_COMPLETE` route fires too early or cannot represent the whole child set. | The topology has no explicit fanout count. | Use a splitter plus `spawnedBy`, or add another topology element that makes the expected child set explicit before relying on `ALL_CHILDREN_COMPLETE`. |

## Full Shape

This condensed topology splits one story into tasks, processes tasks, completes
the story when all tasks complete, and fails the story when any task fails:

```json
{
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "waiting", "type": "PROCESSING" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    },
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "splitter" },
    { "name": "task-worker" },
    { "name": "story-merger" },
    { "name": "story-failure-handler" }
  ],
  "workstations": [
    {
      "name": "split-story",
      "worker": "splitter",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "waiting" }],
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "process-task",
      "worker": "task-worker",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    },
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
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onFailure": { "workType": "story", "state": "failed" }
    },
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
              "parentInput": "story",
              "spawnedBy": "split-story"
            }
          ]
        }
      ],
      "outputs": [{ "workType": "story", "state": "failed" }]
    }
  ]
}
```

The splitter worker must emit generated child work with the parent relationship
attached by the normal Agent Factory generated-work path. The fan-in guards
then match children whose parent ID is the parent story's work ID.

## Validation Checklist

- The guarded child input uses `guards`, not the retired workstation-level
  `join` field.
- `type` is `ALL_CHILDREN_COMPLETE` or `ANY_CHILD_FAILED`.
- `parentInput` names another input work type on the same workstation.
- `parentInput` does not name the guarded child input's own work type.
- `spawnedBy`, when set, names an existing workstation.
- Dynamic fanout uses `spawnedBy` so the runtime can track expected child
  count.
- Child fan-in guards are not placed in `workstations[].guards[]`; that field
  only supports workstation-level `VISIT_COUNT` guards.

## Related

- [Workstations And Workers](../workstations.md)
- [Factory JSON And Work Configuration](../work.md)
- [Workstation Guards And Guarded Loop Breakers](workstation-guards-and-guarded-loop-breakers.md)
- [Batch Inputs](batch-inputs.md)
