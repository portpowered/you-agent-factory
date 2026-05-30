---
author: Agent Factory Team
last-modified: 2026-05-30
doc-id: agent-factory/reference/config
---

# Config

`you docs config` is the canonical packaged reference for `factory.json`
topology: work types, states, workers, workstations, routing, runtime
resources, and portability fields. Use `you docs work` for submitted-work
contracts (`POST /work`, tags, and batch cross-links). Use
[`docs/reference/config.md`](../../../docs/reference/config.md) for the
maintained layout guide around split `workers/`, `workstations/`, and
`inputs/` trees.

Use `factory.json` as the canonical topology file for a you-agent-factory run.
It declares the work types, states, workers, workstations, resources, and
routes that the runtime turns into a Petri-net execution graph.

## When To Use This Guide

| Need | Use |
|------|-----|
| Define `factory.json`, work types, states, top-level resources, routing, or portability fields | This guide (`you docs config`) |
| Submit work via `POST /work`, batch files, tags, or token flow after acceptance | `you docs work` |
| Place batch request files under `inputs/`, define `FACTORY_REQUEST_BATCH`, or choose relation types | `you docs batch-inputs` |
| Tune bounded concurrency pools and workstation resource requirements | `you docs resources` |
| Walk through a full setup sequence with example files and commands | `you docs authoring-factories` |

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

## Split Layout

Keep prompt-heavy runtime details beside the config when you want a readable
working tree:

```text
factory/
  factory.json
  workers/processor/AGENTS.md
  workstations/process/AGENTS.md
  inputs/task/default/
```

`factory.json` still owns the topology. Split `AGENTS.md` files own worker and
workstation runtime content such as system prompts, prompt templates, timeout
limits, and executor settings. Submitted work payloads are not part of the
topology contract; see `you docs work` and `you docs batch-inputs`.

## Top-Level Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | No | Factory-level identifier. Prompt context uses this when a submitted work item does not carry a `project` tag. |
| `inputTypes` | No | Named input kinds. The implicit `default` input type already exists; omit this unless adding a supported non-default input kind. |
| `workTypes` | Yes | Work categories and lifecycle states. Workstation input and output places must reference these names. |
| `resources` | No | Bounded concurrency pools. Workers and workstations declare requirements against these pools through their `resources` entries. |
| `supportingFiles` | No | Portability-only manifest for validation-only external tools and bundled files. This is distinct from runtime-capacity `resources`. |
| `runner` | No | Factory-level default runner ID. Supported built-ins are `codex`, `gemini`, `kiro`, `cursor-cli`, and `opencode`. |
| `workers` | Yes | Worker identities that workstations reference by `name`; see `you docs workers` for worker runtime fields. |
| `workstations` | Yes | Dispatch steps that consume input states and produce output states; see `you docs workstations` for the workstation field contract. |

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

Keep portability-only declarations under `supportingFiles`; runtime-capacity
pools still belong under `resources`. The export bundle intentionally excludes
unsupported project-root files, directories, `.gitkeep`, and temporary or
editor-swap files.

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
| `name` | Yes | Stable work type name. Submitted work uses this as `workTypeName` on `POST /work` and `works[].workTypeName` in batch requests; workstation IO uses this as `workType`. |
| `states` | Yes | State list for the work type. Each state creates one runtime place. |
| `states[].name` | Yes | Stable state name used in workstation IO. |
| `states[].type` | Yes | Lifecycle category: `INITIAL`, `PROCESSING`, `TERMINAL`, or `FAILED`. |
| `handlingBehavior` | No | Optional CLI routing markers for this work type. Use `["DEFAULT"]` on exactly one work type when customers should run one-shot prompts with `you run --factory`. |

Use one `INITIAL` state for normal submissions. Use one `FAILED` state when you
want failed dispatches, provider failures, and cascading dependency failures to
land somewhere visible.

### Default handling for one-shot CLI runs

Mark exactly one work type with `handlingBehavior: ["DEFAULT"]` when you want
customers to submit a single raw-text prompt through the simplified CLI:

