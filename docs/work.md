author: Agent Factory Team
---
last-modified: 2026-04-21
doc-id: agent-factory/work
---

# Factory JSON And Work Configuration

`factory.json` declares the workflow topology for an Agent Factory run. It
defines the work types, states, workers, workstations, resources, and routing
behavior that the runtime turns into a Petri-net execution model.

Use this guide when writing or reviewing `factory.json`. For the JSON file you
drop into `inputs/<workType>/...`, see [Batch Inputs](guides/batch-inputs.md).

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

For the canonical watched-file and API request shape, minimum-field reference,
and submitted `PARENT_CHILD` example, use
[Batch Inputs](guides/batch-inputs.md). The overview below is intentionally
summary-only.

### Field Reference for structured schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `request_id` | string | yes | Stable ID for idempotent batch submission and request history. |
| `type` | string | yes | Must be `FACTORY_REQUEST_BATCH`. |
| `works` | array of work items | yes | At least one work item. Each entry becomes one engine token. |
| `works[].work_type_name` | string | usually | The configured work type name. Must match a `workTypes[].name` in `factory.json`; file inputs can infer it from the watched input folder when omitted. |
| `works[].name` | string | yes | A unique name within this batch. Used in `relations` to declare dependencies and as the token's display name. |
| `works[].payload` | object, string, array, or scalar | no | Optional payload for the work item. |
| `works[].tags` | `map[string]string` | no | Optional tags for this work item. Available in prompt templates via `{{ index (index .Inputs 0).Tags "key" }}` and in parameterized workstation fields (see [workstations.md](workstations.md)). |
| `relations` | array of relations | no | Dependency edges between work items. |
| `relations[].type` | string | yes | Relation type. Use `"DEPENDS_ON"` for sibling prerequisites or `"PARENT_CHILD"` for submitted parent-child membership. |
| `relations[].source_work_name` | string | yes | The blocked work item for `DEPENDS_ON`, or the child work item for `PARENT_CHILD`. Must match a `works[].name`. |
| `relations[].target_work_name` | string | yes | The prerequisite work item for `DEPENDS_ON`, or the parent work item for `PARENT_CHILD`. Must match a `works[].name`. |
| `relations[].required_state` | string | no | Required target state before the dependent work can run. Defaults to `complete` for `DEPENDS_ON`. Ignore this field for `PARENT_CHILD`. |

### Validation Rules

The factory validates the payload before creating any tokens. If validation fails, no tokens are created (atomic submission).

1. `type` must be `FACTORY_REQUEST_BATCH`.
2. `request_id` must be present.
3. The `works` array must contain at least one item.
4. Every work item must have `name` and a resolvable `work_type_name`.
5. Work item names must be unique within the batch.
6. Work item types must match a declared `workTypes[].name` in `factory.json`.
7. Relation types must be supported (`DEPENDS_ON` and `PARENT_CHILD`).
8. Relation source and target names must reference existing work items.
9. Self-referencing dependencies are rejected.

Invalid payloads are rejected and logged. No partial tokens are created.

### Tracking and submitting work in a submission

Tags declared on each work item let you track work at each submitted workstation.

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
| `workers` | Yes | Worker identities and optional inline worker runtime config. Workstations reference workers by `name`. |
| `workstations` | Yes | Dispatch steps that consume input states, invoke workers or logical routing, and produce output states. |

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
- `config flatten` collects the supported allowlist from `factory/scripts/**`,
  `factory/docs/**`, and supported root helper files such as `Makefile` when
  you flatten a checked-in `factory/` layout.
- `SCRIPT` entries target `factory/scripts/...`, `DOC` entries target
  `factory/docs/...`, `ROOT_HELPER` entries target supported project-root
  helper files such as `Makefile`, and `content.encoding` is `utf-8` in this
  v1 slice.
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

A worker is the execution backend a workstation dispatches to. The `workers`
entry can be just a name when runtime details live in `workers/<name>/AGENTS.md`:

```json
{
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" }
  ]
}
```

You can also inline worker runtime fields in `factory.json` for portable
single-file configs:

