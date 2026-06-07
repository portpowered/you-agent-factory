# Config

`you docs config` is the canonical packaged guide for `factory.json` topology:
work types, states, workers, workstations, resources, and routing.

`factory.json` declares the workflow topology for a you-agent-factory run. It
defines the work types, states, workers, workstations, resources, and routing
behavior that the runtime turns into a Petri-net execution model.

Use this page when you need the canonical factory directory layout, the
field-by-field `factory.json` topology contract, and where each authored file
lives. Use `you docs work` for `POST /factory-sessions/{session_id}/work`, tags, and batch
cross-links. For live session inspection (`you session list`, `you factory query`,
status API fields, and `--server` / `--session` on HTTP client commands), see
`you docs sessions`.

## Current Contract

- `factory.json` is the canonical root file. It owns factory-level workflow
  topology such as `id`, `workTypes`, `workers`, `workstations`, routes,
  optional runtime `resources`, non-executable portable `layout`, and the optional portability
  `supportingFiles`; the normative field contract lives on this page.
- Keep worker runtime instructions in `workers/<name>/AGENTS.md`.
- Keep workstation runtime instructions in `workstations/<name>/AGENTS.md`.
- Keep watched work inputs under `inputs/<work-type-or-BATCH>/<channel>/`.
- Inline runtime fields in `factory.json` are still supported for portable
  single-file configs, but the split layout is the recommended authoring path.
- **Live saves** (dashboard graph editor, `PUT /factory-sessions/{id}/factory`,
  import replace-current, `you factory save <name>`, and named-factory upsert)
  always persist the split layout: a thin `factory.json` plus
  `workers/<name>/`, `workstations/<name>/`, bundled files, and default
  `inputs/` channels when definitions exist. Runtime `body` and
  `promptTemplate` values materialized under split dirs are omitted from the
  on-disk `factory.json`.
- **Portable export** (`config flatten`, PNG export) is the explicit opt-in path
  for a single-file `factory.json` that inlines runtime bodies and bundled
  content. That inline-only shape is not what session/API saves write by default.
- Use factory-level `runner` in `factory.json` to set the default runner for
  the factory. Supported built-in runner IDs are `codex`, `gemini`, `kiro`,
  `cursor-cli`, and `opencode`.
- When both inline runtime fields and a split `AGENTS.md` file exist for the
  same workstation, the split runtime definition is authoritative for the
  overlapping runtime fields.
- Treat `supportingFiles` as a portability-only contract: `requiredTools`
  declare validation-only PATH dependencies, while `bundledFiles` carry
  portable file content for factory-relative restoration.

## What Lives Where

```text
factory/
  factory.json
  workers/
    processor/AGENTS.md
  workstations/
    process/AGENTS.md
  inputs/
    task/default/request.json
```

## Minimal Layout

- Put the topology in `factory.json`.
- Put the worker instructions in `workers/processor/AGENTS.md`.
- Put the workstation prompt or runtime instructions in
  `workstations/process/AGENTS.md`.
- Drop watched single-work-type requests under `inputs/task/default/`.
- Drop mixed-work-type or relation-heavy batch files under
  `inputs/BATCH/default/`.

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
| `layout` | No | Non-executable portable graph-editor layout metadata keyed by canonical graph node and edge ids. It must not change runtime topology or execution behavior. |
| `supportingFiles` | No | Portability-only manifest for validation-only external tools and bundled files. This is distinct from runtime-capacity `resources`. |
| `runner` | No | Factory-level default runner ID. Supported built-ins are `codex`, `gemini`, `kiro`, `cursor-cli`, and `opencode`. |
| `workers` | Yes | Worker identities that workstations reference by `name`; see `you docs workers` for worker runtime fields. |
| `workstations` | Yes | Dispatch steps that consume input states and produce output states; see `you docs workstations` for the workstation field contract. |

Do not rely on stale top-level `global_limits` or `exhaustionRules` examples.
The current public `factory.json` authoring contract uses guarded
`LOGICAL_MOVE` workstations and workstation limits for user-configured safety
behavior.

## Portable Graph Layout

Use top-level `layout` when a shared factory should carry authored graph-canvas
geometry across sessions or exports. Layout metadata is presentation-only: the
runtime ignores it when building the executable factory topology.

Omit `layout` entirely when no authored canvas state is needed. Missing layout is
a valid factory state and does not affect runtime execution.

