author: Agent Factory Team
---
last-modified: 2026-04-21
doc-id: agent-factory/work
---

# Factory JSON And Work Configuration

`factory.json` declares the workflow topology for a you-agent-factory run. It
defines the work types, states, workers, workstations, resources, and routing
behavior that the runtime turns into a Petri-net execution model.

Use this guide when writing or reviewing `factory.json`. For the JSON file you
drop into `inputs/<workType>/...`, see [Batch Inputs](batch-inputs.md).

This is the canonical customer-facing guide for work and top-level
`factory.json` configuration. Keep work types, work states, routing behavior,
runtime resource pools, and portability fields here. Keep workstation-specific
runtime step behavior in [Workstations](workstations.md), worker backend fields
in [Workers](workers.md), and submitted request payload details in
[Batch Inputs](batch-inputs.md).

## When To Use This Guide

| Need | Use |
|------|-----|
| Define `factory.json`, work types, states, top-level resources, routing, or portability fields | This guide |
| Place batch request files under `inputs/`, define `FACTORY_REQUEST_BATCH`, or choose `DEPENDS_ON` versus `PARENT_CHILD` | [Batch Inputs](batch-inputs.md) |
| Tune bounded concurrency pools and workstation resource requirements | [Resources](resources.md) |
| Walk through a full setup sequence with example files and commands | [Author Factories](authoring-factories.md) |

## Minimal Factory

A minimal factory needs one work type, one worker, and one workstation that
moves submitted work from an initial state to a terminal state:

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
      "name": "process",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

With the split layout, runtime instructions live beside `factory.json`:

```text
factory/
  factory.json
  workers/processor/AGENTS.md
  workstations/process/AGENTS.md
  inputs/task/default/
```

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
required as `works[].name` for batch requests. Single-work `POST /work` bodies
use the OpenAPI camelCase fields such as `workTypeName` and `traceId`; batch
request bodies continue to use `works[].work_type_name` and `trace_id`. This
change does not alter the existing batch naming rule.

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

## How The Pieces Fit

Work enters the factory as a token in a work type's initial state. A
workstation is enabled when its configured input places have matching tokens.
The workstation dispatches to its worker, then routes the token based on the
worker outcome:

| Worker outcome | Routing field |
|----------------|---------------|
| Accepted | `outputs` |
| Continue | `onContinue` |
| Rejected | `onRejection` |
| Failed, timed out, or errored | `onFailure` |

Each `workType` and `state` pair becomes a place named
`<workType>:<state>`, such as `task:init`.

## Top-Level Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | No | Factory-level identifier. Prompt context uses this when a submitted work item does not carry a `project` tag. |
| `inputTypes` | No | Named input kinds. The implicit `default` input type already exists; omit this unless adding a supported non-default input kind. |
| `workTypes` | Yes | Work categories and lifecycle states. Workstation input and output places must reference these names. |
| `resources` | No | Bounded concurrency pools. Workers and workstations declare requirements against these pools through their `resources` entries. |
| `supportingFiles` | No | Portability-only manifest for validation-only external tools and bundled files. This is distinct from runtime-capacity `resources`. |
| `runner` | No | Factory-level default runner ID. Supported built-ins are `codex`, `gemini`, `kiro`, `cursor-cli`, and `opencode`. |
| `workers` | Yes | Worker identities that workstations reference by `name`; see [Workers](workers.md) for worker runtime fields. |
| `workstations` | Yes | Dispatch steps that consume input states and produce output states; see [Workstations](workstations.md) for the workstation field contract. |

Do not rely on stale top-level `global_limits` or `exhaustionRules` examples.
The current public `factory.json` authoring contract uses guarded
`LOGICAL_MOVE` workstations and workstation limits for user-configured safety
behavior.

## Portability Resource Manifest

Use `supportingFiles` when the factory must declare portability dependencies
that are not runtime-capacity pools.

