---
author: Agent Factory Team
last-modified: 2026-08-09
doc-id: agent-factory/work
---

# Submitted Work

Use this guide when accepting work into a running factory:
`POST /factory-sessions/{session_id}/work`, watched `inputs/`, and how submitted
payloads become tokens in the workflow. Use `~default` as `{session_id}` when
targeting the default compatibility session (the same label the CLI uses when
`--session` is omitted).

For `factory.json` topology—work types, states, workers, workstations,
resources, routing, and portability—see `you docs config`.

For watched batch files under `inputs/`, relation fields, and the
`FACTORY_REQUEST_BATCH` contract, see `you docs batch-inputs`.

For confirming a factory service is running and routing `--server` or
`--session` on submit and work commands, see `you docs sessions`.

This is the canonical customer-facing guide for submitted-work contracts.
Keep single-work API fields, batch cross-links, tag propagation, and
submission-oriented runtime flow here. Keep `factory.json` topology in
`you docs config`.

## When To Use This Guide

| Need | Use |
|------|-----|
| Define `factory.json`, work types, states, routing, or portability fields | `you docs config` |
| Confirm the factory is running, choose run modes, or route `--server` / `--session` | `you docs sessions` |
| Submit one work item with `POST /factory-sessions/{session_id}/work` or understand required API fields | This guide |
| Place batch request files under `inputs/`, define `FACTORY_REQUEST_BATCH`, or choose `DEPENDS_ON` versus `PARENT_CHILD` | `you docs batch-inputs` |
| Walk through a full factory setup with example files and commands | `you docs authoring-factories` |

## How Submitted Work Fits The Runtime

Submitted work enters the factory as a token in a work type's initial state.
The runtime validates the request against the topology declared in
`factory.json` (see `you docs config`), then places the token in the
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
`you docs config` and `you docs workstations`.

## Work State Names And Lifecycle Categories

Every configured Work state has two independent values:

- `name` is the customer-authored state name, such as `queued`, `in-review`,
  `shipped`, or `blocked`. This is the value used by submitted `state` fields
  and workstation input or output routes.
- `type` is the runtime lifecycle category. It is one of `INITIAL`,
  `PROCESSING`, `TERMINAL`, or `FAILED`. The category is not a state name and
  should not be substituted for one.

For example, a Work type can give meaningful customer names to all four
categories:

```json
{
  "name": "release-task",
  "states": [
    { "name": "queued", "type": "INITIAL" },
    { "name": "in-review", "type": "PROCESSING" },
    { "name": "shipped", "type": "TERMINAL" },
    { "name": "blocked", "type": "FAILED" }
  ]
}
```

| Lifecycle category | Runtime meaning | Observable completion meaning |
|--------------------|-----------------|-------------------------------|
| `INITIAL` | Entry or waiting category for newly admitted Work. | Non-terminal; a matching workstation may consume it. |
| `PROCESSING` | Work is still progressing through the authored workflow. | Non-terminal; it may be routed to another authored state. |
| `TERMINAL` | Work reached a successful completion state. | Terminal and successful; ordinary non-terminal processing stops. |
| `FAILED` | Work reached an unsuccessful completion state. | Terminal and failed; ordinary non-terminal processing stops, and it remains distinct from successful `TERMINAL` Work. |

### Starting placement

For a batch request, omitting `works[].state` places the Work in the Work
type's `INITIAL` state. To intentionally place it elsewhere, provide a
customer-authored state name that is declared on that Work type:

```json
{
  "name": "urgent-release",
  "workTypeName": "release-task",
  "state": "in-review",
  "payload": { "title": "Ship the urgent release" }
}
```

Here `in-review` is an explicit starting placement and `PROCESSING` is only
the category declared for that state. An unknown name, or a name belonging to
another Work type, is invalid. See `you docs batch-inputs` for the complete
batch envelope and validation behavior.

### Transitions come from the authored topology

The lifecycle categories do not impose a universal sequence such as
`INITIAL -> PROCESSING -> TERMINAL`. The Factory topology determines which
transitions exist: a workstation `inputs` entry consumes a particular
`{workType, state}` name, and its `outputs`, `onContinue`, `onRejection`, and
`onFailure` routes name the next state. A workflow can therefore skip a
processing state, loop back for another attempt, branch to a failed state, or
use several processing states before success. See `you docs config` for Work
type declarations and `you docs workstations` for workstation routes and
outcomes.

