This is the table of contents for the Agent Factory documentation.

The installed CLI also packages a fixed reference surface under
`infinite-you docs`. Run `infinite-you docs` to list the packaged topics, or
run `infinite-you docs <topic>` for one of `config`, `workstation`, `workers`,
`resources`, `batch-work`, or `templates`.

## Packaged CLI Reference Topics

- `config` is the packaged `factory.json` reference. Start with [Factory JSON and work configuration](reference/work.md) for the broader guide.
- `workstation` is the packaged workstation reference. Start with [Workstations and workers](reference/workstations-and-workers.md) for the broader guide.
- `workers` is the packaged worker reference. Start with [Author AGENTS.md](reference/authoring-agents-md.md) for the broader guide.
- `resources` is the packaged resource reference. Start with [Factory JSON and work configuration](reference/work.md) for the broader guide.
- `batch-work` is the packaged batch-request reference. Start with [Batch inputs](reference/batch-inputs.md) for the broader guide.
- `templates` is the packaged template reference. Start with [Prompt variables](reference/prompt-variables.md) for the broader guide.

## Customer Guides

- [CLI reference](reference/README.md) is the package-owned topic index for the stable `config`, `workstations`, `workers`, `resources`, `batch-work`, and `templates` reference pages.
- [Config reference](reference/config.md) explains the canonical split layout, `factory.json`, and where worker, workstation, and input files live.
- [Workstations reference](reference/workstations.md) explains workstation kinds, route fields, and when to use standard, repeater, or cron steps.
- [Workers reference](reference/workers.md) explains worker types, worker-owned runtime fields, and split `AGENTS.md` placement.
- [Resources reference](reference/resources.md) explains top-level resource pools and workstation or worker resource requirements.
- [Batch-work reference](reference/batch-work.md) explains `FACTORY_REQUEST_BATCH`, watched-file placement, and authored relation types.
- [Templates reference](reference/templates.md) explains supported Go-template surfaces and the JSON-versus-Markdown quoting rule.
- [Author workflows](reference/authoring-workflows.md) explains how to configure and run factory workflows.
- [Author AGENTS.md](reference/authoring-agents-md.md) explains how to configure workers and workstations.
- [Factory JSON and work configuration](reference/work.md) explains `factory.json`, work types, workers, resources, and routes.
- [Batch inputs](reference/batch-inputs.md) explains `FACTORY_REQUEST_BATCH` files, fields, and dependency relations.
- [The Zen of flow](reference/the-zen-of-flow.md) explains the project’s workflow philosophy.
- [Workstations and workers](reference/workstations-and-workers.md) explains workstation kinds, runtime fields, prompts, cron, and worker definitions.
- [Prompt variables](reference/prompt-variables.md) lists values available in workstation prompts.
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
