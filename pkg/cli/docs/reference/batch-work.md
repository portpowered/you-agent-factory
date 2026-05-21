---
author: Agent Factory Team
last-modified: 2026-05-20
doc-id: agent-factory/reference/batch-work
---

# Batch Work

`you docs batch-work` stays available as the stable packaged quick-reference
topic for explicit batch submission. Use
[`docs/reference/batch-inputs.md`](../../../docs/reference/batch-inputs.md) as
the canonical customer guide for the complete `FACTORY_REQUEST_BATCH` contract.

## Canonical Guide

- Canonical public guide:
  `docs/reference/batch-inputs.md`
- Use that guide for watched-file placement, full request fields, validation
  rules, and relation semantics.
- Keep this packaged topic for quick terminal help and stable topic-name
  compatibility.

## Current Contract Summary

- Use `FACTORY_REQUEST_BATCH` when one submission should create multiple work
  items together.
- Put mixed-work-type batches and submitted parent-child batches under
  `factory/inputs/BATCH/default/<request_id>.json`.
- Put single-work-type batches under
  `factory/inputs/<work_type>/default/<request_id>.json`.
- In `inputs/BATCH`, every work item must set `work_type_name`.
- Submitted batch relations use `DEPENDS_ON` and `PARENT_CHILD`.

## Supported Paths

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<request_id>.json` | Mixed-work-type batches and canonical parent-child file input. |
| `factory/inputs/<work_type>/default/<request_id>.json` | Single-work-type watched batches. |
| Any readable `.json` path passed to `you run --work <path>` | Startup batch submission before runtime start. |

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

## Relation Types

| Type | Meaning |
|------|---------|
| `DEPENDS_ON` | The source work waits for the target work to reach a required state. |
| `PARENT_CHILD` | The source work becomes a child of the target work. |

Use `DEPENDS_ON` for prerequisite ordering between siblings and `PARENT_CHILD`
for explicit parent-aware lineage.

## Verification Pointers

- Watched-file and API request shape:
  `docs/reference/batch-inputs.md`
- Startup submission path:
  `you run --work <path>`
- API submission path:
  `PUT /work-requests/{request_id}`

## Related

- `you docs config`
- `you docs workstation`
- `you docs templates`
