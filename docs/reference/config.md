# Config Reference

Use this page when you need the canonical factory layout and where each authored
file lives.

## Current Contract

- `factory.json` is the canonical root file. It owns factory-level workflow
  topology such as `id`, `workTypes`, `workers`, `workstations`, routes,
  optional runtime `resources`, and the optional portability `supportingFiles`.
- Keep worker runtime instructions in `workers/<name>/AGENTS.md`.
- Keep workstation runtime instructions in `workstations/<name>/AGENTS.md`.
- Keep watched work inputs under `inputs/<work-type-or-BATCH>/<channel>/`.
- Inline runtime fields in `factory.json` are still supported for portable
  single-file configs, but the split layout is the recommended authoring path.
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

## Minimal Factory

`factory.json`:

```json
{
  "id": "sample-service",
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

For that minimal factory:

- Put the topology in `factory.json`.
- Put the worker instructions in `workers/processor/AGENTS.md`.
- Put the workstation prompt or runtime instructions in
  `workstations/process/AGENTS.md`.
- Drop watched single-work-type requests under `inputs/task/default/`.
- Drop mixed-work-type or relation-heavy batch files under
  `inputs/BATCH/default/`.

## Portability Manifest

Use `supportingFiles` when the portable factory must declare external tools or
carry bundled helper files beyond workflow topology.

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

- `requiredTools` are declarative only. They describe host tools that later
  load or preflight validation can check on `PATH`; they are not embedded or
  installed by the portability contract.
- `bundledFiles` are distinct from runtime-capacity `resources`. They carry
  portable file content plus a canonical factory-relative `targetPath`.
- The default collected bundle paths for this slice are `factory/scripts/**`,
  `factory/docs/**`, and supported root helper files such as `Makefile`.
- `config flatten` collects those supported files automatically from a checked-in
  `factory/` layout and writes them into `resourceManifest.bundledFiles` in
  deterministic `targetPath` order.
- `type: "SCRIPT"` entries must target `factory/scripts/...`; `type: "DOC"`
  entries must target `factory/docs/...`; `type: "ROOT_HELPER"` entries must
  target a supported project-root helper path such as `Makefile`.
- `targetPath` must use forward slashes and must already be canonical. Absolute
  paths, backslash-separated paths, and paths with `.` or `..` segments are
  rejected instead of being normalized silently.
- `content.encoding` is `utf-8` in this v1 slice, so bundled file payloads are
  inline UTF-8 text.

## Bootstrap Checklist

- Start with `factory.json`, then add split `AGENTS.md` files for any
  prompt-heavy or runtime-heavy worker and workstation definitions.
- Keep one directory per worker or workstation so the runtime can resolve
  `workers/<name>/AGENTS.md` and `workstations/<name>/AGENTS.md` by the names
  used in `factory.json`.
- Use camelCase public config fields in `factory.json`; do not author new
  configs with retired snake_case aliases.
- Keep portability-only declarations under `resourceManifest`; do not overload
  runtime-capacity `resources` with bundled files or external tool checks.
- Treat `inputs/` as submission data, not as part of the topology. The runtime
  watches the path and turns those files into work requests.

## Portable Bundled Files

Use this contract when you want a canonical portable `factory.json` to collect,
carry, and restore supporting files across `config flatten`, `config expand`,
and `LoadRuntimeConfig(...)` without redefining the manifest shape.

- `config flatten` adds supported `factory/scripts/**`, `factory/docs/**`, and
  root helper files such as `Makefile` to
  `resourceManifest.bundledFiles` automatically for checked-in `factory/`
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

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Author workflows](../authoring-workflows.md)
- [Factory JSON and work configuration](../work.md)
- [Workstations and workers](../workstations.md)
- [Author AGENTS.md](../authoring-agents-md.md)
- [Batch inputs](../guides/batch-inputs.md)
