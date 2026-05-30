# Config Reference

Use this page when you need the canonical factory directory layout and where
each authored file lives. Use `you docs config` for the field-by-field
`factory.json` topology contract and [Submitted work](work.md) for submission
contracts.

## Current Contract

- `factory.json` is the canonical root file. It owns factory-level workflow
  topology such as `id`, `workTypes`, `workers`, `workstations`, routes,
  optional runtime `resources`, and the optional portability
  `supportingFiles`; the normative field contract lives in
  `you docs config`.
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

For a minimal `factory.json` example, use
`you docs config` (Minimal Factory).

## Portability Manifest Placement

Use `supportingFiles` in `factory.json` when the portable factory must declare
external tools or carry bundled helper files beyond workflow topology. The
manifest field contract belongs in
`you docs config` (Portability Resource Manifest);
this page only records that bundled files are restored beside the expanded
factory layout.

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
  declares `handlingBehavior: ["DEFAULT"]` (see `you docs config`, Default handling for one-shot CLI runs)
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

- `you docs agents`
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
