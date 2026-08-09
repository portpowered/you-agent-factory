This is the table of contents for the Agent Factory documentation.

The installed CLI also packages a fixed reference surface under
`you docs`. Run `you docs` to list the packaged topics, or
run `you docs <topic>` for packaged topics such as `agents`,
`authoring-factories`, `config`, `factory-validation`, `mock-workers`,
`record-replay`, `guards`, `relationships`, `operations`, `work`,
`workstations`, `workers`, `resources`, `models`, `batch-inputs`, or
`templates`.

## For agents

- Run `you docs agents` first for end-to-end agent orientation (read order, work
  submission, command matrix, planner vs executor, and the packaged topic
  router).
- Before submitting work in a checked-in factory, read `factory.json` plus
  factory-local docs when present:
  - prefer `factory/docs/overview.md` for the instance pipeline, work types, and
    read-before-submit guidance;
  - otherwise `factory/docs/README.md` when that file exists.
- Repo-level factory pointers live in
  [`factory/docs/overview.md`](../factory/docs/overview.md); do not duplicate
  instance walkthroughs in the root docs index.

## Packaged CLI Reference Topics

These are the fixed topic names accepted by `you docs <topic>`.

- `agents` is the packaged agent orientation guide. Run `you docs agents` for
  read order, work submission, the command matrix, planner vs executor, and
  factory-local docs discovery.
- `authoring-factories` is the packaged practical factory authoring guide. Run
  `you docs authoring-factories` for workflow sequencing, runnable examples,
  mock-worker checks, and replay recording.
- `config` is the packaged `factory.json` topology reference. Run `you docs
  config` for work types, states, workers, workstations, resources,
  portability, and top-level layout fields.
- `factory-validation` is the packaged pre-run static validation guide. Run
  `you docs factory-validation` for the required validate-only gate, exact
  file/directory commands, current checks, and the boundary between static
  acceptance and runtime execution.
- `mock-workers` is the packaged mock-worker reference. Run `you docs
  mock-workers` for `--with-mock-workers` and the `mockWorkers` JSON contract.
- `record-replay` is the packaged record and replay reference. Run `you docs
  record-replay` for default recording, `--record`, `--replay`, and incompatible
  flag combinations.
- `guards` is the packaged guards reference. Run `you docs guards` for
  workstation, input, and factory guards plus guarded loop breakers.
- `relationships` is the packaged batch and lineage relations reference. Run
  `you docs relationships` for `DEPENDS_ON`, `PARENT_CHILD`, and runtime
  `SPAWNED_BY` semantics.
- `operations` is the packaged real-pipeline lifetime and restart-recovery
  runbook. Run `you docs operations` for continuous operation, finite-drain
  classification, and same-name Work restoration.
- `work` is the packaged submitted-work reference. Run `you docs work` for
  session-scoped work routes, tags, batch cross-links, and submission-oriented runtime flow.
- `workstations` is the packaged workstation reference. `workstation` remains
  accepted as a compatibility alias for the same raw markdown. Run `you docs
  workstations` for the canonical workstation guide.
- `workers` is the packaged worker quick reference. Run `you docs workers` for
  the canonical worker guide.
- `resources` is the packaged bounded-concurrency reference. Run `you docs
  resources` for the resource slice and `you docs config` for top-level topology
  fields.
- `models` is the packaged model operations quick reference. Run `you docs
  models` for model discovery, invocation, and local or hosted model setup.
- `batch-inputs` is the packaged batch-request reference. Run `you docs
  batch-inputs` for submitted payload fields and watched-file placement.
  `batch-work` remains accepted as a compatibility alias for the same raw
  markdown.
- `templates` is the packaged template syntax reference. Run `you docs
  templates` for template surfaces, the complete variable inventory, and
  JSON-versus-Markdown quoting rules.

## Customer Guides

- [CLI reference](reference/README.md) is the package-owned topic index for the stable packaged reference pages.
- Canonical concept guides:
  - `you docs config` owns work types, work states, top-level `factory.json`,
    routing behavior, runtime resources, and portability fields.
  - `you docs work` owns `POST /factory-sessions/{session_id}/work`,
    submitted-work tags, and batch cross-links.
  - `you docs workstations` owns workstation kinds, route fields, runtime step
    behavior, and workstation-scoped execution settings.
  - `you docs workers` owns worker types, worker-scoped runtime fields, and split
    `workers/<name>/AGENTS.md` placement.
  - `you docs batch-inputs` owns `FACTORY_REQUEST_BATCH` ingress, watched-file
    placement, and authored relation types (`batch-work` remains a CLI alias for
    the same guide).
- Run `you docs config` for the canonical split layout, `factory.json`, and where
  worker, workstation, and input files live.
- Run `you docs factory-validation` before the first Factory run and after
  authored topology, guard, route, schema, or split-layout changes.
- Run `you docs resources` for top-level resource pools and workstation or
  worker resource requirements.
- Run `you docs templates` for supported Go-template surfaces, the complete
  variable inventory, and the JSON-versus-Markdown quoting rule.
- Run `you docs authoring-factories` to configure and run factories end to end,
  including mock-worker checks, replay recording, and reusable inputs under
  [docs/examples](examples/README.md).
- [Author AGENTS.md](reference/authoring-agents-md.md) explains split
  `AGENTS.md` file shape, prompt placement, and authoring patterns.
- [The Zen of flow](reference/the-zen-of-flow.md) explains the project’s workflow
  philosophy.
- [Record/replay reference](reference/record-replay.md) explains how recordings,
  replay, and related runtime inspection flows fit together.

## Maintainer workflow (packaged CLI reference)

- Edit topic markdown only under [`docs/reference/`](reference/README.md).
- Run `make docs-reference-smoke` from the repository root before shipping.
- Do not maintain a parallel copy under `pkg/transports/cli/docs/`; `you docs` serves the
  embedded `docs/reference/` tree.

## Contributor Guides

- [Development guide](internal/development/development.md)
- [Architecture](architecture/architecture.md)
- [Data model](architecture/data-model.md)
- [Logical session identity](architecture/logical-session-identity.md)
- [Cursor agent session storage](internal/development/cursor-agent-session-storage.md)
- [React UI test warning inventory](internal/development/react-ui-test-warning-inventory.md)
