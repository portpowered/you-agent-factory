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

## CLI `you submit` success and verify loop

`you submit` posts one work item to a running factory session (`POST /work` or
`POST /factory-sessions/{sessionId}/work`). On HTTP `201`, stdout is shaped for
operators and scripts; failures never print the success confirmation.

Use the same `--server` base URI and `--session` target as `you work list` and
`you work show` when verifying work in a non-default factory session.

### Human success output

Human-mode stdout (default) includes accepted work metadata and a one-line verify
hint. It does **not** echo the submitted payload or full HTTP response body.

| Line | Meaning |
|------|---------|
| `Submitted: <name> (<workTypeName>)` | Accepted work name and type |
| `traceId: <traceId>` | Trace id returned by the API |
| `workId: <workId>` | Present only when the API returns `workId` |
| `Verify: you work show <work-id>` | Preferred next step when `workId` is present |
| `workId was not returned; verify with:` + `you work list --name <name>` | Fallback when `workId` is absent |

Example with `workId`:

```text
Submitted: driver-incident-review (task)
traceId: caller-trace-1
workId: batch-req-1-driver-incident-review
Verify: you work show batch-req-1-driver-incident-review
```

Example without `workId`:

```text
Submitted: driver-incident-review (task)
traceId: caller-trace-2
workId was not returned; verify with:
you work list --name driver-incident-review
```

Verbose request and response diagnostics (`--verbose`, debug flags) stay on
**stderr** only via the CLI diagnostics channel.

### JSON success output (`you --json submit`)

Global `--json` writes **one** JSON object to stdout and exits `0`. Stdout
contains only that object (no extra prose).

| Key | Type | Meaning |
|-----|------|---------|
| `workId` | string or `null` | Stable work id when returned; JSON `null` when absent (key always present) |
| `name` | string | Accepted work name (API value, else the submitted `--name`) |
| `workTypeName` | string | Accepted work type (API value, else `--work-type-name`) |
| `traceId` | string | Trace id from the API |
| `sessionId` | string | Session id from the API, else the CLI session label (`~default` when omitted) |
| `endpointPath` | string | Scoped path used for the submit request (for example `/factory-sessions/~default/work` or `/factory-sessions/<session>/work`) |

Example (default session):

```json
{
  "workId": "batch-req-1-driver-incident-review",
  "name": "driver-incident-review",
  "workTypeName": "task",
  "traceId": "caller-trace-1",
  "sessionId": "~default",
  "endpointPath": "/factory-sessions/~default/work"
}
```

Example when `workId` is absent (`workId` is JSON `null`):

```json
{
  "workId": null,
  "name": "driver-incident-review",
  "workTypeName": "task",
  "traceId": "caller-trace-2",
  "sessionId": "~default",
  "endpointPath": "/factory-sessions/~default/work"
}
```

### Submit failures

Failures return a non-zero exit code and **no** human success lines or JSON
success object on stdout.

| Situation | Message shape |
|-----------|----------------|
| Factory unreachable (connection refused, timeout, DNS) | `factory not reachable at <url>` |
| HTTP status not `201` | `submission failed (<status>): <message>` when `ErrorResponse.message` is present; otherwise a bounded raw-body preview (200 bytes) |
| Error JSON includes `workId` | Same as above with a stable `workId=<id>` suffix |

### Verify after submit

After a successful submit, confirm the work item was accepted before submitting
again:

1. **When `workId` is present** (human hint or JSON `workId` string): run
   `you work show <work-id>` to inspect that single work item.
2. **When `workId` is absent** (human fallback line): run
   `you work list --name <name>` and, when several items share a name, add
   `--work-type-name <type>` to narrow the listing to the submitted work type.

`you work list` also supports `--state-name`, `--state-type`, `--sort-by`,
`--max-results`, and `--session` for broader inspection. Human-mode list output
is tabular (`WORK ID`, `NAME`, `STATE NAME`, `STATE TYPE`, `RELATIONS`); use
`you --json work list` when scripts need the API-shaped `ListWorkResponse`.

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
