---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/guides/batch-inputs
---

# Batch Inputs

Use a `FACTORY_REQUEST_BATCH` when one submission should create multiple work
items together. A batch can describe independent work, `DEPENDS_ON`
prerequisites, or parent-child membership for parent-aware fan-in.

`you docs batch-work` is a compatibility alias for this guide; both commands
print identical markdown.

`you docs relationships` is the canonical guide for `DEPENDS_ON`,
`PARENT_CHILD`, and parent-aware guard linkage. `you docs guards` covers
parent-aware input guards such as `ALL_CHILDREN_COMPLETE`.

This guide covers the public batch input shape used by watched input files,
`you run --work`, `you submit batch`, and
`PUT /factory-sessions/{session_id}/work-requests/{request_id}`.

## Batch ingress comparison

Use the same canonical `FACTORY_REQUEST_BATCH` document for every path below.
The factory validates the full batch before creating work; invalid batches reject
the whole submission with no partial work.

| Ingress | When to use |
|---------|-------------|
| `you submit` | Submit one work item to a **running** factory through the CLI (unary submit). |
| `you submit batch` | Upsert a batch JSON document to a **running** factory session without restarting it. |
| `you run --work <path>` | Submit a batch file as part of **startup** before or while starting a local factory run. |
| `factory/inputs/BATCH/default/<request_id>.json` | Steady-state **watched-folder** ingress while the factory is already running. |
| `PUT /factory-sessions/{session_id}/work-requests/{request_id}` | HTTP upsert with the same JSON body (`~default` when targeting the default session). |

For single-work CLI or dashboard submission, see `you docs work`. For
relation semantics, see `you docs relationships`.

`WorkRequest` is the canonical batch body for
`PUT /factory-sessions/{session_id}/work-requests/{request_id}`. Each
`works[]` entry accepts the same input fields as a single submit—`payload` for
opaque JSON, or `content` for ordered canonical parts. Structured dashboard
`items` and staged-file staging apply only to
`POST /factory-sessions/{session_id}/work` (`SubmitWorkRequest`); batch callers
use `works[].content` instead. See `you docs work` for the full submission-shape
comparison and mutual-exclusivity rules.

## Quick reference

- Use `FACTORY_REQUEST_BATCH` when one submission should create multiple work
  items together.
- Put mixed-work-type batches and submitted parent-child batches under
  `factory/inputs/BATCH/default/<request_id>.json`.
- Put single-work-type batches under
  `factory/inputs/<work_type>/default/<request_id>.json`.