```json
{
  "layout": {
    "schemaVersion": 1,
    "nodes": [
      {
        "id": "workstation:review",
        "position": { "x": 420, "y": 180 },
        "size": { "width": 156, "height": 196 },
        "locked": false
      }
    ],
    "edges": [
      {
        "id": "workstation-output:workstation:review->work-state:task:done",
        "waypoints": [{ "x": 540, "y": 220 }],
        "labelPosition": { "x": 590, "y": 204 }
      }
    ],
    "groups": [
      {
        "id": "review-lane",
        "label": "Review",
        "bounds": { "x": 360, "y": 120, "width": 520, "height": 360 },
        "nodeIds": ["workstation:review"],
        "parentGroupId": null,
        "color": "blue",
        "locked": false
      }
    ],
    "viewport": { "x": 0, "y": 0, "zoom": 1 },
    "preferences": { "direction": "RIGHT" }
  }
}
```

### Field contract

| Field | Required when `layout` is present | Description |
|-------|-----------------------------------|-------------|
| `schemaVersion` | Yes | Portable layout contract version. The shipped contract is `1`. |
| `nodes` | No | Authored node geometry keyed by canonical graph node ids. |
| `edges` | No | Authored edge waypoints and label positions keyed by canonical graph edge ids. |
| `groups` | No | Flat background groups with bounds and member node ids. |
| `viewport` | Yes | Authored camera position (`x`, `y`, `zoom`). |
| `preferences` | No | Portable display defaults. |

#### `layout.nodes[]`

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Canonical graph node id such as `workstation:<workstationId>`. |
| `position` | Yes | Authored node origin in graph canvas space (`x`, `y`). |
| `size` | No | Authored node dimensions (`width`, `height`). |
| `locked` | No | Optional authored lock flag for future editor affordances. |

#### `layout.edges[]`

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Canonical graph edge id such as `workstation-output:<sourceGraphNodeId>-><targetGraphNodeId>`. |
| `waypoints` | No | Optional intermediate edge points in graph canvas space. |
| `labelPosition` | No | Optional authored label anchor in graph canvas space. |

#### `layout.groups[]`

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Stable authored group id. |
| `bounds` | Yes | Group rectangle (`x`, `y`, `width`, `height`) in graph canvas space. |
| `nodeIds` | Yes | Canonical graph node ids visually contained by the group. May be empty after pruning. |
| `label` | No | Optional visible group label. |
| `parentGroupId` | No | Reserved for future nesting. Omit or set to `null` for flat groups in v1. |
| `color` | No | Optional authored accent or fill color. |
| `locked` | No | Optional authored lock flag for future editor affordances. |

#### `layout.viewport`

| Field | Required | Description |
|-------|----------|-------------|
| `x` | Yes | Authored viewport horizontal offset. |
| `y` | Yes | Authored viewport vertical offset. |
| `zoom` | Yes | Authored viewport zoom factor. |

#### `layout.preferences`

| Field | Required | Description |
|-------|----------|-------------|
| `direction` | No | Preferred authored graph direction: `UP`, `DOWN`, `LEFT`, or `RIGHT`. |

`layout.nodes[].id` and `layout.edges[].id` must use canonical graph ids such as
`workstation:<workstationId>` or
`workstation-output:<sourceGraphNodeId>-><targetGraphNodeId>`. Legacy simplified
edge ids are treated as stale layout references during validation and save.

`layout.preferences` is intentionally narrow. Keep only portable display defaults
that do not hide nodes, rewrite topology, or change execution.

### Round-trip and portability

Layout round-trips through current-factory GET/PUT, backend load/save,
named-factory create/replace, activation, import/export, and
`you config flatten` / `you config expand` when topology is unchanged.

### Validation: blocking topology vs recoverable layout

Factory validation builds pending graph topology indexes from the factory graph,
then evaluates layout only after those indexes exist (`validation.Validate` in
`pkg/factory/validation`).

**Blocking topology validation** covers structural factory defects such as
dangling references, missing outcome routes, and invalid work-type completion
paths. These targets block editable saves, CLI persist pre-checks, and runtime
load when the factory graph itself is invalid.

**Recoverable layout validation** covers presentation metadata that does not
change execution semantics. Layout defects use `factory.layout.*` target codes
with warning severity. They do not make `HasBlockingTargets()` true, so an
otherwise valid factory can still save and load for runtime use.

