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
| `request_id` | yes | Stable request identifier for the whole submission |
| `type` | yes | `FACTORY_REQUEST_BATCH` |
| `works` | yes | One or more submitted work items |
| `relations` | no | Named links between work items in the same batch |

## Work Item Fields

| Field | Required | What to put there |
|-------|----------|-------------------|
| `name` | yes | Unique name within the batch |
| `work_type_name` | usually | Configured work type from `factory.json`; required for `inputs/BATCH` |
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

Read those directions literally: for `PARENT_CHILD`, `source_work_name` is the
child and `target_work_name` is the parent.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Batch inputs](batch-inputs.md)
- [Factory JSON and work configuration](work.md)
- [Parent-aware fan-in](../internal/development/parent-aware-fan-in.md)