To inspect the current authored state name together with its lifecycle
category, use the Work observation surfaces described in [Verify after
submit](#verify-after-submit) and [Watch Work state transitions](#watch-work-state-transitions).
The observation category is the reliable way to distinguish successful
`TERMINAL` Work from failed `FAILED` Work when both have stopped ordinary
processing.

Submitted work payloads are not part of the `factory.json` topology contract.
Use `you docs batch-inputs` for the watched-file and API request
schema, validation rules, relation fields, and submitted `PARENT_CHILD`
examples.

## Single-Work API Submission

`POST /factory-sessions/{session_id}/work` accepts one submitted work item at a
time. Unlike watched-folder batch inputs, it does not infer or synthesize a
request name for accepted work. Send an explicit authored `name` on every
single-work submission. Replace `{session_id}` with a live session id from
`you session list` (often `~default` on single-session hosts).

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

Required fields for `POST /factory-sessions/{session_id}/work`:

- `name`
- `workTypeName`

`name` is required for single-work API submission and remains independently
required as `works[].name` for batch requests. Both
`POST /factory-sessions/{session_id}/work` (`SubmitWorkRequest`) and batch
`WorkRequest` payloads use the OpenAPI camelCase fields from
`api/openapi.yaml`, such as `workTypeName` and `traceId` on submitted work items.
See `you docs batch-inputs` for the full `FACTORY_REQUEST_BATCH` contract,
including `requestId`, relation fields, and optional `currentChainingTraceId`.

## Submission contract shapes

Two public request bodies cover work ingress. Both normalize into the same
runtime `FACTORY_REQUEST_BATCH` shape before dispatch.

| Shape | Route | When to use |
|-------|-------|-------------|
| `SubmitWorkRequest` | `POST /factory-sessions/{session_id}/work` | One-work convenience body for CLI `you submit`, the dashboard submit widget, and single-item HTTP callers |
| `WorkRequest` | `PUT /factory-sessions/{session_id}/work-requests/{request_id}` | Canonical idempotent batch body for `FACTORY_REQUEST_BATCH` upserts (`you submit batch`, watched `inputs/`, pollers) |

Each submitted work item carries input through one of three payload fields.
Use only the field that matches your client; do not combine them on the same
request.

| Field | Surfaces | Purpose |
|-------|----------|---------|
| `payload` | `SubmitWorkRequest`, batch `works[]` | Opaque JSON (or legacy string) work input for templates and worker dispatch |
| `content` | `SubmitWorkRequest`, batch `works[]` | Ordered canonical multimedia/model parts (`text`, `image`, `audio`, and related types) |
| `items` | `SubmitWorkRequest` only | Dashboard-authored structured submit items that reference staged files |

**Staged files:** `POST /factory-sessions/{session_id}/work/staged-files` uploads
dashboard-authored file bytes. The stage response returns a backend-owned `url`
and `stagedFileRef` that structured `items[]` entries must cite. Staging exists
only for dashboard-authored file payloads used by structured submit items—not for
direct `content[]` submissions that already carry their own `url`.

**Mutual exclusivity:** `items` cannot be combined with `content` or `payload` on
the same submit request. Explicit `content` and `payload` on the same work item
are also rejected when they conflict. Batch upserts that need ordered parts set
`works[].content` per work item instead of `items` (which is single-submit only).

## Multimedia content URLs

File-backed work content (images, audio, video, and binary parts) is addressed
by a canonical `url` on each `content[]` part. API clients can submit remote or
inline media without copying bytes onto the factory host except through the
optional stage API.

Supported schemes:

| Scheme | Typical use |
|--------|-------------|
| `file://` | Local assets already readable on the factory host |
| `https://` / `http://` | CDN or remote assets fetched at dispatch time |
| `data:` | Small inline payloads encoded in the URL |

The legacy `file` field on canonical content parts is deprecated. During
migration, ingest may normalize file-only parts to `url`, but new submissions
should send `url` directly. Validation rejects empty `url`, unsupported schemes,
and `url` plus `file` on the same part.

### Stage then submit (dashboard or API)

1. `POST /factory-sessions/{session_id}/work/staged-files` with `itemType`,
   `fileName`, `mediaType`, and `contentBase64`.
2. Use the returned `url` (and `stagedFileRef` when needed) on structured
   `POST /factory-sessions/{session_id}/work` items.

```json
{
  "name": "screenshot-review",
  "workTypeName": "task",
  "items": [
    { "type": "text", "text": "Review this screenshot." },
    {
      "type": "image",
      "url": "file:///var/factory/staged/review.png",
      "stagedFileRef": "staged-abc123",
      "fileName": "review.png",
      "mediaType": "image/png"
    }
  ]
}
```

### Direct URL submit examples

Local file reference:

```json
{
  "name": "local-image-review",
  "workTypeName": "task",
  "content": [
    { "type": "text", "text": "Inspect the attached image." },
    {
      "type": "image",
      "url": "file:///opt/assets/review.png",
      "contentType": "image/png"
    }
  ]
}
```

Remote HTTPS asset:

```json
{
  "type": "image",
  "url": "https://cdn.example.test/assets/review.png",
  "contentType": "image/png"
}
```

Inline `data:` URL:

```json
{
  "type": "image",
  "url": "data:image/png;base64,iVBORw0KGgo=",
  "contentType": "image/png"
}
```

At dispatch time the runtime materializes `url` values to readable local paths
for providers such as Codex (`-i` flags). Remote fetch failures surface as
`media url inaccessible` before the provider subprocess starts. Private,
link-local, and metadata targets are blocked by default (SSRF guard).

Event history and `WORK_REQUEST` payloads keep the canonical `url` values, not
materialized temporary paths.

## CLI `you submit` success and verify loop

`you submit` posts one work item to a running factory session via
`POST /factory-sessions/{session_id}/work` (CLI default session `~default` when
`--session` is omitted). On HTTP `201`, stdout is shaped for
operators and scripts; failures never print the success confirmation.

Use the same `--server` base URI and `--session` target as `you work list` and
`you work show` when verifying work in a non-default factory session. See
`you docs sessions` for session list, routing tables, and run-mode guidance.

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

## Watch Work state transitions

Use `you work watch` when a script needs to wait for Work transitions without
repeatedly calling `you work list`:

```text
you --server http://localhost:7437 work watch
you --server http://localhost:7437 work watch --session session-beta --follow
```

The command targets `~default` when `--session` is omitted. An explicit
`--session` selects one live Factory Session. Stdout is NDJSON: each complete
line is one `you.work.watch.v1` object for a canonical Work state transition.
Required fields are `schemaVersion`, `sessionId`, `eventId`, `sequence`,
`eventTime`, `workId`, `workTypeName`, `fromState`, `toState`, `source`, and
`terminal`; `triggerWorkId` and `reason` are omitted when they are absent.
Lines remain in strictly increasing canonical event sequence order, and other
Factory Events do not become output lines. Diagnostics stay on stderr.

Finite mode (the default) flushes the terminal transition and exits `0` after
every Work in the observed cohort is terminal or failed. `--follow` emits
terminal transitions but stays attached for later transitions until Ctrl-C or
parent-context cancellation. A transient stream disconnect resumes from the
last accepted event ID and sequence with bounded retries, suppressing replayed
lines. Unknown sessions, stale retention cursors, and exhausted retries fail
with a non-zero diagnostic. Watch does not persist a cursor or promise durable
history; it observes the live session's canonical event stream.

For `jq`, select terminal lines while preserving one JSON object per line:

```bash
you --server http://localhost:7437 work watch --session session-beta \
  | jq -c 'select(.terminal)'
```

The equivalent PowerShell pipeline is:

```powershell
you --server http://localhost:7437 work watch --session session-beta |
  ConvertFrom-Json |
  Where-Object terminal
```

Press Ctrl-C to stop a follow stream. The command closes its active stream and
does not write a partial JSON line.

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

Use `you docs templates` for the full template variable inventory and
quoting rules.

## Related

- `you docs config`
- `you docs batch-inputs`
- `you docs authoring-factories`
- `you docs templates`
- `you docs relationships`
- `you docs guards`
- `you docs workstations`
- `you docs workers`