Recoverable layout cases include:

- **Missing layout** — valid; runtime ignores absent layout metadata.
- **Unsupported `schemaVersion`** — `factory.layout.unsupportedSchemaVersion`;
  layout still loads when structurally valid.
- **Stale node, edge, or group member references** —
  `factory.layout.unknownNodeReference`, `factory.layout.unknownEdgeReference`,
  and `factory.layout.unknownGroupMemberReference`; references that do not match
  pending graph ids are warnings, not topology failures.
- **Non-finite geometry** — `factory.layout.invalidGeometry`; NaN or infinite
  coordinates are warnings during validation.
- **Malformed layout JSON** — boundary validation rejects structurally invalid
  layout on editable OpenAPI payloads. Runtime load
  (`expandFactoryConfigForRuntimeLoad`) can strip malformed top-level `layout`
  and continue when the remaining factory topology is valid.

### Save-time pruning and `layoutOutcomes`

Save paths (`PrepareFactoryLayoutPayload`, editable `PUT`, named-factory upsert,
`you factory save`) prune stale layout references against the pending topology
before persist:

- Remove `layout.nodes[]` entries whose ids are absent from the pending node
  index.
- Remove `layout.edges[]` entries whose ids are absent from the pending edge
  index.
- Remove unknown node ids from each `layout.groups[].nodeIds`.
- Keep groups with empty `nodeIds` unless the author deletes the group
  explicitly.
- Reject entries with non-finite geometry rather than persisting them.

Pruning and rejection outcomes are returned on save responses as ephemeral
`layoutOutcomes` (structured validation targets). `layoutOutcomes` is omitted
from persisted `factory.json`, ignored on save requests, and not returned on
subsequent GETs.

| Code | Recoverable | Meaning |
|------|-------------|---------|
| `factory.layout.unsupportedSchemaVersion` | Yes | `schemaVersion` is not the supported contract (`1`). |
| `factory.layout.unknownNodeReference` | Yes | `layout.nodes[].id` does not match a pending graph node. |
| `factory.layout.unknownEdgeReference` | Yes | `layout.edges[].id` does not match a pending graph edge. |
| `factory.layout.unknownGroupMemberReference` | Yes | `layout.groups[].nodeIds[]` references an unknown graph node. |
| `factory.layout.invalidGeometry` | Yes | Layout coordinates contain NaN or infinity. |

## Portability Resource Manifest

Use `supportingFiles` in `factory.json` when the portable factory must declare
external tools or carry bundled helper files beyond workflow topology.

In v1 shared-factory flows, that same portability manifest also carries starter
work copied from the source factory's live `inputs/` tree. Sharing snapshots
every valid work item present under `inputs/<work-type-or-BATCH>/<channel>/` at
the moment the share operation runs, including the case where the directory is
empty.

That share-time copy is detached after the recipient factory is created:

- Later edits to the original factory or its `inputs/` files do not retroactively
  update earlier shared copies.
- Recipient edits inside the copied factory's `inputs/` tree do not mutate the
  original author factory.

Example shared-factory starter work:

```text
source factory before share
  inputs/
    task/default/customer-bug.md
    BATCH/default/release-sweep.json

shared recipient after import or create
  inputs/
    task/default/customer-bug.md
    BATCH/default/release-sweep.json
```

The recipient copy is ready to inspect or run immediately, but it is no longer
live-linked to the source factory.

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
- `config flatten` collects the supported allowlist from `factory/scripts/**`,
  `factory/docs/**`, and supported root helper files such as `Makefile` when
  you flatten a checked-in `factory/` layout.
- `targetPath` must use forward slashes and must not be absolute or contain `.`
  or `..` path segments.

## Work Types

A work type describes one kind of work and every state that work can occupy.
Submitted work references `workTypes[].name` as `workTypeName`. See
`you docs work` for API and batch submission fields.

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Stable work type name used in workstation IO and submitted work. |
| `states` | Yes | State list for the work type. Each state creates one runtime place. |
| `states[].name` | Yes | Stable state name used in workstation IO. |
| `states[].type` | Yes | Lifecycle category: `INITIAL`, `PROCESSING`, `TERMINAL`, or `FAILED`. |
| `handlingBehavior` | No | Optional CLI routing markers. Use `["DEFAULT"]` on exactly one work type for `you run --factory`. |

### Default Handling For One-Shot CLI Runs

