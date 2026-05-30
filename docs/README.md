This is the table of contents for the Agent Factory documentation.

The installed CLI also packages a fixed reference surface under
`you docs`. Run `you docs` to list the packaged topics, or
run `you docs <topic>` for packaged topics such as `agents`,
`authoring-factories`, `config`, `mock-workers`, `record-replay`, `guards`,
`relationships`, `work`, `workstations`, `workers`, `resources`, `models`,
`batch-inputs`, or `templates`.

## For agents

- Run `you docs agents` first for end-to-end agent orientation (read order, work
  submission, command matrix, planner vs executor, and the packaged topic
  router).
- Before submitting work in a checked-in factory, read `factory.json` plus
  factory-local docs when present:
  - prefer `factory/docs/overview.md` for the instance pipeline, work types, and
    read-before-submit guidance;
  - otherwise `factory/docs/README.md` when that file exists.
- Repo-level factory pointers live in [`factory/README.md`](../factory/README.md);
  do not duplicate instance walkthroughs in the root factory README.

## Packaged CLI Reference Topics

These are the fixed topic names accepted by `you docs <topic>`.

- `agents` is the packaged agent orientation guide. Use
  [Agents](reference/agents.md) for read order, work submission, the command
  matrix, planner vs executor, and factory-local docs discovery.
- `authoring-factories` is the packaged practical factory authoring guide. Use
  [Author factories](reference/authoring-factories.md) for workflow sequencing,
  runnable examples, mock-worker checks, and replay recording.
- `config` is the packaged `factory.json` topology reference. Use
  [Config](reference/config.md) for work types, states, workers, workstations,
  resources, portability, and top-level layout fields.
- `mock-workers` is the packaged mock-worker reference. Use
  [Mock workers](reference/mock-workers.md) for `--with-mock-workers` and the
  `mockWorkers` JSON contract.
- `record-replay` is the packaged record and replay reference. Use
  [Record and replay](reference/record-replay.md) for default recording,
  `--record`, `--replay`, and incompatible flag combinations.
- `guards` is the packaged guards reference. Use [Guards](reference/guards.md)
  for workstation, input, and factory guards plus guarded loop breakers.
- `relationships` is the packaged batch and lineage relations reference. Use
  [Relationships](reference/relationships.md) for `DEPENDS_ON`, `PARENT_CHILD`,
  and runtime `SPAWNED_BY` semantics.
- `work` is the packaged submitted-work reference. Use
  [Submitted work](reference/work.md) for `POST /work`, tags, batch cross-links,
  and submission-oriented runtime flow.
- `workstations` is the packaged workstation reference. `workstation` remains
  accepted as a compatibility alias for the same raw markdown. Use
  [Workstations](reference/workstations.md) for the canonical workstation
  guide.
- `workers` is the packaged worker quick reference. Use
  [Workers](reference/workers.md) for the canonical worker guide.
- `resources` is the packaged bounded-concurrency reference. Use
  [Resources](reference/resources.md) for the resource slice and
  [Config](reference/config.md) for top-level topology fields.
- `models` is the packaged model operations quick reference. Use
  [Models and model operations](reference/models.md) for model discovery,
  invocation, and local or hosted model setup.
- `batch-inputs` is the packaged batch-request reference. Use
  [Batch inputs](reference/batch-inputs.md) for submitted payload fields and
  watched-file placement. `batch-work` remains accepted as a compatibility
  alias for the same raw markdown.
- `templates` is the packaged template syntax reference. Use
  [Templates](reference/templates.md) for template surfaces, the complete
  variable inventory, and JSON-versus-Markdown quoting rules.

## Customer Guides

- [CLI reference](reference/README.md) is the package-owned topic index for the stable packaged reference pages.
- Canonical concept guides:
  - [Config](reference/config.md) owns work types, work states, top-level
    `factory.json`, routing behavior, runtime resources, and portability fields.
  - [Submitted work](reference/work.md) owns `POST /work`, submitted-work tags,
    and batch cross-links.
  - [Workstations reference](reference/workstations.md) owns workstation kinds,
    route fields, runtime step behavior, and workstation-scoped execution
    settings.
  - [Workers reference](reference/workers.md) owns worker types, worker-scoped
    runtime fields, and split `workers/<name>/AGENTS.md` placement.
  - [Batch inputs](reference/batch-inputs.md) owns `FACTORY_REQUEST_BATCH`
    ingress, watched-file placement, and authored relation types (`batch-work`
    remains a CLI alias for the same guide).
- [Config reference](reference/config.md) explains the canonical split layout,
  `factory.json`, and where worker, workstation, and input files live.
- [Resources reference](reference/resources.md) explains top-level resource pools
  and workstation or worker resource requirements.
- [Templates reference](reference/templates.md) explains supported Go-template
  surfaces, the complete variable inventory, and the JSON-versus-Markdown quoting
  rule.
- [Author factories](reference/authoring-factories.md) explains how to configure
  and run factories end to end, including mock-worker checks, replay recording,
  and reusable inputs under [docs/examples](examples/README.md).
- [Author AGENTS.md](reference/authoring-agents-md.md) explains split
  `AGENTS.md` file shape, prompt placement, and authoring patterns.
- [The Zen of flow](reference/the-zen-of-flow.md) explains the project’s workflow
  philosophy.
- [Understand a run timeline](internal/development/run-timeline.md) explains how
  `/events`, recordings, replay, and the dashboard use one ordered event timeline.

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
