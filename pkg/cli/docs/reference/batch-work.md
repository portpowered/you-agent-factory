---
author: Agent Factory Team
last-modified: 2026-04-22
doc-id: agent-factory/reference/batch-work
---

# Batch Work

Use a `FACTORY_REQUEST_BATCH` when one submission should create multiple work
items together. A batch can describe independent work, `DEPENDS_ON`
prerequisites, and `PARENT_CHILD` relations for parent-aware fan-in.

## Example Request

```json
{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "workTypeName": "story-set",
      "state": "waiting",
      "payload": { "title": "April release story set" }
    },
    {
      "name": "story-auth",
      "workTypeName": "story",
      "payload": { "title": "Harden auth session handling" }
    }
  ],
  "relations": [
    {
      "type": "PARENT_CHILD",
      "sourceWorkName": "story-auth",
      "targetWorkName": "story-set"
    }
  ]
}
```

## Supported Paths

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<requestId>.json` | Mixed-work-type batches and canonical parent-child file input. |
| `factory/inputs/<work_type>/default/<requestId>.json` | Single-work-type watched batches. |
| Any readable JSON path passed to `agent-factory run --work <path>` | Startup batch submission before runtime start. |

## Request Fields

| Field | Description |
|-------|-------------|
| `requestId` | Stable idempotency key for the submission. |
| `type` | Must be `FACTORY_REQUEST_BATCH`. |
| `works` | One or more work items. |
| `relations` | Optional named relations between items in the same request. |

## Relation Types

| Type | Meaning |
|------|---------|
| `DEPENDS_ON` | The source work waits for the target work to reach a required state. |
| `PARENT_CHILD` | The source work becomes a child of the target work. |

Use `workTypeName` in public batch payloads. Do not use the retired
`work_type_id` alias.

## Related

- `agent-factory docs config`
- `agent-factory docs workstation`
- `agent-factory docs templates`
