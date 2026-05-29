---
author: Agent Factory Team
last-modified: 2026-05-21
doc-id: agent-factory/reference/config
---

# Config

`you docs config` stays available as the stable packaged split-layout quick
reference. Use [`docs/reference/config.md`](../../../docs/reference/config.md)
for the maintained layout guide and
[`docs/reference/work.md`](../../../docs/reference/work.md) for the broader
factory and work contract.

Use `factory.json` as the canonical topology file for a you-agent-factory run.
It declares the work types, states, workers, workstations, resources, and
routes that the runtime turns into a Petri-net execution graph.

## Minimal Shape

The smallest useful config has one work type, one worker, and one workstation:

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
  "workers": [{ "name": "executor" }],
  "workstations": [
    {
      "name": "process-task",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }]
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
  workers/executor/AGENTS.md
  workstations/process-task/AGENTS.md
  inputs/task/default/
```

`factory.json` still owns the topology. Split `AGENTS.md` files own worker and
workstation runtime content such as system prompts, prompt templates, timeout
limits, and executor settings.

For an end-to-end run walkthrough, including `--with-mock-workers`,
`--record`, `--replay`, `--no-record`, and reusable files under
`docs/examples/`, use
[`docs/reference/authoring-factories.md`](../../../docs/reference/authoring-factories.md).

## Core Fields

| Field | Description |
|-------|-------------|
| `project` | Optional factory-wide project name used when submitted work does not provide one. |
| `runner` | Optional factory-level default runner ID: `codex`, `gemini`, `kiro`, `cursor-cli`, or `opencode`. |
| `workTypes` | Declares work categories and lifecycle states. |
| `resources` | Declares shared concurrency pools. |
| `workers` | Declares worker identities and optional inline worker runtime config. |
| `workstations` | Declares dispatch steps, routing, and optional inline runtime fields. |

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
  retired top-level loop-breaking fields.
- Prefer split `AGENTS.md` files for long prompts and inline runtime fields for
  portable or recorded single-file configs.

## Portability And Bundled Files

- Keep portability-only declarations under `supportingFiles`; runtime-capacity
  pools still belong under `resources`.
- `config flatten` automatically collects eligible `factory/scripts/**`,
  `factory/docs/**`, supported root helper files such as `Makefile`, and the
  current valid starter work present under `inputs/<work-type-or-BATCH>/<channel>/`
  into `supportingFiles.bundledFiles`.
- The export bundle intentionally excludes unsupported project-root files,
  directories, `.gitkeep`, and temporary or editor-swap files.
- Shared-factory `INPUT` bundled files are share-time starter-work snapshots.
  Imported recipients restore detached copies, so later source-factory edits do
  not rewrite an already shared factory and recipient edits do not flow back to
  the source.
- Use [`docs/reference/config.md`](../../../docs/reference/config.md) and
  [`docs/reference/work.md`](../../../docs/reference/work.md) for the maintained
  allowlist, target-path, and portability-manifest contract details.

## Related

- `you docs workstation`
- `you docs workers`
- `you docs resources`
- `you docs batch-work`
- `you docs templates`
