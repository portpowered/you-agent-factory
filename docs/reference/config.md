# Config Reference

Use this page when you need the canonical factory directory layout, the
field-by-field `factory.json` topology contract, and where each authored file
lives. Use [Submitted work](work.md) for `POST /work`, tags, and batch
cross-links.

## Current Contract

- `factory.json` is the canonical root file. It owns factory-level workflow
  topology such as `id`, `workTypes`, `workers`, `workstations`, routes,
  optional runtime `resources`, and the optional portability
  `supportingFiles`; the normative field contract lives on this page.
- Keep worker runtime instructions in `workers/<name>/AGENTS.md`.
- Keep workstation runtime instructions in `workstations/<name>/AGENTS.md`.
- Keep watched work inputs under `inputs/<work-type-or-BATCH>/<channel>/`.
- Inline runtime fields in `factory.json` are still supported for portable
  single-file configs, but the split layout is the recommended authoring path.
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
| `supportingFiles` | No | Portability-only manifest for validation-only external tools and bundled files. This is distinct from runtime-capacity `resources`. |
| `runner` | No | Factory-level default runner ID. Supported built-ins are `codex`, `gemini`, `kiro`, `cursor-cli`, and `opencode`. |
| `workers` | Yes | Worker identities that workstations reference by `name`; see [Workers](workers.md) for worker runtime fields. |
| `workstations` | Yes | Dispatch steps that consume input states and produce output states; see [Workstations](workstations.md) for the workstation field contract. |

Do not rely on stale top-level `global_limits` or `exhaustionRules` examples.
The current public `factory.json` authoring contract uses guarded
`LOGICAL_MOVE` workstations and workstation limits for user-configured safety
behavior.

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
[Submitted work](work.md) for API and batch submission fields.

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

## Workstation IO And Resources

Workstation inputs, outputs, rejection routes, failure routes, and guarded
loop-breaker routes use `{ "workType": "<name>", "state": "<name>" }`. Top-level
`resources` declare bounded concurrency pools that workstations reference through
`resources` entries. See [Workstations](workstations.md), [Workers](workers.md),
[Resources](resources.md), and [Guards](guards.md) for field-level contracts.

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
  factory `runner`, then legacy worker `modelProvider` compatibility, then the
  default `codex` runner.
- Validate runner prerequisites before execution. Built-in runner selection
  expects the corresponding local CLI on `PATH`, and runner-specific auth or
  local setup must already be in place.
- Keep portability-only declarations under `supportingFiles`; do not overload
  runtime-capacity `resources` with bundled files or external tool checks.
- Treat `inputs/` as submission data, not as part of the topology. The runtime
  watches the path and turns those files into work requests.

## Portable Bundled Files

Use this contract when you want a canonical portable `factory.json` to collect,
carry, and restore supporting files across `config flatten`, `config expand`,
and `LoadRuntimeConfig(...)` without redefining the manifest shape.

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

Canonical guides: [Mock workers](mock-workers.md) and
[Record and replay](record-replay.md). For an end-to-end authoring walkthrough,
see [Author factories](authoring-factories.md).

## Related

- [Agents](agents.md)
- [Guards](guards.md)
- [Relationships](relationships.md)
- [Mock workers](mock-workers.md)
- [Record and replay](record-replay.md)
- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Author factories](authoring-factories.md)
- [Submitted work](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Batch inputs](batch-inputs.md)
