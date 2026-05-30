---
author: Agent Factory Team
last-modified: 2026-05-30
doc-id: agent-factory/work
---

# Submitted Work

`you docs work` owns submitted-work contracts: how work enters a factory after
you author a request, not how `factory.json` declares topology. Use
`you docs config` for work types, states, workers, workstations, routing,
resources, and portability fields.

Use this guide when submitting work through the API or watched `inputs/`
folders. For the JSON file you drop into `inputs/<workType>/...`, see
[Batch Inputs](batch-inputs.md). For `factory.json` field tables and routing
topology, see `you docs config`.

## When To Use This Guide

| Need | Use |
|------|-----|
| Submit via `POST /work`, map tags into templates, or trace token flow after acceptance | This guide (`you docs work`) |
| Define `factory.json`, work types, states, routing, or portability fields | `you docs config` |
| Place batch request files, define `FACTORY_REQUEST_BATCH`, or choose `DEPENDS_ON` versus `PARENT_CHILD` | `you docs batch-inputs` |
| Walk through a full setup sequence with example files and commands | `you docs authoring-factories` |

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

## Tags And Templates

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

Single-work `POST /work` submissions follow the same tag merge once the factory
accepts the item and materializes a token. Use `you docs templates` for the full
template variable inventory and quoting rules.

## How Submitted Work Moves

After the factory accepts a submission, work enters as a token in the target
work type's initial state. Workstations consume and produce those tokens
according to the topology declared in `factory.json` (see `you docs config`).

From a submission perspective:

1. Validate the request shape (`POST /work` or `FACTORY_REQUEST_BATCH` under
   `inputs/`).
2. The runtime creates tokens in the declared initial states for each accepted
   work item.
3. Enabled workstations dispatch to workers and route tokens using the
   outcome fields configured in `factory.json`.

| Worker outcome | Routing field |
|----------------|---------------|
| Accepted | `outputs` |
| Continue | `onContinue` |
| Rejected | `onRejection` |
| Failed, timed out, or errored | `onFailure` |

Each accepted item's `workTypeName` must match a `workTypes[].name` in
`factory.json`. The factory routes tokens to places named `<workType>:<state>`,
such as `task:init`, based on workstation IO declared in config.

## Submission Checklist

- Every submitted item includes a unique `name` and a valid `workTypeName`.
- Batch files use `type: "FACTORY_REQUEST_BATCH"` and stable `requestId` values.
- Relation endpoints name work items that exist in the same batch request.
- Tags intended for templates or parameterized fields are set on the submitted
  work item before acceptance.
- Topology changes (new states, routes, or work types) belong in `factory.json`
  and are documented under `you docs config`, not in submission payloads.

## Related

- `you docs config`
- [Batch Inputs](batch-inputs.md)
- `you docs relationships`
- `you docs templates`
- `you docs authoring-factories`
- `you docs workstations`
- `you docs workers`