Mark exactly one work type with `handlingBehavior: ["DEFAULT"]` when you want
customers to submit a single raw-text prompt through the simplified CLI.
Validation rejects more than one `DEFAULT` work type. Factories used with
`you run --factory <factory.json> <prompt>` must declare `DEFAULT` on exactly
one work type.

## Invocation Contract

Factory invocation is the shared one-shot contract used by:

- `you run --factory <factory.json> <text>`
- `cat request.txt | you run --factory <factory.json>`
- `POST /factory-sessions/{session_id}/invocations`

CLI and API invocations resolve one canonical input and one canonical
`primaryResult`. The runtime does not treat CLI prompt runs as a separate
submission mode with different return semantics.

### Input sources

The current invocation slice is text-first:

| Surface | Supported source now | Notes |
|---------|----------------------|-------|
| CLI | Trailing positional text or non-TTY stdin | Supplying both is rejected with `INVOCATION_INPUT_SOURCE_CONFLICT`. Empty selected stdin is rejected with `INVOCATION_INPUT_EMPTY`. |
| API | Top-level `sourceKind: "text"` plus canonical `content` (`WorkContent`) | `fileRef` and `audioStream` are reserved future source categories and are not accepted yet. |

Use `you docs sessions` for the session-scoped invocation API examples. Reserve
future source categories in authored configs and client code, but do not imply
they are implemented today.

### `invocationReturn`

`Factory.invocationReturn` is the factory-authored policy that tells CLI and API
invocations which completed work content should be returned as the
`primaryResult`.

| Field | Required | Description |
|-------|----------|-------------|
| `policy` | Yes when `invocationReturn` is present | `SUBMITTED_WORK_TERMINAL` or `EXPLICIT`. |
| `workTypeName` | Required for `EXPLICIT` | Work type name to select from the current invocation scope. |
| `terminalState` | Required for `EXPLICIT` | Terminal state name that must be reached for the selected work. |
| `workName` | No | Optional authored work name filter when multiple scoped work items share the same type and terminal state. |

When `invocationReturn` is omitted, runtimes use the documented
`SUBMITTED_WORK_TERMINAL` fallback. That fallback traces the work item
originally submitted by the invocation until it reaches its first terminal
output, then returns that work content as the `primaryResult`.

Example using the default fallback by omission:

```json
{
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "queued", "type": "INITIAL" },
        { "name": "done", "type": "TERMINAL" }
      ],
      "handlingBehavior": ["DEFAULT"]
    }
  ]
}
```

Example with an explicit primary-result selector:

```json
{
  "invocationReturn": {
    "policy": "EXPLICIT",
    "workTypeName": "report",
    "terminalState": "finalized",
    "workName": "summary"
  }
}
```

Use `EXPLICIT` when the invocation should return a downstream derived result
instead of the first terminal content for the originally submitted work item.
If the configured selector cannot resolve a result in the current invocation
scope, the invocation fails with `INVOCATION_PRIMARY_RESULT_UNRESOLVED` and no
success payload.

## Workstation IO

Workstation inputs, outputs, rejection routes, failure routes, and guarded
loop-breaker routes use `{ "workType": "<name>", "state": "<name>" }`. See
`you docs workstations` and `you docs guards` for field-level
contracts.

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
workstation is in flight. See `you docs resources` for typed model pools
and `you docs workers` for worker-side requirement metadata.

## Topology Authoring Checklist

- Every `workstations[].worker` matches a `workers[].name`.
- Every IO object references an existing `workType` and `state`.
- Every normal workflow path has a failure route when failure should be visible.
- Repeater and review-loop paths have a guarded `LOGICAL_MOVE` loop breaker.
- Runtime `resources` entries reference declared resources and use positive capacity.

## Bootstrap Checklist

- Start with `factory.json`, then add split `AGENTS.md` files for any
  prompt-heavy or runtime-heavy worker and workstation definitions.
- Keep one directory per worker or workstation so the runtime can resolve
  `workers/<name>/AGENTS.md` and `workstations/<name>/AGENTS.md` by the names
  used in `factory.json`.
- Use camelCase public config fields in `factory.json`; do not author new
  configs with retired snake_case aliases.
- Runner precedence is explicit: workstation `runner` override first, then
  factory `runner`, then worker `modelProvider` compatibility when no explicit runner is set, then the
  default `codex` runner.
