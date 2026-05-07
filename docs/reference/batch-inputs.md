author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/guides/batch-inputs
---

# Batch Inputs

Use a `FACTORY_REQUEST_BATCH` when one submission should create multiple work
items together. A batch can describe independent work, `DEPENDS_ON`
prerequisites, or parent-child membership for parent-aware fan-in.

This guide covers the public batch input shape used by watched input files,
`agent-factory run --work`, and `PUT /work-requests/{request_id}`.

## Quick Start

Use the `BATCH` watched folder when the request contains mixed work types or
submitted parent-child relations:

```text
factory/inputs/BATCH/default/release-story-set.json
```

Write one canonical request body:

```json
{
  "request_id": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "work_type_name": "story-set",
      "state": "waiting",
      "payload": {
        "title": "April release story set"
      },
      "tags": {
        "project": "sample-service",
        "branch": "ralph/april-release"
      }
    },
    {
      "name": "story-auth",
      "work_type_name": "story",
      "payload": {
        "title": "Harden auth session handling"
      },
      "tags": {
        "project": "sample-service",
        "branch": "ralph/april-release"
      }
    },
    {
      "name": "story-billing",
      "work_type_name": "story",
      "payload": {
        "title": "Polish billing retry UX"
      },
      "tags": {
        "project": "sample-service",
        "branch": "ralph/april-release"
      }
    }
  ],
  "relations": [
    {
      "type": "PARENT_CHILD",
      "source_work_name": "story-auth",
      "target_work_name": "story-set"
    },
    {
      "type": "PARENT_CHILD",
      "source_work_name": "story-billing",
      "target_work_name": "story-set"
    }
  ]
}
```

Read each `PARENT_CHILD` relation as: the source work item is the child, and
the target work item is the parent. In the example above, `story-auth` and
`story-billing` become children of `story-set`.

The parent work item sets `"state": "waiting"` because parent-aware fan-in
usually consumes the parent from a non-initial waiting state. Use the exact
state name expected by the parent input in your `factory.json` topology.

Use the same request body for API submission:

```bash
curl -X PUT "http://localhost:7437/work-requests/release-story-set" \
  -H "Content-Type: application/json" \
  --data @factory/inputs/BATCH/default/release-story-set.json
```

The path `request_id` and body `request_id` must match.

## Where To Put Batch Files

Watched input files use this layout:

```text
factory/inputs/<work_type-or-BATCH>/<channel>/<filename>.json
```

