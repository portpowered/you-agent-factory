---
author: ralph agent
last modified: 2026, april, 21
doc-id: AGF-DOC-003
---

# Understand A Run Timeline

After reading this guide, you will be able to follow one Agent Factory run through the same ordered event timeline used by `/events`, recordings, replay, and the dashboard.

## Prerequisites

- An Agent Factory process running locally.
- The HTTP API port for that process. The preferred default port is `7437`,
  and the CLI falls forward to the next available port when that port is busy.
  Use the `Dashboard URL` printed at startup for the exact port.

## Stream The Timeline

`GET /events` streams the run timeline as server-sent events. The stream sends existing events first, then keeps the connection open for new events.

```bash
curl -N http://127.0.0.1:7437/events
```

Each `data:` frame is one `FactoryEvent`. Events are ordered by `context.sequence`. Replay and selected-tick views use `context.tick` to explain what the factory knew at a logical point in the run.

If you are upgrading an older consumer, update any legacy event filters to the current public event names before switching over.

## Timeline Order

A normal run appears in this order:

| Step | Public event | Meaning |
|------|--------------|---------|
| Run requested | `RUN_REQUEST` | The run began and the effective runtime configuration is known. |
| Topology defined | `INITIAL_STRUCTURE_REQUEST` | The factory topology, work types, workstations, workers, and resources are available before work moves. |
| Work entered | `WORK_REQUEST` | One or more work items entered the factory. Single-work submissions are normalized into this event. |
| Relationship declared | `RELATIONSHIP_CHANGE_REQUEST` | Parent-child or dependency relations for a batch request were recorded. This event appears only when relations exist. |
| Dispatch requested | `DISPATCH_REQUEST` | A workstation began processing input work. |
| Inference requested | `INFERENCE_REQUEST` | A model-worker provider attempt is about to call the provider. |
| Inference responded | `INFERENCE_RESPONSE` | The matching provider attempt returned or failed. |
| Dispatch responded | `DISPATCH_RESPONSE` | A workstation finished and reported `ACCEPTED`, `REJECTED`, or `FAILED`. |
| State responded | `FACTORY_STATE_RESPONSE` | The factory reported a lifecycle state such as `RUNNING`, `IDLE`, or `COMPLETED`. |
| Run responded | `RUN_RESPONSE` | The run ended and final metadata is available. |

The dashboard, record/replay, and selected-tick views all start from this same timeline. If two views disagree, compare the event ids, event types, sequence numbers, ticks, work ids, and dispatch ids first.

Names ending in `_REQUEST` describe accepted work or scheduled lifecycle steps.
Names ending in `_RESPONSE` describe the resulting outcome or state snapshot.

## Work Entering The Factory

A `WORK_REQUEST` event records the request that introduced work to the run. Request identity lives in `context.requestId`; the work items live in `payload.works`.

```json
{
  "schemaVersion": "agent-factory.event.v1",
  "id": "evt-work-001",
  "type": "WORK_REQUEST",
  "context": {
    "sequence": 2,
    "tick": 1,
    "eventTime": "2026-04-18T16:00:00Z",
    "requestId": "req-doc-001",
    "traceIds": ["trace-doc-001"],
    "workIds": ["work-doc-001"],
    "source": "api"
  },
  "payload": {
    "type": "FACTORY_REQUEST_BATCH",
    "source": "api",
    "works": [
      {
        "name": "Review onboarding copy",
        "work_id": "work-doc-001",
        "request_id": "req-doc-001",
        "work_type_name": "doc",
        "trace_id": "trace-doc-001",
        "payload": {
          "path": "docs/onboarding.md"
        },
        "tags": {
          "priority": "normal"
        }
      }
    ],
    "relations": []
  }
}
```

## Inference Attempts

Model-worker dispatches emit `INFERENCE_REQUEST` before the provider call and
`INFERENCE_RESPONSE` after that attempt succeeds or fails. Both events include
`dispatchId`, `transitionId`, `attempt`, and `inferenceRequestId` so retries can
be correlated without parsing provider output.

`INFERENCE_REQUEST.payload.prompt` is the rendered prompt sent to the provider.
Recordings that include this event can contain customer task text and should be
handled as sensitive artifacts.