- Validate runner prerequisites before execution. Built-in runner selection
  expects the corresponding local CLI on `PATH`, and runner-specific auth or
  local setup must already be in place.
- Keep portability-only declarations under `supportingFiles`; do not overload
  runtime-capacity `resources` with bundled files or external tool checks.
- Treat `inputs/` as submission data, not as part of the topology. The runtime
  watches the path and turns those files into work requests.

## Live Saves Vs Portable Export

Session and API factory writes share one on-disk persist pipeline. Whether you
save the default session factory (`REPLACE_CURRENT` on the root-as-factory
layout) or a named factory (`UPSERT_NAMED_AND_ACTIVATE` or named
`REPLACE_CURRENT`), the server normalizes the submitted factory JSON, runs
pre-save topology checks on that normalized view, writes a thin `factory.json`,
expands `workers/` and `workstations/` runtime files, validates the staged tree with `LoadRuntimeConfig`, and commits atomically.
Pre-save topology checks use the same normalized factory view as the split write;
full alignment with validate-only `POST /factory-validations` is tracked in the
**Factory validation convergence** maintainer PRD (`prd-factory-validation-convergence`).
The post-persist `LoadRuntimeConfig` gate remains part of this split-layout persist
contract.

| Path | On-disk result |
|------|----------------|
| Dashboard graph editor save | Split layout under the session factory root |
| `PUT /factory-sessions/{id}/factory` | Same split layout as dashboard save |
| Import replace-current / create-named | Same split layout for equivalent content |
| `you factory save <name>` | Split layout under the named factory directory |
| `config flatten` / PNG portable export | Opt-in single-file `factory.json` with inlined runtime and bundled content |

`GET` current-factory responses may still return a fully inlined `Factory` for
editing even though live saves persist split files on disk. Clients continue to
send inlined JSON; the split layout is a server-side persist detail.

### Upgrading From Monolithic-Only `factory.json`

Older deployments may have only a monolithic `factory.json` at the session
factory root (inline `body` / `promptTemplate` with no `workers/` or
`workstations/` trees). The first live save after upgrade runs the same split
expansion as named-factory persist: expect a one-time diff that adds
`workers/<name>/` and `workstations/<name>/` directories, thins `factory.json`,
and prunes stale split dirs for entities removed from the submitted config.
Subsequent saves refresh split files in place without re-inflating
`factory.json`.

## Portable Bundled Files

Use this contract when you want a canonical portable `factory.json` to collect,
carry, and restore supporting files across `config flatten`, `config expand`,
and `LoadRuntimeConfig(...)` without redefining the manifest shape.

- Checked-in files under `factory/docs/**` are bundled by default. `config
  flatten` and the first load of a split layout discover those docs and add them
  to `supportingFiles.bundledFiles` when the manifest does not already list DOC
  entries.
- After a factory lists DOC entries in `supportingFiles.bundledFiles`, the
  manifest is authoritative for dashboard/API saves: create, rename, edit, and
  delete operations round-trip through `PUT /factory-sessions/{id}/factory` and
  update the saved current-factory document version without requiring a full page
  reload.
- Implicit default bundling (checked-in docs discovered on disk) is separate
  from explicit dashboard edits to bundled docs. Dashboard edits persist DOC
  entries in the manifest and materialize or prune the matching files under
  `factory/docs/**` on save.
- `config flatten` adds supported `factory/scripts/**`, `factory/docs/**`, and
  root helper files such as `Makefile` to
  `supportingFiles.bundledFiles` automatically for checked-in `factory/`
  layouts.
- `config expand` restores bundled files onto disk beside the expanded
  `factory.json`, `workers/**/AGENTS.md`, and `workstations/**/AGENTS.md`
  layout.
- `LoadRuntimeConfig(...)` materializes bundled files before it returns when it
  loads a standalone portable `factory.json`, so script-backed workers can use
  the restored files without a separate expand step.
- Restored `type: "SCRIPT"` entries are written with executable permissions on
  Unix-like systems so direct-exec script paths remain runnable after a portable
  roundtrip.
- Invalid bundled-file targets are rejected before any file is written. That
  includes absolute paths, escaping paths, and target trees that escape through
  pre-existing symlinks or Windows junctions.
- Keep bundled-file examples on the canonical `targetPath` contract such as
  `Makefile`, `factory/scripts/setup-workspace.py`, and `factory/docs/usage.md`.

