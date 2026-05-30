---
author: Agent Factory Team
last-modified: 2026-05-30
doc-id: agent-factory/reference/agents
---

# Agents

`you docs agents` is the packaged entry point for autonomous agents and human
operators who need one orientation path across factories, work submission, CLI
commands, and packaged reference topics.

Run this topic first when you land in an unfamiliar factory. Then read the
factory's own `factory.json`, instance guidance under `factory/docs/` when
present, and the topic-specific guides linked from the router table below.

## Start Here

1. Run `you docs agents` (this guide) for cross-factory orientation.
2. Open the target factory's `factory.json` and run `you docs config` for
   topology tables and routing fields.
3. Read relevant `workstations/<name>/AGENTS.md` and `workers/<name>/AGENTS.md`
   files for the work types you will touch.
4. When the factory ships instance guidance, prefer
   `factory/docs/overview.md` or `factory/docs/README.md` over copying paths
   from another factory.
5. Open the task-specific topic (`you docs batch-inputs`, `you docs work`,
   `you docs relationships`, and so on) before submitting work.

Do not guess `workTypeName`, watched-folder paths, or initial states from a
different factory. Normative guidance in this guide uses generic work-type
names such as `task` or `story` only as examples.

## Read Order For Any Factory

| Step | What to read | Why |
|------|----------------|-----|
| 1 | `factory.json` | Declares work types, states, workers, workstations, resources, and routes. |
| 2 | `you docs config` | Field-by-field topology reference for `factory.json`. |
| 3 | `workstations/*/AGENTS.md` and `workers/*/AGENTS.md` | Runtime prompts and scoped execution settings for the stations you enable. |
| 4 | Task topic (`you docs work`, `you docs batch-inputs`, …) | Submission shape, tags, relations, and API contracts for your ingress path. |
| 5 | `factory/docs/overview.md` or `factory/docs/README.md` | Instance pipeline, inboxes, and maintainer notes when the factory authors them. |

For a full authoring walkthrough with runnable examples, see
`you docs authoring-factories`.

## Factory-Local Docs

Checked-in and exported factories may ship guidance under `factory/docs/`:

| Path | Use |
|------|-----|
| `factory/docs/overview.md` | Preferred instance walkthrough: pipeline, work types, input layout, read-before-submit notes. |
| `factory/docs/README.md` | Alternate instance index when a factory uses that name instead of `overview.md`. |

When either file exists, read it after `factory.json` and before submitting
work. Instance-specific work type names belong only in those factory-local
files—not in this packaged guide.

## Submitting Work

Work enters a running factory through watched markdown/JSON inboxes, startup
batch files, the CLI, or the HTTP API. Pick the path that matches how the
factory is already running and whether you need relations on the request.

| Ingress | When to use | Canonical detail |
|---------|-------------|------------------|
| `factory/inputs/<work_type>/default/<request_id>.json` | Watched-folder batch for a single work type while the factory runs. | `you docs batch-inputs` |
| `factory/inputs/BATCH/default/<request_id>.json` | Mixed work types or submitted `PARENT_CHILD` / `DEPENDS_ON` graphs. | `you docs batch-inputs` |
| `you run --work <path>` | Submit a readable batch JSON file before or as part of starting a run. | `you docs batch-inputs` |
| `you submit` | CLI single-work submission when the factory is already running. | `you docs work` |
| `POST /work` | HTTP single-work submission. | `you docs work` |
| `PUT /work-requests/{request_id}` | HTTP batch submission with the same `FACTORY_REQUEST_BATCH` body as file input. | `you docs batch-inputs` |

Batch files use `type: "FACTORY_REQUEST_BATCH"` with OpenAPI camelCase fields
such as `requestId`, `workTypeName`, and `works[].name`. Single-work API calls
require explicit `name` and `workTypeName` on every item.

Before authoring batch JSON, read the target `factory.json` and any
`factory/docs/overview.md` or `factory/docs/README.md` for valid work types and
inbox layout.

## Command Matrix