```json
{
  "supportingFiles": {
    "requiredTools": [
      {
        "name": "python",
        "command": "python3",
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
          "inline": "print('portable')\n"
        }
      },
      {
        "type": "DOC",
        "targetPath": "factory/docs/usage.md",
        "content": {
          "encoding": "utf-8",
          "inline": "# Usage\n"
        }
      }
    ]
  }
}
```

- `requiredTools` declare validation-only external dependencies that later
  portability checks can probe on `PATH`.
- `bundledFiles` carry portable file content and a canonical factory-relative
  `targetPath`; they are not the same as runtime `resources`.
- In v1 shared-factory flows, the runtime also uses `bundledFiles` to carry a
  share-time snapshot of every valid work item currently present under
  `inputs/<work-type-or-BATCH>/<channel>/`. The copy happens when the share
  operation runs, so later edits to the original factory or its `inputs/`
  contents do not change an already shared recipient factory.
- `config flatten` collects the supported allowlist from `factory/scripts/**`,
  `factory/docs/**`, and supported root helper files such as `Makefile` when
  you flatten a checked-in `factory/` layout.
- `SCRIPT` entries target `factory/scripts/...`, `DOC` entries target
  `factory/docs/...`, `ROOT_HELPER` entries target supported project-root
  helper files such as `Makefile`, and `content.encoding` is `utf-8` in this
  v1 slice.
- Shared-factory starter-work copies are restored as detached recipient files.
  Recipients can inspect, edit, or run the copied files in their own
  `inputs/` tree without mutating the original author factory.
- `targetPath` must use forward slashes and must not be absolute or contain `.`
  or `..` path segments.

## Work Types

A work type describes one kind of work and every state that work can occupy:

```json
{
  "name": "story",
  "states": [
    { "name": "init", "type": "INITIAL" },
    { "name": "in-review", "type": "PROCESSING" },
    { "name": "complete", "type": "TERMINAL" },
    { "name": "failed", "type": "FAILED" }
  ]
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Stable work type name. Batch inputs use this as `work_type_name`; workstation IO uses this as `workType`. |
| `states` | Yes | State list for the work type. Each state creates one runtime place. |
| `states[].name` | Yes | Stable state name used in workstation IO. |
| `states[].type` | Yes | Lifecycle category: `INITIAL`, `PROCESSING`, `TERMINAL`, or `FAILED`. |

Use one `INITIAL` state for normal submissions. Use one `FAILED` state when you
want failed dispatches, provider failures, and cascading dependency failures to
land somewhere visible.

## Workers

A worker is the execution backend a workstation dispatches to. In
`factory.json`, the work topology needs a stable worker `name` so
`workstations[].worker` can route to that backend:

```json
{
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" }
  ]
}
```

Keep worker runtime fields, provider values, script commands, permission
settings, and split-versus-inline worker guidance in
[Workers](workers.md). This work guide only owns the fact that `workers` is a
top-level collection and that workstation routing refers to workers by name.
Runner precedence across those surfaces is explicit: workstation `runner`,
then factory `runner`, then legacy worker `modelProvider`, then the default
`codex` runner.

## Workstations

A workstation is the step that connects work topology to execution. In
`factory.json`, workstations declare which work states enable the step and
which states receive the outcome:

```json
{
  "name": "execute-story",
  "behavior": "REPEATER",
  "worker": "executor",
  "inputs": [{ "workType": "story", "state": "init" }],
  "outputs": [{ "workType": "story", "state": "in-review" }],
  "onFailure": { "workType": "story", "state": "failed" },
  "resources": [{ "name": "agent-slot", "capacity": 1 }]
}
```

Keep workstation kinds, routing fields, runtime fields, cron behavior, guards,
script-copy behavior, and split-versus-inline workstation guidance in
[Workstations](workstations.md). This work guide only owns how work states and
top-level factory routing fit together.

## Script-Backed Portability

Top-level portability dependencies belong in `supportingFiles`. Script-backed
worker commands, copied script references, inline workstation runtime fields,
and migration aliases are workstation and worker contract details. Use
[Workstations](workstations.md) for workstation-side script portability and
[Workers](workers.md) for script-worker runtime fields.

## Workstation IO

Inputs, outputs, rejection routes, failure routes, and guarded loop-breaker
routes all use the same IO shape:

```json
{ "workType": "story", "state": "in-review" }
```

| Field | Required | Description |
|-------|----------|-------------|
| `workType` | Yes | Must match a `workTypes[].name`. |
| `state` | Yes | Must match one state on that work type. |

The config validator rejects workstation IO that points to missing work types
or missing states. Input guards are workstation-specific; see
[Workstations](workstations.md) for guard fields and behavior.

## Resources

Resources limit concurrent dispatches across workstations:

```json
{
  "resources": [
    { "name": "agent-slot", "capacity": 2 }
  ],
  "workstations": [
    {
      "name": "execute",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    }
  ]
}
```

Each declared resource creates `<resource>:available` tokens equal to
`capacity`. Runtime `resources` entries consume the requested capacity while the
workstation is in flight. The runtime returns consumed resource tokens when the
dispatch completes, fails, rejects, or emits generated work.

## Guarded Loop Breakers

Use an explicit guarded `LOGICAL_MOVE` workstation to route work out of loops
when a visit threshold is reached:

```json
{
  "workstations": [
    {
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [{ "type": "VISIT_COUNT", "workstation": "review-story", "maxVisits": 3 }],
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "failed" }]
    }
  ]
}
```

Pair `REPEATER` workstations and review loops with a guarded `LOGICAL_MOVE`
workstation so work cannot cycle forever. The exact guard fields and
`LOGICAL_MOVE` workstation contract are owned by
[Workstations](workstations.md).

## Complete Example

This example accepts story work, executes it, reviews it, and allows review
feedback to route the story back for another execution pass. Guarded
`LOGICAL_MOVE` workstations cap the execution and review loops.

```json
{
  "id": "sample-service",
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
  "resources": [
    { "name": "agent-slot", "capacity": 2 }
  ],
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" },
    { "name": "loop-breaker" }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
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
      "name": "executor-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [{ "type": "VISIT_COUNT", "workstation": "execute-story", "maxVisits": 50 }],
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }]
    },
    {
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [{ "type": "VISIT_COUNT", "workstation": "review-story", "maxVisits": 3 }],
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "failed" }]
    }
  ]
}
```

The review loop breaker consumes `story:init` because `review-story` routes
rejected work back there before the loop-breaker route can fire.

At runtime:

1. The factory validates the submitted work request and creates one `story:init` token for the incoming story.
2. `execute-story` consumes that token, runs the executor, and routes success to `story:in-review`.
3. `review-story` consumes `story:in-review`. Accepted work moves to `story:complete`; rejected work routes back to `story:init`.
4. If the same story revisits `execute-story` 50 times, `executor-loop-breaker` wins the next eligible routing decision and moves the token to `story:failed`.
5. If the same story revisits `review-story` 3 times, `review-loop-breaker` consumes the rejected `story:init` token and moves it to `story:failed`.

## Authoring Checklist

- Every `workstations[].worker` matches a `workers[].name`.
- Every IO object references an existing `workType` and `state`.
- Every normal workflow path has a failure route when failure should be visible.
- Rejection routes intentionally go backward or to a review state.
- Repeater and review-loop paths have a guarded `LOGICAL_MOVE` loop breaker.
- Runtime `resources` entries reference declared resources and use positive capacity.
- New configs use `behavior` for scheduling and `type` only for runtime worker or workstation implementation.
- New configs do not depend on ignored stale fields such as `global_limits` or `worktree_cleanup`.

## Related

- [Workstations](workstations.md)
- [Workers](workers.md)
- [Resources](resources.md)
- [Batch Inputs](batch-inputs.md)
- [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
- [Templates](templates.md)
- [Author AGENTS.md](authoring-agents-md.md)