- In `inputs/BATCH`, every work item must set `workTypeName`.
- Submitted batch relations use `DEPENDS_ON` and `PARENT_CHILD`.

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<request_id>.json` | Mixed-work-type batches and canonical parent-child file input. |
| `factory/inputs/<work_type>/default/<request_id>.json` | Single-work-type watched batches. |
| Any readable `.json` path passed to `you run --work <path>` | Startup batch submission before runtime start. |
| `you submit batch <path>` (or `--file`, pipe, inline JSON) | Upsert a batch to a running factory session via CLI. |

Minimal batch:

```json
{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-auth",
      "workTypeName": "story",
      "payload": { "title": "Harden auth session handling" }
    }
  ]
}
```

| Relation type | Meaning |
|---------------|---------|
| `DEPENDS_ON` | The source work waits for the target work to reach a required state. |
| `PARENT_CHILD` | The source work becomes a child of the target work. |

Use `DEPENDS_ON` for prerequisite ordering between siblings and `PARENT_CHILD`
for explicit parent-aware lineage. See [Choose The Relation Type](#choose-the-relation-type)
for examples and field tables.

## One batch, one name namespace

`works[].name` is the authored name of a Work item in this request. Names must
be unique across the entire `works[]` array, including when the items have
different `workTypeName` values. The factory uses those names to resolve
`relations[]`; it does not use the Work ID, a work type, or a name from another
submission as an implicit relation endpoint.

Every `sourceWorkName` and `targetWorkName` must match a `works[].name` in the
same submitted `FACTORY_REQUEST_BATCH`. A relation cannot point at an existing
Work from an earlier batch, a Work in a different batch, or an item that is not
also included in this request. If the batch needs that relationship, submit the
related Work items together and declare the relation by their authored names.

This name scope is separate from request idempotency: reusing a `requestId`
reconciles the same submission, while changing the `requestId` starts a new
batch with a new name namespace.

## Before you submit

Read the checked-in factory topology before authoring batch files:

- `factory.json` — work types, states, and routing context for valid
  `workTypeName` and `state` values.
- `factory/docs/overview.md` when present — instance walkthrough, pipeline, and
  read-before-submit guidance for this factory.
- `factory/docs/README.md` when `factory/docs/overview.md` is absent — factory-local
  orientation before guessing `workTypeName` or input layout.

Do not infer work types from unrelated examples when this factory defines its
own topology.

## Quick Start

Use the `BATCH` watched folder when the request contains mixed work types or
submitted parent-child relations:

```text
factory/inputs/BATCH/default/release-story-set.json
```

Write one canonical request body:

This is a valid mixed-work-type batch: every item names its own public
`workTypeName`, and both relation endpoints are names of items in this same
`works[]` array.

```json
{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "workTypeName": "story-set",
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
      "workTypeName": "story",
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
      "workTypeName": "story",
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
      "sourceWorkName": "story-auth",
      "targetWorkName": "story-set"
    },
    {
      "type": "PARENT_CHILD",
      "sourceWorkName": "story-billing",
      "targetWorkName": "story-set"
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
curl -X PUT "http://localhost:7437/factory-sessions/~default/work-requests/release-story-set" \
  -H "Content-Type: application/json" \
  --data @factory/inputs/BATCH/default/release-story-set.json
```

The path `{request_id}` and body `requestId` must match.

## CLI batch submit (`you submit batch`)

When a factory is already running, upsert the same JSON body with
`you submit batch` instead of writing to a watched inbox or calling `curl`.
The command validates locally, then issues `PUT` to
`/factory-sessions/{session_id}/work-requests/{requestId}` (default session
`~default` when `--session` is omitted).

File path (primary form):

```bash
you submit batch ./factory/inputs/BATCH/default/release-story-set.json
```

Explicit file flag (`--file` wins when both a flag and a positional path are set):

```bash
you submit batch --file ./batches/deploy.json
```

Pipe batch JSON without a temp file:

```bash
cat batch.json | you submit batch
```

Read batch JSON from stdin explicitly:

```bash
you submit batch - < batch.json
```

Inline JSON positional (convenient for small batches; shell argument length limits apply):

```bash
you submit batch '{"requestId":"release-story-set","type":"FACTORY_REQUEST_BATCH","works":[{"name":"story-auth","workTypeName":"story","payload":{"title":"Harden auth session handling"}}]}'
```

Validate the public batch envelope locally without contacting the server
(`--dry-run` exits 0 on valid input even when the factory is unreachable). A
dry run checks the JSON shape, public field aliases, request discriminator,
request ID, non-empty `works[]`, and the topology-independent batch rules:
`works[].name` must be unique across the whole request, and every relation
endpoint must match a name in that request's `works[]`. It cannot compare
`workTypeName`, `state`, or `requiredState` with the running factory's topology
or validate the complete relation graph. Live admission repeats the same
topology-independent checks before applying Factory-topology validation.

```bash
you submit batch --dry-run '{"requestId":"release-story-set","type":"FACTORY_REQUEST_BATCH","works":[{"name":"story-auth","workTypeName":"story","payload":{"title":"Harden auth session handling"}}]}'
```

Target a non-default live session with structured output:

```bash
you --server http://localhost:7437 --json submit batch --session session-beta ./batch.json
```

Run `you submit batch --help` for the full input-mode matrix, global `--server`,
`--json`, and `--verbose` flags.

## Poller Stdout Contract

Service-owned `POLLER` workstations submit work through the same canonical
batch ingress path. A script-backed poller may write one JSON payload to
stdout using either of these shapes:

1. A canonical `FACTORY_REQUEST_BATCH` object.
2. An object with `"submissions": [...]`, where each item uses the internal
   submit-style record fields already used by the runtime.

Examples:

```json
{
  "requestId": "linear-issues-team-a-2026-05-22T07:00Z",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "issue-123",
      "workTypeName": "task",
      "payload": {
        "externalId": "ISSUE-123"
      }
    }
  ]
}
```

```json
{
  "submissions": [
    {
      "requestId": "linear-issues-team-a-2026-05-22T07:00Z",
      "workId": "issue-123",
      "name": "issue-123",
      "workTypeName": "task",
      "traceId": "linear-issue-123"
    }
  ]
}
```

Rules:

- Poller output must carry a stable non-empty `requestId`. Reusing the same
  `requestId` on a later poll is an idempotent no-op instead of creating
  duplicate work.
- Canonical batch output follows the same validation rules as watched-file and
  API-submitted `FACTORY_REQUEST_BATCH` requests.
- Raw factory event emission is not supported in poller stdout.
- The current script poller runner captures stdout when the subprocess exits,
  so a script poller should emit one complete batch payload per run.

## Where To Put Batch Files

Watched input files use this layout:

```text
factory/inputs/<work_type-or-BATCH>/<channel>/<filename>.json
```

Use these paths:

| Path | Use |
|------|-----|
| `factory/inputs/BATCH/default/<request_id>.json` | Manual mixed-work-type batches and canonical parent-child file input. |
| `factory/inputs/<work_type>/default/<request_id>.json` | Manual single-work-type batches. The watched folder can infer `workTypeName` when omitted. |
| `factory/inputs/<work_type>/<execution_id>/<request_id>.json` | Generated work tied to a parent execution. The channel name becomes the execution ID. |
| Any readable path passed to `you run --work <path>` | Startup work submitted before the run begins. |

Filename rules:

- The file must end in `.json` for the watcher to parse it as an explicit
  batch. Markdown files and non-batch JSON files are wrapped as one raw-payload
  work item instead.
- Prefer `<request_id>.json`, using lowercase words separated by hyphens.
- Keep the JSON `requestId` stable across retries. The filename does not have
  to match `requestId`, but matching them makes idempotency and logs easier to
  reason about.
- Avoid temporary suffixes such as `.tmp`, `.swp`, or `~`; the watcher ignores
  those files.

When a batch file is placed under `factory/inputs/BATCH/default/`, every work
item must set `workTypeName` explicitly because the folder does not imply one
shared work type.

## How Batches Work

The factory indexes every `works[].name` and validates the full batch before it
creates work tokens. Invalid JSON, retired field aliases, duplicate work names,
unknown same-batch relation endpoints, invalid work types, invalid states,
self-relations, and dependency cycles reject the whole batch. No partial Work
is created: a valid item earlier in `works[]` is not admitted when another item
or relation makes the request invalid.

## Atomic validation and rejection examples

The following focused fragments show invalid public batch shapes. They assume
the named `workTypeName` values exist in the target factory; the failure shown
is the batch rule being illustrated. Each request is rejected during admission,
and the factory creates no Work from any part of that request.

Duplicate names are invalid across the whole batch, not just within one work
type:

```json
{
  "requestId": "duplicate-name",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "release", "workTypeName": "story-set" },
    { "name": "release", "workTypeName": "story" }
  ]
}
```

The deterministic diagnostic names the duplicate and both entry paths, for
example: `work_request: duplicate name "release": works[1].name conflicts
with works[0].name; works[].name must be unique across the entire batch,
including across different workTypeName values; rename or remove one entry`.
Rename one entry or remove it before submitting. For example, this corrected
batch uses two distinct names even though the Work types remain different:

```json
{
  "requestId": "distinct-names",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "release-set", "workTypeName": "story-set" },
    { "name": "release-story", "workTypeName": "story" }
  ]
}
```

Relation endpoints must both be present in this request; an existing Work in a
different submission does not satisfy the lookup:

```json
{
  "requestId": "outside-batch-endpoint",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "publish", "workTypeName": "story" }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "publish",
      "targetWorkName": "review"
    }
  ]
}
```

The same deterministic check is available in live submission and `--dry-run`.
It identifies the relation index and type, both endpoint values, and the
missing field, for example: `work_request: relations[0] relation type
"DEPENDS_ON" has sourceWorkName "publish" and targetWorkName "review";
endpoint targetWorkName="review" is missing from this batch; relation
endpoints must name Work declared in this batch's works[] (not previously
submitted Work); add the named Work to works[] or correct targetWorkName`.
Repair it by adding the named Work to this request or changing the endpoint to
one of the names already declared in `works[]`. This corrected request adds
the prerequisite instead of referring to a Work submitted earlier:

```json
{
  "requestId": "intra-batch-endpoint",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "publish", "workTypeName": "story" },
    { "name": "review", "workTypeName": "story" }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "publish",
      "targetWorkName": "review"
    }
  ]
}
```

A relation cannot point to itself:

```json
{
  "requestId": "self-relation",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "review", "workTypeName": "story" }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "review",
      "targetWorkName": "review"
    }
  ]
}
```

`DEPENDS_ON` edges cannot form a cycle:

```json
{
  "requestId": "dependency-cycle",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "plan", "workTypeName": "story" },
    { "name": "execute", "workTypeName": "story" }
  ],
  "relations": [
    { "type": "DEPENDS_ON", "sourceWorkName": "plan", "targetWorkName": "execute" },
    { "type": "DEPENDS_ON", "sourceWorkName": "execute", "targetWorkName": "plan" }
  ]
}
```

For `DEPENDS_ON`, `requiredState` must be a state configured on the target Work
type. `PARENT_CHILD` must omit `requiredState`; supplying it rejects the whole
batch because that relation type records lineage rather than prerequisite
state gating:

```json
{
  "requestId": "unknown-required-state",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    { "name": "review", "workTypeName": "story" },
    { "name": "publish", "workTypeName": "story" }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "publish",
      "targetWorkName": "review",
      "requiredState": "not-a-configured-state"
    }
  ]
}
```

The server's admission diagnostic identifies the offending work or relation
and rule (duplicate name, unknown endpoint, self-dependency, dependency cycle,
or unknown required state). The duplicate-name and same-batch endpoint checks
also run during `--dry-run`; the dry run does not contact a Factory and cannot
validate topology-dependent values such as work types, states, or required
states. In live submission, any failed check rejects the whole request before
Work or relationships are admitted.

After validation, the factory normalizes the batch:

1. Missing work IDs are generated as `batch-<requestId>-<work-name>`.
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
  "sourceWorkName": "publish",
  "targetWorkName": "review",
  "requiredState": "complete"
}
```