Use these paths:

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<request_id>.json` | Manual mixed-work-type batches and canonical parent-child file input. |
| `factory/inputs/<work_type>/default/<request_id>.json` | Manual single-work-type batches. The watched folder can infer `work_type_name` when omitted. |
| `factory/inputs/<work_type>/<execution_id>/<request_id>.json` | Generated work tied to a parent execution. The channel name becomes the execution ID. |
| Any readable path passed to `agent-factory run --work <path>` | Startup work submitted before the run begins. |

Filename rules:

- The file must end in `.json` for the watcher to parse it as an explicit
  batch. Markdown files and non-batch JSON files are wrapped as one raw-payload
  work item instead.
- Prefer `<request_id>.json`, using lowercase words separated by hyphens.
- Keep the JSON `request_id` stable across retries. The filename does not have
  to match `request_id`, but matching them makes idempotency and logs easier to
  reason about.
- Avoid temporary suffixes such as `.tmp`, `.swp`, or `~`; the watcher ignores
  those files.

When a batch file is placed under `factory/inputs/BATCH/default/`, every work
item must set `work_type_name` explicitly because the folder does not imply one
shared work type.

## How Batches Work

The factory validates the full batch before it creates work tokens. Invalid
JSON, retired field aliases, duplicate work names, unknown relation names,
invalid work types, self-relations, and dependency cycles reject the whole
batch. No partial work is created.

After validation, the factory normalizes the batch:

1. Missing work IDs are generated as `batch-<request_id>-<work-name>`.
2. Missing work item trace IDs inherit the first trace ID in the batch, or a
   generated request trace.
3. Work item tags receive `_work_name` and `_work_type` values.
4. `state` places a work item directly into that work type's named state
   instead of its initial state.
5. `DEPENDS_ON` relations are attached to the blocked work token.
6. `PARENT_CHILD` relations are attached to the child work token and set the
   child's parent lineage for parent-aware guards.
7. Canonical history records a `WORK_REQUEST` event before related `WORK_INPUT`
   and `RELATIONSHIP_CHANGE_REQUEST` events.

Independent items in the same batch may dispatch in parallel, subject to the
workflow topology, resource limits, worker capacity, and normal scheduler
rules.

## Choose The Relation Type

Use the relation that matches the behavior you need:

| Relation type | Use it when | Source means | Target means |
|---------------|-------------|--------------|--------------|
| `DEPENDS_ON` | One sibling work item must wait for another sibling work item to reach a state. | The blocked work item. | The prerequisite work item. |
| `PARENT_CHILD` | A child work item should belong to a parent's child set for parent-aware fan-in. | The child work item. | The parent work item. |

`DEPENDS_ON` example:

```json
{
  "type": "DEPENDS_ON",
  "source_work_name": "publish",
  "target_work_name": "review",
  "required_state": "complete"
}
```

Read that as: `publish` waits for `review`.

`PARENT_CHILD` example:

```json
{
  "type": "PARENT_CHILD",
  "source_work_name": "story-auth",
  "target_work_name": "story-set"
}
```

Read that as: `story-auth` is a child of `story-set`.

Use `PARENT_CHILD` for submitted parent-aware batches. Use `DEPENDS_ON` for
ordinary prerequisite ordering between siblings. A single batch may include
both relation types when the workflow needs both parent membership and sibling
ordering.

## Minimum Fields For Parent-Child File Input

The smallest useful parent-child batch needs these fields:

| Field | Required | Why it matters |
|-------|----------|----------------|
| `request_id` | Yes | Stable idempotency key for the full submission. |
| `type` | Yes | Must be `FACTORY_REQUEST_BATCH`. |
| `works[].name` | Yes | Relations refer to work items by name. |
| `works[].work_type_name` | Yes for `inputs/BATCH` | Mixed-work-type parent-child batches cannot rely on folder inference. |
| `works[].state` on the parent | Usually | Place the parent directly into the waiting state consumed by the parent-aware fan-in workstation. |
| `relations[].type` | Yes | Use `PARENT_CHILD` for submitted parent-child membership. |
| `relations[].source_work_name` | Yes | Name of the child work item. |
| `relations[].target_work_name` | Yes | Name of the parent work item. |

Children usually omit `state` so they start in their work type's initial
state. Set a child `state` only when you intentionally need non-initial
placement.

## Request Fields

| Field | Required | Description |
|-------|----------|-------------|
| `request_id` | Yes | Stable client-provided request identifier. The API requires the path `request_id` and body `request_id` to match. Some lower-level submit paths can fill a missing ID, but public batch files should set it explicitly. |
| `type` | Yes | Must be `FACTORY_REQUEST_BATCH`. |
| `works` | Yes | Array of one or more work items. |
| `relations` | No | Array of relations between named work items in this batch. |

Do not use `work_type_id`. Public batch inputs use `work_type_name`; retired
`work_type_id` aliases are rejected at submit boundaries.

## Work Item Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Human-readable work name. Names must be unique within the batch because relations refer to work by name. |
| `work_type_name` | Usually | Configured work type from `factory.json`. Watched input files can infer this from `factory/inputs/<work_type>/...` when omitted, but `inputs/BATCH` requires it on every work item. |
| `state` | No | Starting state for this work item. Omit it to use the work type's initial state. Use it on a parent item when fan-in should start from a waiting state. |
| `work_id` | No | Stable unique work ID. Omit this unless an external system needs a specific ID. |
| `request_id` | No | Per-work request ID override. Omit this for normal batches so work items inherit the top-level `request_id`. |
| `trace_id` | No | Trace identifier. Omit this to let the batch share one trace. |
| `payload` | No | Opaque work payload. Objects, arrays, strings, numbers, booleans, and `null` are accepted. |
| `tags` | No | String key-value metadata available to prompt templates and parameterized workstation fields. |

Avoid setting tag names that begin with `_work_`. The factory writes
`_work_name` and `_work_type` during normalization.

## Relation Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Use `DEPENDS_ON` or `PARENT_CHILD`. |
| `source_work_name` | Yes | Name of the blocked work item for `DEPENDS_ON`, or the child work item for `PARENT_CHILD`. |
| `target_work_name` | Yes | Name of the prerequisite work item for `DEPENDS_ON`, or the parent work item for `PARENT_CHILD`. |
| `required_state` | Only for `DEPENDS_ON` | Target state required before the source can run. Defaults to `complete`. Ignore this field for `PARENT_CHILD`. |

Declare batch relations by name. Do not use `target_work_id` in submitted batch
relations; target work IDs are resolved during normalization and may appear in
events after submission.

## Validation Checklist

Before dropping a batch file into `factory/inputs/...`, confirm:

- The filename ends in `.json`.
- `type` is exactly `FACTORY_REQUEST_BATCH`.
- `request_id` is stable and unique for the intended submission.
- Every work item has a unique `name`.
- Every `inputs/BATCH` work item sets `work_type_name`.
- Parent work items that feed fan-in use the exact waiting `state` expected by
  the guarded parent input.
- Every `PARENT_CHILD.source_work_name` names a child.
- Every `PARENT_CHILD.target_work_name` names a parent.
- Every relation source and target matches a work item name.
- `required_state`, when used on `DEPENDS_ON`, names an actual state on the
  target work type.
- `DEPENDS_ON` relations do not create cycles.

## Related

- [Work](../reference/work.md)
- [Author workflows](../reference/authoring-workflows.md)
- [Parent-aware fan-in](../internal/development/parent-aware-fan-in.md)
- [Workstations](../reference/workstations-and-workers.md)
- [Prompt variables](../reference/prompt-variables.md)
