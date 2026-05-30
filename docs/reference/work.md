---
author: Agent Factory Team
last-modified: 2026-05-30
doc-id: agent-factory/work
---

# Submitted Work

Use this guide when accepting work into a running factory: `POST /work`,
watched `inputs/`, and how submitted payloads become tokens in the workflow.

For `factory.json` topology—work types, states, workers, workstations,
resources, routing, and portability—see [Config](config.md) (`you docs config`).

For watched batch files under `inputs/`, relation fields, and the
`FACTORY_REQUEST_BATCH` contract, see [Batch Inputs](batch-inputs.md).

This is the canonical customer-facing guide for submitted-work contracts.
Keep single-work API fields, batch cross-links, tag propagation, and
submission-oriented runtime flow here. Keep `factory.json` topology in
[Config](config.md).

## When To Use This Guide

| Need | Use |
|------|-----|
| Define `factory.json`, work types, states, routing, or portability fields | [Config](config.md) |
| Submit one work item with `POST /work` or understand required API fields | This guide |
| Place batch request files under `inputs/`, define `FACTORY_REQUEST_BATCH`, or choose `DEPENDS_ON` versus `PARENT_CHILD` | [Batch Inputs](batch-inputs.md) |
| Walk through a full factory setup with example files and commands | [Author factories](authoring-factories.md) |

## How Submitted Work Fits The Runtime

Submitted work enters the factory as a token in a work type's initial state.
The runtime validates the request against the topology declared in
`factory.json` (see [Config](config.md)), then places the token in the
matching initial place (for example `task:init`).

From there, enabled workstations consume tokens from their input places,
dispatch to workers, and route outcomes based on worker results:

| Worker outcome | Routing field |
|----------------|---------------|
| Accepted | `outputs` |
| Continue | `onContinue` |
| Rejected | `onRejection` |
| Failed, timed out, or errored | `onFailure` |

Each accepted submission references a `workTypeName` that must exist in
`factory.json`. Workstation routing details are owned by
[Config](config.md) and [Workstations](workstations.md).

Submitted work payloads are not part of the `factory.json` topology contract.
Use [Batch Inputs](batch-inputs.md) for the watched-file and API request
schema, validation rules, relation fields, and submitted `PARENT_CHILD`
examples.

## Single-Work API Submission

`POST /work` accepts one submitted work item at a time. Unlike watched-folder
batch inputs, it does not infer or synthesize a request name for accepted work.
Send an explicit authored `name` on every single-work submission.

```json
{
  "name": "driver-incident-review",
  "workTypeName": "task",
  "traceId": "optional caller trace id",
  "payload": {},
  "tags": {
    "priority": "high"
  },
  "relations": []
}
```

Required fields for `POST /work`:

- `name`
- `workTypeName`

`name` is required for single-work API submission and remains independently
required as `works[].name` for batch requests. Both `POST /work` (`SubmitWorkRequest`)
and batch `WorkRequest` payloads use the OpenAPI camelCase fields from
`api/openapi.yaml`, such as `workTypeName` and `traceId` on submitted work items.
See [Batch Inputs](batch-inputs.md) for the full `FACTORY_REQUEST_BATCH` contract,
including `requestId`, relation fields, and optional `currentChainingTraceId`.

## Submit Success Response

On HTTP `201`, `POST /work` and session-scoped
`POST /factory-sessions/{sessionId}/work` return a `SubmitWorkResponse` body
with camelCase fields aligned to `api/openapi.yaml`:

| Field | Description |
|-------|-------------|
| `traceId` | Required trace identifier for the accepted submission. |
| `workId` | Normalized work identifier after submit acceptance (for example `batch-<requestId>-<name>` when callers omit an explicit id). |
| `name` | Authored work name returned for verification. |
| `workTypeName` | Configured work type for the accepted item. |

Example success body:

```json
{
  "traceId": "trace-abc",
  "workId": "batch-req-1-driver-incident-review",
  "name": "driver-incident-review",
  "workTypeName": "task"
}
```

Use these fields to confirm which work was accepted before listing or
re-submitting. See [Agents](agents.md) for the submit → wait → verify loop.

## CLI Submit Confirmation

`you submit` posts the same unary contract to the scoped submit path for the
effective session (`/work` for the default session alias, or
`/factory-sessions/{sessionId}/work` when `--session` is set).

On HTTP `201` with global `--json`, stdout is one confirmation object (not the
raw API envelope alone). Fields mirror the API identifiers plus CLI routing
metadata:

| Field | Source |
|-------|--------|
| `workId`, `name`, `workTypeName`, `traceId` | `SubmitWorkResponse` (CLI falls back to `--name` / `--work-type-name` when the API omits optional strings). |
| `sessionId` | Effective session label (`~default` for the default session). |
| `endpointPath` | Scoped submit path used for the HTTP request (for example `/factory-sessions/~default/work`). |

Human mode prints the same identifiers and one verify hint:
`you work show <workId>` when `workId` is non-empty, otherwise
`you work list --name <name>`.

Non-`201` responses exit non-zero with bounded error text on stderr; stdout
has no success confirmation. Transport failures (`factory not reachable at ...`)
remain distinct from API rejection messages.

## Tags And Prompt Templates

Tags declared on submitted work items are available after the batch request has
been accepted:

```
FACTORY_REQUEST_BATCH work tags
    ↓
Token.Tags (merged with _work_name, _workType)
    ↓
Prompt templates: {{ index (index .Inputs 0).Tags "branch" }}
    ↓
Parameterized fields: "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}"
```

Use [Templates](templates.md) for the full template variable inventory and
quoting rules.

## Related

- [Config](config.md)
- [Batch Inputs](batch-inputs.md)
- [Author factories](authoring-factories.md)
- [Templates](templates.md)
- [Relationships](relationships.md)
- [Guards](guards.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