This bundle slice is intentionally narrow. `config flatten` does not recurse
through arbitrary project files outside the documented allowlist.

## Run Controls

`you run` supports optional factory selection, one-shot prompts, mock workers,
and record/replay flags:

- `--factory <factory.json>` — load a portable `factory.json` by file path and,
  with a trailing positional prompt, submit raw text to the work type that
  declares `handlingBehavior: ["DEFAULT"]` (see [Default handling for one-shot CLI runs](#default-handling-for-one-shot-cli-runs))
- `--with-mock-workers` — deterministic worker outcomes without live provider calls
- `--record`, `--replay`, `--no-record` — control replay artifact capture and playback

Example one-shot run:

```bash
you run --factory ./factory.json "Fix the lint issues"
```

`--factory` cannot be combined with `--dir`. Use `--dir` for the traditional
factory-directory layout and inbox workflows.

### Clean invocation contract for `you run --factory`

One-shot batch `you run --factory` invocations enter a clean invocation mode
when they submit prompt or work input and are not `--continuously`. This mode
is intended for shell pipelines and other result-oriented automation.

Operator-oriented runs keep the existing startup behavior:

- `you` with no args
- `you run --continuously`
- `you run --dir ...` without factory prompt submission

#### Input source rules

Clean invocation accepts exactly one primary input source per invocation:

- a trailing positional prompt
- stdin via `-`
- piped non-TTY stdin
- `--work <path>` for a canonical batch file

Do not combine multiple payload sources in the same invocation. If stdin and a
non-empty positional prompt are both present, the command exits non-zero before
runtime startup and writes a stable stderr error with code
`INVOCATION_INPUT_SOURCE_CONFLICT` naming the conflicting sources
(`positional_text`, `stdin_text`).

Example ambiguity failure:

```bash
printf 'from stdin' | you run --factory ./factory.json "from arg"
```

Text stderr:

```text
INVOCATION_INPUT_SOURCE_CONFLICT: invocation input sources conflict: positional_text, stdin_text
```

JSON stderr with global `--json`:

```json
{"code":"INVOCATION_INPUT_SOURCE_CONFLICT","message":"invocation input sources conflict: positional_text, stdin_text"}
```

#### Success stdout contract

On success, stdout carries only the primary result from the sole work type that
declares `handlingBehavior: ["DEFAULT"]`.

- Default text mode writes the result body only.
- Global `--json` writes exactly one JSON object to stdout.
- Clean invocation never adds startup banners, dashboard URLs, runtime-log
  paths, simple-dashboard snapshots, or recording-path notices to stdout.

Text success example:

```bash
you run --factory ./factory.json "Summarize the changelog" > result.txt
```

JSON success example for text positional or stdin invocation:

```bash
you --json run --factory ./factory.json "Summarize the changelog"
```

```json
{"requestId":"req-123","traceId":"trace-123","status":"COMPLETED","primaryResult":[{"type":"text","text":"Summary text"}]}
```

`--work` clean invocations still emit the work-target JSON envelope
`{"output":"...","workId":"...","workTypeName":"...","traceId":"...","sessionId":"..."}`
because they resolve a submitted work file instead of the shared session
invocation API.

Stdin-only example:

```bash
printf 'Summarize stdin input' | you run --factory ./factory.json
```

#### Failure stderr contract

When a clean invocation fails, is cancelled, or times out, it exits non-zero,
emits no success payload on stdout, and writes the failure contract to stderr.

Stable error codes include:

- `RUN_INVOCATION_FAILED` for runtime or work failures
- `RUN_INVOCATION_CANCELLED` for SIGINT or SIGTERM cancellation
- `RUN_INVOCATION_TIMEOUT` when the invocation deadline is exceeded
- `INVOCATION_INPUT_SOURCE_CONFLICT` when positional text and stdin conflict before startup

Without `--json`, stderr is a single concise text line beginning with the stable
error code. With global `--json`, stderr is a single parseable JSON object with
at least `code` and `message`.

Default replay recording still follows the configured recording behavior; clean
invocation only changes what reaches stdout and stderr.

Canonical guides: `you docs mock-workers` and
`you docs record-replay`. For an end-to-end authoring walkthrough,
see `you docs authoring-factories`.

## Factory validation matrix

Pre-mutation validation for OpenAPI `Factory` payloads is centralized in
`validationentry.ValidateFactoryAPI` (`pkg/factory/validationentry`). Each call
maps the payload once with `FactoryConfigFromOpenAPI`, then runs the profile
selected in `validation.Options`. Post-persist and prompt-run paths stay separate
on purpose: they validate disk layout or a narrowed prompt-run contract instead of
reusing the OpenAPI pre-check profiles.

| Entry point | When it runs | Profile / mechanism | Uses `ValidateFactoryAPI`? |
|-------------|--------------|---------------------|----------------------------|
| `POST /factory-validations` | Validate-only; no persist | `ProfileTopology` — structural checks on the mapped config (duplicates, dangling references, outcome routes, work-type completion) | Yes |
| Editable save pre-check (`factorysave.validateEditableFactoryTopology`) | Before `PUT` / graph save writes split layout | `ProfilePrePersist` — `LoadFromCanonicalJSON` normalization (bundled files, blocking load) then full `Validate()` | Yes |
| `you factory save` / `update --from` | Before `configpersist` writes named factory | `ProfilePrePersist` (same as editable save) | Yes |
| Persist post-write (`LoadRuntimeConfig` on staged split layout) | After files are materialized on disk, before commit | Disk-backed load: merge `workers/` / `workstations/` AGENTS.md, materialize bundled files, `validateBlockingFactoryLoad`, runtime definition maps | No — intentional second gate |
| `you run --factory` prompt submission | Before writing temporary prompt work file | `factoryrun.ValidateFactoryForPromptRun` — structural `Validate()` plus exactly one `handlingBehavior: ["DEFAULT"]` work type | No — v1 intentional subset (see below) |
| Runtime session / engine startup | When activating or loading a factory directory | `configload.LoadRuntimeConfigFromFactoryDir` (same core as `LoadRuntimeConfig`) | No |

### Profiles (`pkg/factory/validation`)

- **`ProfileTopology`** — Matches validate-only dashboard checks. One OpenAPI map,
  then `validation.Validate(&cfg)`. Does not call `LoadFromCanonicalJSON`.
- **`ProfilePrePersist`** — Matches editable save and CLI save-from-file
  pre-checks. One OpenAPI map, then marshal → `LoadFromCanonicalJSON` → on
  invalid-named-factory, blocking-load subset → otherwise full `Validate()`.
  Pass `WorkstationLoader` when split worker/workstation bodies must resolve.

Validate-only and save pre-check can disagree on the same JSON when save uses
`ProfilePrePersist` but the client only called validate with the default topology
profile. Regression tests in `pkg/service/factorysave` and
`pkg/factory/validationentry` lock pre-persist parity between save and
`ValidateFactoryAPI`; use `ProfilePrePersist` when comparing to save.

`Validate()` appends recoverable `factory.layout.*` warnings only after pending
graph topology indexes are built. Blocking topology targets and recoverable
layout targets stay separate: layout warnings do not make
`HasBlockingTargets()` true. Save paths additionally prune stale layout
references and return ephemeral `layoutOutcomes` on the save response. See
[Portable Graph Layout](#portable-graph-layout) for the field contract and
recovery semantics.

### Post-persist `LoadRuntimeConfig`

Live saves and named-factory persist stage a split layout, then call
`LoadRuntimeConfig` on the staging directory before commit (`pkg/config/layout.go`).
That pass proves the on-disk factory (thin `factory.json`, AGENTS.md trees,
bundled files) is runnable, including checks that require filesystem state. It is
not folded into `ValidateFactoryAPI` in v1.

### Prompt-run exception

`factoryrun.ValidateFactoryForPromptRun` validates an already-expanded
`*interfaces.FactoryConfig` from a portable `factory.json` path. It requires
structural validity and exactly one `DEFAULT` handling work type for
`you run --factory <path> <prompt>`. It does **not** call `ValidateFactoryAPI` in
v1: prompt runs load from disk, not from an editable OpenAPI payload, and the
product contract is intentionally narrower than full pre-persist save checks.
Converging prompt-run onto `ValidateFactoryAPI` is a future option if prompt and
save paths need identical failure codes for the same file.

## Related

- `you docs agents`
- `you docs work`
- `you docs mock-workers`
- `you docs record-replay`
- `you docs guards`
- `you docs relationships`
- `you docs authoring-factories`
- `you docs workstations`
- `you docs workers`
- `you docs resources`
- `you docs batch-work`
- `you docs templates`
- `docs/reference/README.md`
- `docs/README.md`
