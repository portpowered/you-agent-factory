---
author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/authoring-workflows
---

# Authoring Workflows

Use this guide to create and run a current Agent Factory workflow with the
public `factory.json` contract. Keep topology in `factory.json`, worker runtime
instructions in `workers/<name>/AGENTS.md`, and workstation prompts in
`workstations/<name>/AGENTS.md`.

Use this guide for workflow sequencing, runnable examples, and command order.
Use [Factory JSON And Work Configuration](work.md) for the field-by-field
`factory.json` reference, [Workstations](workstations.md) for workstation
runtime fields, [Workers](workers.md) for worker backend fields, and
[Batch Inputs](batch-inputs.md) for the watched-file and API request shape.

## Recommended Layout

```text
factory/
  factory.json
  workers/
    executor/AGENTS.md
    reviewer/AGENTS.md
  workstations/
    execute-story/AGENTS.md
    review-story/AGENTS.md
  inputs/
    story/
      default/
```

`factory.json` owns the work graph: work types, states, workers, workstations,
resources, and routing. The split `AGENTS.md` files own prompt-heavy runtime
configuration that is easier to maintain outside JSON.

## Minimal Workflow

A minimal workflow needs one work type, one worker, and one workstation:

```json
{
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "processor" }
  ],
  "workstations": [
    {
      "name": "process-task",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

At runtime:

1. A submitted `task` work item starts in `task:init`.
2. `process-task` is enabled when a token is present in that place.
3. Accepted work routes to `task:complete`.
4. Failed or timed-out work routes to `task:failed`.

Use [Factory JSON And Work Configuration](work.md#how-the-pieces-fit) for the
canonical routing contract, including continue and rejection routes.

## Build Your First Workflow

This walkthrough creates a two-stage execution and review loop with canonical
camelCase config fields.

### 1. Create `factory.json`

```json
{
  "id": "sample-service",
  "resources": [
    { "name": "agent-slot", "capacity": 1 }
  ],
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "in-review", "type": "PROCESSING" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
      "onContinue": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    },
    {
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [{ "type": "VISIT_COUNT", "workstation": "review-story", "maxVisits": 3 }],
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }]
    }
  ]
}
```

This topology gives you one execution pass, one review pass, and an explicit
guarded loop breaker so a rejected story cannot cycle forever.

### Optional portability manifest

Add `supportingFiles` only when the workflow also needs declarative host-tool
checks or bundled helper files that should travel with the factory contract.
Use
[Factory JSON And Work Configuration](work.md#portability-resource-manifest)
for the manifest fields and validation rules.

### 2. Create the split runtime definitions

`workers/executor/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are a software engineer. Implement the requested story and run focused
verification before finishing.
```

`workers/reviewer/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 30m
skipPermissions: true
---

You review the story implementation and return ACCEPTED only when the change is
ready.
```

`workstations/execute-story/AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 1h
---

Implement the story.

Story payload:
{{ (index .Inputs 0).Payload }}

Return CONTINUE when the story made ordinary partial progress but needs another
execution pass.
Return COMPLETE only when the story is ready to advance into review.
```

`workstations/review-story/AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 30m
---

Review the story implementation.

Story payload:
{{ (index .Inputs 0).Payload }}

Return ACCEPTED when the story is ready.
Return REJECTED with concrete feedback when another pass is needed.
```

### 3. Start the factory

Use mock workers for the first routing check:

```bash
you run --dir ./factory --with-mock-workers
```

The command loads `factory.json`, resolves the split `AGENTS.md` files, starts
continuous mode, and exposes the dashboard and API on the configured port.

### 4. Submit work

Create a startup or watched-file request:

```json
{
  "request_id": "story-001",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-001",
      "work_type_name": "story",
      "payload": {
        "title": "Add review checklist"
      }
    }
  ]
}
```

Run it at startup:

```bash
you run --dir ./factory --with-mock-workers --work ./fixtures/story-001.json
```

Or drop the file under `factory/inputs/story/default/` while the factory is
already running.

## Related Contract Detail

- [Factory JSON And Work Configuration](work.md) owns work types, states,
  routing, resources, and portability fields.
- [Workstations](workstations.md) owns workstation kinds, runtime fields,
  route fields, and guards.
- [Workers](workers.md) owns worker types, backend fields, and worker
  `AGENTS.md` placement.
- [Author AGENTS.md](authoring-agents-md.md) owns split file shape, prompt
  placement, and authoring patterns.

## Failure Routing And Provider Behavior

For workflow design, add explicit failure, continue, and rejection destinations
to the topology so every outcome lands somewhere intentional. Use
[Factory JSON And Work Configuration](work.md#how-the-pieces-fit) for the
canonical routing contract, [Workstations](workstations.md) for route fields
and execution limits, and [Workers](workers.md) for worker backend behavior.

## Test Workflows With Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

For the simplest validation run, omit the config path:

```bash
you run --dir ./factory --with-mock-workers
```

That is equivalent to this config:

```json
{
  "mockWorkers": []
}
```

To target specific dispatches, pass a config path:

```bash
you run --dir ./factory --with-mock-workers ./mock-workers.json
```

Example:

```json
{
  "mockWorkers": [
    {
      "id": "reviewer-rejects-first-pass",
      "workerName": "reviewer",
      "workstationName": "review-story",
      "workInputs": [
        {
          "workType": "story",
          "state": "in-review",
          "inputName": "work"
        }
      ],
      "runType": "reject",
      "rejectConfig": {
        "stdout": "needs changes",
        "stderr": "missing acceptance criteria",
        "exitCode": 42
      }
    }
  ]
}
```

Selection fields combine as filters:

| Field | Matches |
|-------|---------|
| `workerName` | Worker identity from `workers[].name` |
| `workstationName` | Workstation currently executing |
| `workInputs` | Consumed token fields such as `workType`, `state`, `inputName`, `traceId`, or `payloadHash` |

If no entry matches, mock-worker mode returns the default accepted result.

## Authoring Checklist

- Keep the public workflow contract in `factory.json`.
- Use camelCase factory-config fields such as `workTypes`, `resources`,
  `onFailure`, `onRejection`, and `maxVisits`.
- Use `supportingFiles` only for portability-only concerns such as
  validation-only PATH tools and explicitly bundled scripts or docs.
- Keep prompt-heavy worker and workstation runtime fields in split `AGENTS.md`
  files unless you intentionally need a single-file config.
- Add a guarded `LOGICAL_MOVE` workstation for repeater or review loops.
- Use [Batch Inputs](batch-inputs.md) for `FACTORY_REQUEST_BATCH`
  request files.
- Use [Workstations](workstations.md) for cron, prompt templates, timeouts, and
  workstation runtime field details.
- Use [Workers](workers.md) for worker backend field details.

## Related

- [Factory JSON And Work Configuration](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Batch Inputs](batch-inputs.md)
- [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
- [Prompt Template Variables](prompt-variables.md)
- [README](../README.md)