```json
{
  "name": "lint",
  "type": "SCRIPT_WORKER",
  "command": "go",
  "args": ["test", "./..."],
  "timeout": "10m"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Worker identity referenced by `workstations[].worker`. |
| `type` | Split or inline runtime config | `MODEL_WORKER` or `SCRIPT_WORKER`. Required in inline worker config or worker `AGENTS.md`. |
| `model` | Model workers | Provider model name. |
| `modelProvider` | Model workers | Model-provider identifier used in diagnostics and model routing. Built-in values are `CLAUDE` and `CODEX`. |
| `executorProvider` | Model workers | Executor adapter identifier used to choose the worker execution wrapper, for example `SCRIPT_WRAP` in local default scaffolds. |
| `command` | Script workers | Executable name for `SCRIPT_WORKER`. |
| `args` | Script workers | Command arguments. Values can use prompt-template variables. |
| `timeout` | No | Go duration such as `10m` or `1h`. |
| `stopToken` | No | Model-output token that marks accepted completion when configured. |
| `skipPermissions` | No | Provider-specific permission shortcut used by supported providers. |

Prefer split `AGENTS.md` files for long model instructions. Prefer inline
worker fields for generated, recorded, or single-file factory configs.
The canonical source of truth for worker-contract values is the `Worker` schema
in [`api/openapi.yaml`](../api/openapi.yaml). Current built-in
`modelProvider` values are `CLAUDE` and `CODEX`, and the current public
`executorProvider` value is `SCRIPT_WRAP`.

## Workstations

A workstation is the step that connects topology to execution:

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

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Stable workstation and transition name. |
| `behavior` | No | Scheduling behavior: `STANDARD`, `REPEATER`, or `CRON`. Defaults to `STANDARD`. |
| `worker` | Usually | Worker name. Required for model or script dispatch and for cron workstations. Omit only for `LOGICAL_MOVE` runtime workstations. |
| `inputs` | Usually | Work or resource places that enable the workstation. Cron workstations may omit customer inputs but still consume internal time work. |
| `outputs` | Usually | Places produced when the worker accepts. Cron workstations require at least one output. |
| `onRejection` | No | Place produced when the worker rejects. |
| `onFailure` | Recommended | Place produced when the worker fails or times out. |
| `resources` | No | Resource capacity consumed while this workstation runs. |
| `copyReferencedScripts` | No | When `true`, `agent-factory config expand` copies supported referenced script files for this workstation's bound `SCRIPT_WORKER`. Omit it or set `false` to keep script references external. |
| `guards` | No | Workstation-level `VISIT_COUNT` guards. Parent fan-in belongs on per-input guards. |
| `CRON` | Cron only | Trigger timing for `behavior: "CRON"`. |

Runtime fields such as `type`, `promptFile`, `promptTemplate`,
`limits.maxExecutionTime`, `stopWords`, `workingDirectory`, `worktree`, and
`env` can live either inline on the
workstation entry or in `workstations/<name>/AGENTS.md`. See
[Workstations](workstations.md) for the full workstation guide.

## Config Portability For Script-Backed Layouts

`agent-factory config flatten` supports script-backed workstations without a
split `workstations/<name>/AGENTS.md` file when the workstation already
declares inline runtime fields in `factory.json`. Keep at least one runtime
field inline, such as `type: "MODEL_WORKSTATION"`, so the flattened config
still carries a complete standalone workstation definition.

Use `copyReferencedScripts` on the workstation when `agent-factory config
expand` should materialize supported relative script files into the expanded
layout. When the field is omitted or `false`, expand leaves those script files
external and only writes the split config files.

Portable script-backed example:

```json
{
  "workers": [
    {
      "name": "workspace-setup",
      "type": "SCRIPT_WORKER",
      "command": "python",
      "args": ["scripts/setup-workspace.py", "--mode", "portable"]
    }
  ],
  "workstations": [
    {
      "name": "setup-workspace",
      "type": "MODEL_WORKSTATION",
      "worker": "workspace-setup",
      "copyReferencedScripts": true,
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }]
    }
  ]
}
```

With that shape:

1. `agent-factory config flatten ./factory` succeeds even if
   `workstations/setup-workspace/AGENTS.md` does not exist.
2. The flattened JSON keeps the inline workstation runtime fields and the
   script-worker command metadata needed for a later expand.
3. `agent-factory config expand ./factory.json` copies
   `scripts/setup-workspace.py` into the expanded layout only because
   `copyReferencedScripts` is explicitly `true`.

Only supported factory-bundle-relative script references are copied. The
current expand path accepts either a relative script `command` or the first
non-flag script argument for interpreter-style commands such as `python`,
`powershell`, `bash`, `node`, and `bun`. Absolute paths and `..`-escaping
paths are rejected instead of being rewritten.

Legacy workstation `timeout`, singular workstation stop aliases, and retired
workstation resource aliases are accepted only to load older factories.
Canonical flattening, replay serialization, and current docs rewrite those
inputs to `limits.maxExecutionTime`, `stopWords`, and `resources`.

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
| `guards` | Inputs only | Parent-aware fan-in guards for this input. The current mapper uses the first guard entry. |

The config validator rejects workstation IO that points to missing work types
or missing states.

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

Use an explicit guarded `LOGICAL_MOVE` workstation to cap loops:

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

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Must be `LOGICAL_MOVE` for a no-worker loop-breaker route. |
| `guards[].type` | Yes | Use `VISIT_COUNT` to gate the loop-breaker route. |
| `guards[].workstation` | Yes | Workstation whose visits are counted. |
| `guards[].maxVisits` | Yes | Positive visit threshold. |
| `inputs` | Yes | Place to consume from when the threshold is exceeded. |
| `outputs` | Yes | Place to move work into. |

Pair `REPEATER` workstations and review loops with a guarded `LOGICAL_MOVE`
workstation so work cannot cycle forever.

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
- [Batch Inputs](guides/batch-inputs.md)
- [Parent-Aware Fan-In](guides/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
- [Prompt Template Variables](prompt-variables.md)
- [Author AGENTS.md](authoring-agents-md.md)
