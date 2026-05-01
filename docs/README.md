This is the table of contents for the Agent Factory documentation.

The installed CLI also packages a fixed reference surface under
`agent-factory docs`. Run `agent-factory docs` to list the packaged topics, or
run `agent-factory docs <topic>` for one of `config`, `workstation`, `workers`,
`resources`, `batch-work`, or `templates`.

## Packaged CLI Reference Topics

- `config` is the packaged `factory.json` reference. Start with [Factory JSON and work configuration](work.md) for the broader guide.
- `workstation` is the packaged workstation reference. Start with [Workstations and workers](workstations.md) for the broader guide.
- `workers` is the packaged worker reference. Start with [Author AGENTS.md](authoring-agents-md.md) for the broader guide.
- `resources` is the packaged resource reference. Start with [Factory JSON and work configuration](work.md) for the broader guide.
- `batch-work` is the packaged batch-request reference. Start with [Batch inputs](guides/batch-inputs.md) for the broader guide.
- `templates` is the packaged template reference. Start with [Prompt variables](prompt-variables.md) for the broader guide.

## Customer Guides

- [CLI reference](reference/README.md) is the package-owned topic index for the stable `config`, `workstations`, `workers`, `resources`, `batch-work`, and `templates` reference pages.
- [Config reference](reference/config.md) explains the canonical split layout, `factory.json`, and where worker, workstation, and input files live.
- [Workstations reference](reference/workstations.md) explains workstation kinds, route fields, and when to use standard, repeater, or cron steps.
- [Workers reference](reference/workers.md) explains worker types, worker-owned runtime fields, and split `AGENTS.md` placement.
- [Resources reference](reference/resources.md) explains top-level resource pools and workstation or worker resource requirements.
- [Batch-work reference](reference/batch-work.md) explains `FACTORY_REQUEST_BATCH`, watched-file placement, and authored relation types.
- [Templates reference](reference/templates.md) explains supported Go-template surfaces and the JSON-versus-Markdown quoting rule.
- [Author workflows](authoring-workflows.md) explains how to configure and run factory workflows.
- [Author AGENTS.md](authoring-agents-md.md) explains how to configure workers and workstations.
- [Factory JSON and work configuration](work.md) explains `factory.json`, work types, workers, resources, and routes.
- [Batch inputs](guides/batch-inputs.md) explains `FACTORY_REQUEST_BATCH` files, fields, and dependency relations.
- [CLI release policy](guides/cli-release-policy.md) explains the maintainer semver tag workflow for CLI releases and why publication is tag-driven from `main`.
- [Parent-aware fan-in](guides/parent-aware-fan-in.md) explains how parent work waits for spawned children to complete or fail.
- [Workstation guards and guarded loop breakers](guides/workstation-guards-and-guarded-loop-breakers.md) explains when to use `guards`, guarded `LOGICAL_MOVE` loop breakers, resources, and runtime limits.
- [Workstations and workers](workstations.md) explains workstation kinds, runtime fields, prompts, cron, and worker definitions.
- [Prompt variables](prompt-variables.md) lists values available in workstation prompts.
- [Understand a run timeline](run-timeline.md) explains how `/events`, recordings, replay, and the dashboard use one ordered event timeline.
- [Migrate event type names](event-vocabulary-migration.md) explains the old-to-new event mapping and how to update existing consumers.
- [Record and replay a run](record-replay.md) explains how to save and replay an event-log artifact.

## Contributor Guides

- [Development guide](development/development.md)
- [Architecture](development/architecture.md)
- [API inventory](development/api-inventory.md)
- [Dashboard UI replay testing](development/dashboard-ui-replay-testing.md)
- [Factory config generated-schema boundary inventory](development/factory-config-generated-schema-boundary-inventory.md)
- [Live dashboard](development/live-dashboard.md)
- [Record/replay maintainer guide](development/record-replay.md)