```json
{
  "schemaVersion": "agent-factory.event.v1",
  "id": "evt-inference-request-001",
  "type": "INFERENCE_REQUEST",
  "context": {
    "sequence": 4,
    "tick": 2,
    "eventTime": "2026-04-18T16:00:03Z",
    "traceIds": ["trace-doc-001"],
    "workIds": ["work-doc-001"],
    "dispatchId": "dispatch-process-001",
    "source": "worker"
  },
  "payload": {
    "inferenceRequestId": "inference-request-001",
    "dispatchId": "dispatch-process-001",
    "transitionId": "process",
    "attempt": 1,
    "workingDirectory": "/workspace/project",
    "worktree": "/workspace/project/.worktrees/doc-001",
    "prompt": "Review the onboarding copy."
  }
}
```

The matching response uses the same `inferenceRequestId` and records the
provider outcome, response text when present, duration, and failure details when
available.

```json
{
  "schemaVersion": "agent-factory.event.v1",
  "id": "evt-inference-response-001",
  "type": "INFERENCE_RESPONSE",
  "context": {
    "sequence": 5,
    "tick": 2,
    "eventTime": "2026-04-18T16:00:06Z",
    "traceIds": ["trace-doc-001"],
    "workIds": ["work-doc-001"],
    "dispatchId": "dispatch-process-001",
    "source": "worker"
  },
  "payload": {
    "inferenceRequestId": "inference-request-001",
    "dispatchId": "dispatch-process-001",
    "transitionId": "process",
    "attempt": 1,
    "outcome": "SUCCEEDED",
    "response": "Updated the onboarding copy.",
    "durationMillis": 3000
  }
}
```

## Dispatch Response

A `DISPATCH_RESPONSE` event records the outcome from a workstation. The event keeps the customer-visible work, worker, workstation, result, and diagnostics needed to explain the run.

```json
{
  "schemaVersion": "agent-factory.event.v1",
  "id": "evt-dispatch-002",
  "type": "DISPATCH_RESPONSE",
  "context": {
    "sequence": 6,
    "tick": 3,
    "eventTime": "2026-04-18T16:00:08Z",
    "traceIds": ["trace-doc-001"],
    "workIds": ["work-doc-001"],
    "dispatchId": "dispatch-process-001",
    "source": "worker"
  },
  "payload": {
    "completionId": "completion-process-001",
    "dispatchId": "dispatch-process-001",
    "transitionId": "process",
    "workstation": {
      "name": "doc-processor",
      "worker": "copy-editor",
      "inputs": [
        {
          "workType": "doc",
          "state": "init"
        }
      ],
      "outputs": [
        {
          "workType": "doc",
          "state": "in-review"
        }
      ]
    },
    "worker": {
      "name": "copy-editor",
      "type": "MODEL_WORKER",
      "provider": "codex"
    },
    "outcome": "ACCEPTED",
    "output": "Updated the onboarding copy.",
    "durationMillis": 8200,
    "inputs": [
      {
        "name": "Review onboarding copy",
        "work_id": "work-doc-001",
        "request_id": "req-doc-001",
        "work_type_name": "doc",
        "trace_id": "trace-doc-001"
      }
    ],
    "outputWork": [
      {
        "name": "Review onboarding copy",
        "work_id": "work-doc-001",
        "request_id": "req-doc-001",
        "work_type_name": "doc",
        "trace_id": "trace-doc-001"
      }
    ]
  }
}
```

## Replay Divergence

Replay re-runs the recorded event timeline. If current behavior no longer matches the recording, replay stops at the first material difference and reports the event id and tick to inspect.

```json
{
  "category": "dispatch_mismatch",
  "tick": 3,
  "dispatch_id": "dispatch-process-001",
  "expected_event_id": "evt-dispatch-001",
  "observed_event_id": "evt-dispatch-current-001",
  "expected": "worker=copy-editor workstation=doc-processor",
  "observed": "worker=reviewer workstation=doc-review"
}
```

Start investigation with `expected_event_id` in the recorded artifact, then compare the observed event from the current run at the same tick.

## Common Errors

| Error | Cause | Resolution |
|-------|-------|------------|
| The stream stays open after the last event | `/events` is a live server-sent event stream. | Keep the command open when watching live runs, or stop `curl` when you only need the historical events. |
| Replay reports `unsupported replay artifact schemaVersion` | The artifact was not written by a supported Agent Factory recorder. | Record the run again with the current binary, or use a fixture already migrated to the current event-log artifact. |
| A dashboard view does not match a recording | The view and recording may be based on different runs or different event ids. | Compare `schemaVersion`, `id`, `type`, `context.sequence`, `context.tick`, `context.workIds`, and `context.dispatchId` before comparing rendered views. |

## Next Steps

- [Agent Factory record and replay](record-replay.md)
- [Author workflows](../reference/authoring-workflows.md)
- [Run the live dashboard](development/live-dashboard.md)