| Command | Factory must be running? | Typical agent use |
|---------|--------------------------|-------------------|
| `you docs` / `you docs <topic>` | No | Load packaged reference markdown (embedded in the CLI). |
| `you docs agents` | No | Start here for orientation and topic routing. |
| `you run --dir <factory>` | Starts runtime | Run the factory from a directory; optional natural-language task string. |
| `you run --factory <factory.json>` | Starts runtime | Run from an explicit `factory.json` path. |
| `you run --work <batch.json>` | Submits before/at start | Seed work from a batch file when bringing the factory up. |
| `you submit` | Yes | Submit one work item through the CLI against a live factory. |
| Dashboard / `GET /work`, `GET /status` | Yes | Observe tokens, traces, and completion without changing topology. |

Use `you docs config` when you need to change or verify topology. Use
`you docs work` and `you docs batch-inputs` when you need to change what work
enters the factory, not how the graph is declared.

Mock and record controls (`you run --with-mock-workers`, `--record`, `--replay`,
`--no-record`) are documented in `you docs mock-workers` and
`you docs record-replay`.

## Planner, Executor, And AGENTS.md Authoring

Three roles show up often in agent-driven factories. This guide orients all of
them; only the third has a dedicated split-file shape guide.

| Role | Responsibility | Read first |
|------|----------------|------------|
| **Planner / scheduler** | Decomposes goals, chooses work types, authors batch or API submissions, and monitors completion. | This guide → `factory.json` → `you docs batch-inputs` or `you docs work` |
| **Executor** | Consumes tokens at workstations, follows worker and workstation `AGENTS.md` bodies, and routes outcomes via configured fields. | Relevant `workstations/*/AGENTS.md` → `you docs workstations` → `you docs workers` |
| **AGENTS.md author** | Edits split worker and workstation prompt files without duplicating topology in markdown. | [Author AGENTS.md](authoring-agents-md.md) |

Planners should not embed topology tables in submission payloads—declare work
types in `factory.json` (`you docs config`) and submit through the ingress
table above. Executors should not invent new routes; they follow
`outputs`, `onContinue`, `onRejection`, and `onFailure` already declared in
`factory.json`.

[Author AGENTS.md](authoring-agents-md.md) owns YAML frontmatter, prompt
bodies, prompt files, and split-file placement. It does not replace
`you docs config` for `factory.json` fields or `you docs work` for submission
contracts.

## Topic Router

Run `you docs <topic>` for packaged markdown. Compatibility aliases
`batch-work` and `workstation` resolve to the same bytes as
`batch-inputs` and `workstations`.

| Topic | Run | One-line purpose |
|-------|-----|------------------|
| agents | `you docs agents` | Agent orientation, read order, submission ingress, and this router (start here). |
| authoring-factories | `you docs authoring-factories` | End-to-end factory authoring workflow and runnable examples. |
| config | `you docs config` | `factory.json` topology: work types, states, routing, resources, portability. |
| mock-workers | `you docs mock-workers` | Deterministic mock-worker runs and JSON selection contract. |
| record-replay | `you docs record-replay` | Record and replay run modes, artifacts, and flag combinations. |
| guards | `you docs guards` | Workstation, input, and factory guards and guarded loop breakers. |
| relationships | `you docs relationships` | Batch `DEPENDS_ON` / `PARENT_CHILD` and runtime `SPAWNED_BY` lineage. |
| work | `you docs work` | Submitted work: `POST /work`, tags, tokens, and batch cross-links. |
| workstations | `you docs workstations` | Workstation kinds, route fields, and runtime step behavior. |
| workers | `you docs workers` | Worker types, model providers, and script workers. |
| resources | `you docs resources` | Resource capacity and workstation resource requirements. |
| models | `you docs models` | Local and hosted model setup for workers and CLI model commands. |
| batch-inputs | `you docs batch-inputs` | Batch files, `FACTORY_REQUEST_BATCH`, and watched-folder layout (`batch-work` alias). |
| templates | `you docs templates` | Prompt template variables and Go template behavior. |

## Related

- `you docs authoring-factories` — full setup walkthrough with examples.
- `you docs config` — `factory.json` topology owner.
- `you docs work` — single-work and tag flow after submission.
- `you docs batch-inputs` — batch ingress and `FACTORY_REQUEST_BATCH`.
- [Author AGENTS.md](authoring-agents-md.md) — split worker and workstation file shape.
