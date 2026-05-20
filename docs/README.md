This is the table of contents for the Agent Factory documentation.

The installed CLI also packages a fixed reference surface under
`you docs`. Run `you docs` to list the packaged topics, or
run `you docs <topic>` for one of `config`, `workstation`, `workers`,
`resources`, `batch-work`, or `templates`.

## Packaged CLI Reference Topics

These are the fixed topic names accepted by `infinite-you docs <topic>`.

- `config` is the packaged `factory.json` layout reference. Use
  [Factory JSON and work configuration](reference/work.md) for the canonical
  work and topology contract.
- `workstation` is the packaged workstation quick reference. Use
  [Workstations](reference/workstations.md) for the canonical workstation
  guide.
- `workers` is the packaged worker quick reference. Use
  [Workers](reference/workers.md) for the canonical worker guide.
- `resources` is the packaged bounded-concurrency reference. Use
  [Resources](reference/resources.md) for the resource slice and
  [Factory JSON and work configuration](reference/work.md) for top-level
  topology.
- `batch-work` is the packaged batch-request reference. Use
  [Batch inputs](reference/batch-inputs.md) for submitted payload fields and
  watched-file placement.
- `templates` is the packaged template syntax reference. Use
  [Templates](reference/templates.md) for template surfaces and
  [Prompt variables](reference/prompt-variables.md) for the complete variable
  list.

## Customer Guides

- [CLI reference](reference/README.md) is the package-owned topic index for the stable `config`, `workstation`, `workers`, `resources`, `batch-work`, and `templates` reference pages.
- Canonical concept guides:
  - [Factory JSON and work configuration](reference/work.md) owns work types, work states, top-level `factory.json`, routing, resources, and portability fields.
  - [Workstations reference](reference/workstations.md) owns workstation kinds, route fields, runtime step behavior, and workstation-scoped execution settings.
  - [Workers reference](reference/workers.md) owns worker types, worker-scoped runtime fields, and split `workers/<name>/AGENTS.md` placement.
- [Config reference](reference/config.md) explains the canonical split layout, `factory.json`, and where worker, workstation, and input files live.
- [Resources reference](reference/resources.md) explains top-level resource pools and workstation or worker resource requirements.
- [Batch inputs](reference/batch-inputs.md) explains `FACTORY_REQUEST_BATCH`, watched-file placement, and authored relation types.
- [Templates reference](reference/templates.md) explains supported Go-template surfaces and the JSON-versus-Markdown quoting rule.
- [Author workflows](reference/authoring-workflows.md) explains how to configure and run factory workflows.
- [Author AGENTS.md](reference/authoring-agents-md.md) explains split `AGENTS.md` file shape, prompt placement, and authoring patterns.
- [Batch inputs](reference/batch-inputs.md) explains `FACTORY_REQUEST_BATCH` files, fields, and dependency relations.
- [The Zen of flow](reference/the-zen-of-flow.md) explains the project’s workflow philosophy.
- [Workstations and workers](reference/workstations-and-workers.md) is a combined workflow-oriented overview that links to the canonical workstation and worker guides for contract details.
- [Prompt variables](reference/prompt-variables.md) lists values available in workstation prompts and rendered runtime fields.
- [Understand a run timeline](internal/development/run-timeline.md) explains how `/events`, recordings, replay, and the dashboard use one ordered event timeline.

## Contributor Guides

- [Development guide](internal/development/development.md)
- [Architecture](internal/development/architecture.md)
- [API inventory](internal/development/api-inventory.md)
- [CLI release policy](internal/development/cli-release-policy.md)
- [Dashboard UI replay testing](internal/development/dashboard-ui-replay-testing.md)
- [Factory config generated-schema boundary inventory](internal/development/factory-config-generated-schema-boundary-inventory.md)
- [Live dashboard](internal/development/live-dashboard.md)
- [Parent-aware fan-in](internal/development/parent-aware-fan-in.md)
- [Record/replay maintainer guide](internal/development/record-replay.md)
- [Workstation guards and guarded loop breakers](internal/development/workstation-guards-and-guarded-loop-breakers.md)
