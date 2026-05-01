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

Use [Factory JSON And Work Configuration](work.md) for the field-by-field
reference, [Workstations And Workers](workstations.md) for prompt and cron
fields, and [Batch Inputs](guides/batch-inputs.md) for the watched-file and API
request shape.

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
3. Accepted work routes through `outputs`.
4. Ordinary partial-progress work routes through `onContinue` when configured.
5. Rejected work routes through `onRejection` when configured.
6. Failed or timed-out work routes through `onFailure`.

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

```json
{
  "supportingFiles": {
    "requiredTools": [
      {
        "name": "python",
        "command": "python",
        "purpose": "Runs bundled helper scripts",
        "versionArgs": ["--version"]
      }
    ],
    "bundledFiles": [
      {
        "type": "ROOT_HELPER",
        "targetPath": "Makefile",
        "content": {
          "encoding": "utf-8",
          "inline": "test:\n\tgo test ./...\n"
        }
      },
      {
        "type": "SCRIPT",
        "targetPath": "factory/scripts/setup-workspace.py",
        "content": {
          "encoding": "utf-8",
          "inline": "print('portable setup')\n"
        }
      },
      {
        "type": "DOC",
        "targetPath": "factory/docs/usage.md",
        "content": {
          "encoding": "utf-8",
          "inline": "# Usage\nRun the setup script before starting the workflow.\n"
        }
      }
    ]
  }
}
```

- `requiredTools` are declarative only. Load or preflight validation can check
  whether `command` resolves on `PATH`, but the factory does not install or
  embed those tools.
- `config flatten` collects the supported allowlist from `factory/scripts/**`,
  `factory/docs/**`, and supported root helper files such as `Makefile` when
  you flatten a checked-in `factory/` layout.
- `SCRIPT` entries must target `factory/scripts/...`, `DOC` entries must
  target `factory/docs/...`, `ROOT_HELPER` entries must target a supported
  project-root helper file such as `Makefile`, and `content.encoding` is
  `utf-8` in this v1 portability slice.
- `targetPath` must already be canonical: use forward slashes, keep it
  factory-relative, and do not use absolute paths or `.` / `..` segments.

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
agent-factory run --dir ./factory --with-mock-workers
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
agent-factory run --dir ./factory --with-mock-workers --work ./fixtures/story-001.json
```

Or drop the file under `factory/inputs/story/default/` while the factory is
already running.

## Failure Routing And Provider Behavior

Use `onFailure` on workstations for terminal worker failures and timeouts.
Accepted work routes through `outputs`. Ordinary executor iteration routes
through `onContinue` when configured. Explicit reviewer feedback routes through
`onRejection`.

For model-backed workers, normalized provider behavior applies before the token
reaches its final route:

- permanent auth, bad request, and misconfiguration failures are terminal
- retryable provider failures retry inside the executor before the workflow
  sees a final failure
- throttling can pause the affected provider/model lane and requeue the
  in-flight work to its pre-transition position

The canonical timeout and normalized-failure reference lives in
[Authoring AGENTS.md](./authoring-agents-md.md#timeout-and-failure-behavior).

## Test Workflows With Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

For the simplest validation run, omit the config path:

```bash
agent-factory run --dir ./factory --with-mock-workers
```

That is equivalent to this config:

```json
{
  "mockWorkers": []
}
```

To target specific dispatches, pass a config path:

```bash
agent-factory run --dir ./factory --with-mock-workers ./mock-workers.json
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
- Use `resourceManifest` only for portability-only concerns such as
  validation-only PATH tools and explicitly bundled scripts or docs.
- Keep prompt-heavy worker and workstation runtime fields in split `AGENTS.md`
  files unless you intentionally need a single-file config.
- Add a guarded `LOGICAL_MOVE` workstation for repeater or review loops.
- Use [Batch Inputs](guides/batch-inputs.md) for `FACTORY_REQUEST_BATCH`
  request files.
- Use [Workstations And Workers](workstations.md) for cron, prompt templates,
  timeouts, and runtime field details.

## Related

- [Factory JSON And Work Configuration](work.md)
- [Workstations And Workers](workstations.md)
- [Batch Inputs](guides/batch-inputs.md)
- [Parent-Aware Fan-In](guides/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
- [Prompt Template Variables](prompt-variables.md)
- [README](../README.md)