Read that as: `publish` waits for `review`.

`PARENT_CHILD` example:

```json
{
  "type": "PARENT_CHILD",
  "sourceWorkName": "story-auth",
  "targetWorkName": "story-set"
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
| `requestId` | Yes | Stable idempotency key for the full submission. |
| `type` | Yes | Must be `FACTORY_REQUEST_BATCH`. |
| `works[].name` | Yes | Relations refer to work items by name. |
| `works[].workTypeName` | Yes for `inputs/BATCH` | Mixed-work-type parent-child batches cannot rely on folder inference. |
| `works[].state` on the parent | Usually | Place the parent directly into the waiting state consumed by the parent-aware fan-in workstation. |
| `relations[].type` | Yes | Use `PARENT_CHILD` for submitted parent-child membership. |
| `relations[].sourceWorkName` | Yes | Name of the child work item. |
| `relations[].targetWorkName` | Yes | Name of the parent work item. |

Children usually omit `state` so they start in their work type's initial
state. Set a child `state` only when you intentionally need non-initial
placement.

## Request Fields

| Field | Required | Description |
|-------|----------|-------------|
| `requestId` | Yes | Stable client-provided request identifier. The API requires the path `{request_id}` and body `requestId` to match. Some lower-level submit paths can fill a missing ID, but public batch files should set it explicitly. |
| `type` | Yes | Must be `FACTORY_REQUEST_BATCH`. |
| `works` | Yes | Array of one or more work items. |
| `relations` | No | Array of relations between named work items in this batch. |
| `currentChainingTraceId` | No | Optional default chaining-trace identifier applied to submitted work items that omit `currentChainingTraceId` or `traceId`. |

Do not use `work_type_id`. Public batch inputs use `workTypeName`; retired
`work_type_id` aliases are rejected at submit boundaries with guidance to use
`workTypeName`.

## Work Item Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Authored Work name. Names must be unique across the entire batch because relations refer to Work by name. |
| `workTypeName` | Usually | Configured work type from `factory.json`. Watched input files can infer this from `factory/inputs/<work_type>/...` when omitted, but `inputs/BATCH` requires it on every work item. |
| `state` | No | Starting state for this work item. Omit it to use the work type's initial state. Use it on a parent item when fan-in should start from a waiting state. |
| `workId` | No | Stable unique work ID. Omit this unless an external system needs a specific ID. |
| `requestId` | No | Per-work request ID override. Omit this for normal batches so work items inherit the top-level `requestId`. |
| `currentChainingTraceId` | No | Preferred chaining-trace identifier for this work item. |
| `traceId` | No | Legacy trace identifier retained for compatibility; prefer `currentChainingTraceId`. Omit either field to let the batch share one trace. |
| `payload` | No | Opaque work payload. Objects, arrays, strings, numbers, booleans, and `null` are accepted. |
| `tags` | No | String key-value metadata available to prompt templates and parameterized workstation fields. |

Avoid setting tag names that begin with `_work_`. The factory writes
`_work_name` and `_work_type` during normalization.

## Relation Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Use `DEPENDS_ON` or `PARENT_CHILD`. |
| `sourceWorkName` | Yes | Name of the blocked work item for `DEPENDS_ON`, or the child work item for `PARENT_CHILD`. |
| `targetWorkName` | Yes | Name of the prerequisite work item for `DEPENDS_ON`, or the parent work item for `PARENT_CHILD`. |
| `requiredState` | Only for `DEPENDS_ON` | Target state required before the source can run. Defaults to `complete`. Omit it for `PARENT_CHILD`; supplying it rejects the whole batch. |

Declare batch relations by name. Do not use `targetWorkId` in submitted batch
relations; target work IDs are resolved during normalization and may appear in
events after submission.

## Visualize batch dependencies (`you work visualize`)

Inspect declared work dependencies in a local batch file without submitting it
to a factory. The command is **read-only**: it parses the batch JSON from disk,
derives a dependency graph, and writes diagram text to stdout. It does not submit
work, contact a running factory, or render diagram images directly.

Graph nodes represent work items from `works[]`. Directed edges represent
declared batch relations (`DEPENDS_ON` and `PARENT_CHILD`) using
`sourceWorkName` → `targetWorkName` semantics from the batch file.

| Output | Command |
|--------|---------|
| Raw Mermaid `flowchart` (default) | `you work visualize <batch-file.json>` |
| Markdown with fenced `mermaid` block | `you work visualize --format markdown-mermaid <batch-file.json>` |

Redirect stdout to save the diagram for your own renderer or docs tooling:

```text
you work visualize batch.json > my-graph.mermaid
you work visualize --format markdown-mermaid batch.json > graph.md
```

On success, graph output goes to stdout and diagnostics go to stderr. Invalid
JSON, unsupported batch shape, missing files, and unsupported `--format` values
exit non-zero with an empty stdout graph.

See `you docs relationships` for relation semantics and validation rules.

## Validation Checklist

Before dropping a batch file into `factory/inputs/...`, confirm:

- The filename ends in `.json`.
- `type` is exactly `FACTORY_REQUEST_BATCH`.
- `requestId` is stable and unique for the intended submission.
- Every work item has a unique `name` across the whole `works[]` array, even
  when entries use different `workTypeName` values.
- Every `inputs/BATCH` work item sets `workTypeName`.
- Parent work items that feed fan-in use the exact waiting `state` expected by
  the guarded parent input.
- Every `PARENT_CHILD.sourceWorkName` names a child.
- Every `PARENT_CHILD.targetWorkName` names a parent.
- Every relation source and target matches a work item name in this request's
  `works[]`; do not target previously submitted or otherwise existing Work by
  name.
- `requiredState`, when used on `DEPENDS_ON`, names an actual state on the
  target work type.
- `DEPENDS_ON` relations do not create cycles.

## Related

- `you docs agents`
- `you docs config`
- `you docs work`
- `you docs authoring-factories`
- `you docs relationships`
- `you docs guards`
- `you docs workstations`
- `you docs templates`