```json
{
  "name": "task",
  "handlingBehavior": ["DEFAULT"],
  "states": [
    { "name": "init", "type": "INITIAL" },
    { "name": "complete", "type": "TERMINAL" },
    { "name": "failed", "type": "FAILED" }
  ]
}
```

Validation rules:

- A factory may declare `DEFAULT` on at most one work type. More than one
  `DEFAULT` work type is rejected at config load time.
- Factories used with `you run --factory <factory.json> <prompt>` must declare
  `DEFAULT` on exactly one work type. Omitting `handlingBehavior` on every work
  type fails fast for that command.
- Factories that only use `--dir`, watched `inputs/`, or `--work` batch files do
  not need `handlingBehavior` unless you also want the simplified prompt path.

When `DEFAULT` is set, `you run --factory` submits the positional prompt as raw
text to that work type (equivalent to a single Markdown inbox file) and exits
after batch idle completion. See `you docs authoring-factories` for the
copy-pasteable command form.

## Workers

A worker is the execution backend a workstation dispatches to. In
`factory.json`, the topology needs a stable worker `name` so
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
settings, and split-versus-inline worker guidance in `you docs workers`. This
config guide only owns the fact that `workers` is a top-level collection and
that workstation routing refers to workers by name. Runner precedence is
explicit: workstation `runner`, then factory `runner`, then legacy worker
`modelProvider`, then the default `codex` runner.

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
`you docs workstations`. This config guide only owns how work states and
top-level factory routing fit together.

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
or missing states. Input guards are workstation-specific; see `you docs guards`
for guard types and attachment levels and `you docs workstations` for
workstation fields that carry them.

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
`LOGICAL_MOVE` workstation contract are owned by `you docs workstations`.

## How The Pieces Fit

Work enters the factory as a token in a work type's initial state after a
submission is accepted. A workstation is enabled when its configured input
places have matching tokens. The workstation dispatches to its worker, then
routes the token based on the worker outcome:

| Worker outcome | Routing field |
|----------------|---------------|
| Accepted | `outputs` |
| Continue | `onContinue` |
| Rejected | `onRejection` |
| Failed, timed out, or errored | `onFailure` |

Each `workType` and `state` pair becomes a place named `<workType>:<state>`,
such as `task:init`. See `you docs work` for how submitted payloads become
those tokens.

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

## Run Controls

`you run` supports optional factory selection, one-shot prompts, mock workers,
and record/replay flags:

- `--factory <factory.json>` — load a portable `factory.json` by file path and,
  with a trailing positional prompt, submit raw text to the work type that
  declares `handlingBehavior: ["DEFAULT"]` (see Default handling for one-shot CLI runs)
- `--with-mock-workers` — deterministic worker outcomes without live provider calls
- `--record`, `--replay`, `--no-record` — control replay artifact capture and playback

Example one-shot run:

```bash
you run --factory ./factory.json "Fix the lint issues"
```

`--factory` cannot be combined with `--dir`. Use `--dir` for the traditional
factory-directory layout and inbox workflows.

Canonical guides: `you docs mock-workers` and `you docs record-replay`. For an
end-to-end authoring walkthrough and reusable files under `docs/examples/`, use
`you docs authoring-factories`.

## Authoring Rules

- Use camelCase public fields such as `workTypes`, `modelProvider`,
  `executorProvider`, `stopWords`, and `maxExecutionTime`.
- Use `behavior` for workstation scheduling behavior and `type` for runtime
  implementation details.
- Runner precedence is workstation `runner`, then factory `runner`, then
  legacy worker `modelProvider`, then the default `codex` runner.
- Built-in runner selection expects the corresponding local CLI and auth/setup
  to already be available before execution starts.
- Keep guarded `LOGICAL_MOVE` workstations explicit instead of relying on
  retired top-level loop-breaking fields. See `you docs guards` for guard
  types and loop-breaker patterns.
- Prefer split `AGENTS.md` files for long prompts and inline runtime fields for
  portable or recorded single-file configs.

## Related

- `you docs work`
- `you docs mock-workers`
- `you docs record-replay`
- `you docs guards`
- `you docs relationships`
- `you docs authoring-factories`
- `you docs workstations`
- `you docs workers`
- `you docs resources`
- `you docs batch-inputs`
- `you docs templates`
