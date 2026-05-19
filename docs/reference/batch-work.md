# Batch Work Reference

Use this page when you need the current `FACTORY_REQUEST_BATCH` authoring
surface for watched files, `agent-factory run --work`, or
`PUT /work-requests/{request_id}`.

## Current Contract

- Use `FACTORY_REQUEST_BATCH` as the canonical submit shape.
- Put mixed-work-type batches and submitted parent-child batches under
  `factory/inputs/BATCH/default/<request_id>.json`.
- Put single-work-type batches under
  `factory/inputs/<work_type>/default/<request_id>.json`.
- In `inputs/BATCH`, every work item must set `work_type_name`.
- Use canonical `state` and `work_type_name`; retired aliases such as
  `target_state` and `work_type_id` are rejected at submit boundaries.
- Submitted batch relations use `DEPENDS_ON` and `PARENT_CHILD`.
  Runtime-only relation types such as `SPAWNED_BY` are not authored in batch
  files.

## Where To Put Batch Files

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<request_id>.json` | Mixed work types or submitted parent-child batches |
| `factory/inputs/<work_type>/default/<request_id>.json` | Single-work-type batches |
| `factory/inputs/<work_type>/<execution_id>/<request_id>.json` | Generated or routed work tied to one execution |
| Any readable `.json` path passed to `agent-factory run --work` | Startup batch submission before the run begins |

Use a `.json` filename for explicit batch input. Markdown and non-batch JSON
files are wrapped as one raw-payload work item instead of being parsed as a
structured batch.

## Minimal Batch

```json
{
  "request_id": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-auth",
      "work_type_name": "story",
      "payload": { "title": "Harden auth session handling" }
    }
  ]
}
```

Place that file at
`factory/inputs/story/default/release-story-set.json` for a single-work-type
submission, or at `factory/inputs/BATCH/default/release-story-set.json` when
the batch mixes work types.

## Request Fields

| Field | Required | What to put there |
|-------|----------|-------------------|
| `requestId` | yes | Stable request identifier for the whole submission |
| `type` | yes | `FACTORY_REQUEST_BATCH` |
| `works` | yes | One or more submitted work items |
| `relations` | no | Named links between work items in the same batch |

## Work Item Fields

| Field | Required | What to put there |
|-------|----------|-------------------|
| `name` | yes | Unique name within the batch |
| `workTypeName` | usually | Configured work type from `factory.json`; required for `inputs/BATCH` |
| `state` | no | Explicit starting state; omit it to use the work type's initial state |
| `payload` | no | Raw work payload |
| `tags` | no | String metadata available to templates and parameterized fields |

## Relation Types

| Use this relation | When to use it | Source means | Target means |
|-------------------|----------------|--------------|--------------|
| `DEPENDS_ON` | One sibling work item must wait for another | The blocked work item | The prerequisite work item |
| `PARENT_CHILD` | A child work item should belong to a parent work item | The child work item | The parent work item |

Use `DEPENDS_ON` for prerequisite ordering between siblings:

```json
{
  "type": "DEPENDS_ON",
  "source_work_name": "publish",
  "target_work_name": "review",
  "required_state": "complete"
}
```

Use `PARENT_CHILD` when parent-aware fan-in or child membership needs explicit
parent lineage:

```json
{
  "type": "PARENT_CHILD",
  "source_work_name": "story-auth",
  "target_work_name": "story-set"
}
```

Read those directions literally: for `PARENT_CHILD`, `sourceWorkName` is the
child and `targetWorkName` is the parent.

## Reading Runtime Work Relations

`GET /work` and `infinite-you work list` return `results[].relations` on each
listed work item when that source work currently has outbound runtime
relationships. Read every relation from the listed work item outward:

- `sourceWorkName` is the listed work item that owns the `relations` entry.
- `targetWorkName` is the other work item this source points at.
- `targetWorkId`, when present, is the stable runtime ID for that target work.
- `requiredState`, when present, is the target state the source is waiting on.
  This is normally set on `DEPENDS_ON` relations and omitted for lineage-only
  relation types.

In enumeration output, the relation types mean:

| Relation type | Read it as |
|---------------|------------|
| `DEPENDS_ON` | This listed work item is blocked until the target work item reaches `requiredState` (or `complete` when no explicit state is shown). |
| `PARENT_CHILD` | This listed work item is the child and the target work item is its parent. |
| `SPAWNED_BY` | This listed work item was created or fanned out from the target work item. |

Example runtime relation entries:

```json
{
  "type": "DEPENDS_ON",
  "sourceWorkName": "review",
  "targetWorkName": "draft",
  "targetWorkId": "work-draft",
  "requiredState": "complete"
}
```

Read that as: the listed `review` work depends on `draft` completing.

```json
{
  "type": "PARENT_CHILD",
  "sourceWorkName": "story-auth",
  "targetWorkName": "story-set",
  "targetWorkId": "work-story-set"
}
```

Read that as: the listed `story-auth` work is a child of `story-set`.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Batch inputs](batch-inputs.md)
- [Factory JSON and work configuration](work.md)
- [Parent-aware fan-in](../internal/development/parent-aware-fan-in.md)
