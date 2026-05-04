# Agent Factory API Inventory

This inventory captures the current HTTP behavior implemented by `pkg/api/server.go` and `pkg/api/handlers.go`. The authored Agent Factory contract source is `api/openapi-main.yaml` plus referenced fragments such as `api/components/schemas/events/`; `api/openapi.yaml` is the bundled published artifact described here. Keep this document aligned with that bundled contract when routes are added or removed.

## Scope

The current JSON and event API surface covers these endpoints:

- `POST /work`
- `GET /work`
- `GET /work/{id}`
- `PUT /work-requests/{request_id}`
- `GET /events`
- `GET /status`

`GET /dashboard/ui` and `GET /dashboard/ui/*` serve the embedded dashboard application shell and static assets. Static dashboard UI delivery is outside the JSON transport contract.

Removed cleanup surfaces are intentionally absent from the active API:

- `GET /state`
- `GET /dashboard`
- `GET /dashboard/stream`
- `GET /traces/{traceID}`
- `GET /work/{id}/trace`
- `GET /workflows`
- `GET /workflows/{id}`

## Standard Error Shape

JSON handlers emit errors as:

```json
{
  "message": "human-readable message",
  "code": "MACHINE_READABLE_CODE"
}
```

Current codes observed in handlers are `BAD_REQUEST`, `NOT_FOUND`, and `INTERNAL_ERROR`.

## Endpoints

### POST /work

Submits one work item to the running factory.

Request body:

```json
{
  "name": "optional display name",
  "work_type_name": "task",
  "trace_id": "optional caller trace id",
  "payload": {},
  "tags": {
    "key": "value"
  },
  "relations": []
}
```

Required fields:

- `work_type_name`

Success:

- Status: `201 Created`
- Body:

```json
{
  "trace_id": "trace-..."
}
```

Behavior notes:

- If `trace_id` is omitted, the handler normalizes the submission and returns a generated trace ID.
- `payload` is accepted as raw JSON and forwarded to the factory submission request.
- `relations` uses the Petri net relation shape currently defined by `petri.Relation`.

Errors:

- `400 BAD_REQUEST` when the JSON body cannot be decoded.
- `400 BAD_REQUEST` when `work_type_name` is empty.
- `500 INTERNAL_ERROR` when factory submission fails.

### PUT /work-requests/{request_id}

Submits or retries one canonical work request batch.

Path parameters:

- `request_id`: stable caller-provided request identifier.

Request body:

```json
{
  "request_id": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "work_type_name": "story-set",
      "state": "waiting"
    },
    {
      "name": "story-a",
      "work_type_name": "story"
    }
  ],
  "relations": [
    {
      "type": "PARENT_CHILD",
      "source_work_name": "story-a",
      "target_work_name": "story-set"
    }
  ]
}
```

Success:

- Status: `201 Created`
- Body:

```json
{
  "request_id": "request-1",
  "trace_id": "trace-request-1"
}
```

Behavior notes:

- The path `request_id` must match the JSON body `request_id`.
- Repeated request IDs are idempotent and return the originally accepted trace metadata.
- Missing work item trace IDs inherit a stable request trace.
- `works[].state` places a submitted work item directly into a named state instead of the initial state.
- `PARENT_CHILD.source_work_name` is the child and `PARENT_CHILD.target_work_name` is the parent.
- Public work-request bodies use `state` and `work_type_name`; internal aliases such as `target_state` and `work_type_id` are rejected.

Errors:

- `400 BAD_REQUEST` when the request is malformed or fails work-request validation.
- `500 INTERNAL_ERROR` when factory submission fails unexpectedly.

### GET /work

Lists current work tokens from the engine state snapshot marking.

Query parameters:

- `maxResults`: optional positive integer page size. Defaults to `50`; malformed or empty values return `400 BAD_REQUEST`, and non-positive values fall back to the default page size.
- `nextToken`: optional base64-encoded token ID cursor.

Success:

- Status: `200 OK`
- Body:

```json
{
  "results": [
    {
      "id": "tok-1",
      "place_id": "task:init",
      "name": "display name",
      "work_id": "work-1",
      "work_type": "task",
      "trace_id": "trace-1",
      "tags": {
        "key": "value"
      },
      "created_at": "2026-04-12T16:30:00Z",
      "entered_at": "2026-04-12T16:30:00Z"
    }
  ],
  "paginationContext": {
    "maxResults": 50,
    "nextToken": "base64-token-id"
  }
}
```

Behavior notes:

- Results are sorted by token ID before pagination.
- List responses omit token history.
- Internal time work tokens such as `__system_time` are omitted from this public list; use `GET /events` or debug projections for cron timing metadata.
- `paginationContext` is omitted when no further page exists.

Errors:

- `500 INTERNAL_ERROR` when the engine state snapshot cannot be read.

### GET /work/{id}

Returns one token by token ID from the current marking.

Path parameters:

- `id`: token ID.

Success:

- Status: `200 OK`
- Body: the same token shape as `GET /work`, with `history` included.

History shape:

```json
{
  "total_visits": {
    "transition": 1
  },
  "consecutive_failures": {},
  "place_visits": {
    "task:init": 1
  },
  "last_error": "optional error"
}
```

Errors:

- `404 NOT_FOUND` when the token ID is missing from the marking.
- `404 NOT_FOUND` when the token ID belongs to hidden internal time work.
- `500 INTERNAL_ERROR` when the engine state snapshot cannot be read.

### GET /events

Streams canonical factory events as server-sent events.

Success:

- Status: `200 OK`
- Content type: `text/event-stream`
- Body: historical `data: {...}` events first, followed by live events.

Behavior notes:

- Consumers reconstruct selected ticks from `factory.ReconstructFactoryWorldState(...)`.
- The browser dashboard uses this endpoint as its canonical runtime source.
- The stream remains open until the client disconnects or the factory event stream closes.

Errors:

- `500 INTERNAL_ERROR` when event streaming is unavailable.

### GET /status

Returns aggregate lifecycle and resource counts from the engine state snapshot.

Success:

- Status: `200 OK`
- Body:

```json
{
  "factory_state": "RUNNING",
  "runtime_status": "IDLE",
  "total_tokens": 3,
  "categories": {
    "initial": 0,
    "processing": 1,
    "terminal": 1,
    "failed": 0
  },
  "resources": [
    {
      "name": "executor-slot",
      "available": 1,
      "total": 2
    }
  ]
}
```

Behavior notes:

- Work-token categories are derived from workflow topology state definitions.
- Resource totals are derived from topology resource capacity, with availability counted from current resource tokens.
- Internal time work is excluded from customer-facing token totals and categories.
- The Agent Factory CLI `status` command uses this route.

Errors:

- `500 INTERNAL_ERROR` when the engine state snapshot cannot be read.

## Verification

The current inventory is backed by handler and functional coverage in:

- `pkg/api/server_test.go`, including `TestSubmitWorkThenListWork_ConfirmsObservedJSONFields` and `TestGetStatus_ReturnsAggregateSnapshotStatus`.
- `tests/functional_test/generated_api_smoke_test.go`, including removed-route assertions for retired cleanup endpoints.
