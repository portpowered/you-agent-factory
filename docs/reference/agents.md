---
author: Agent Factory Team
last-modified: 2026-05-31
doc-id: agent-factory/guides/agents
---

# Agents

Use this guide when you are an autonomous agent or operator learning how to
orient inside any you-agent-factory deployment: which docs to read first, how
work enters the factory, and which `you docs` topic answers the next question.

Run `you docs agents` whenever you need the end-to-end agent playbook. Run
`you docs` with no topic to print the packaged topic index.

## Start Here

1. Run `you docs agents` (this guide).
2. Run `you docs` to list every packaged topic.
3. Open the target factory's `factory.json`.
4. When the factory ships companion docs, read them before submitting work:
   - prefer `factory/docs/overview.md`
   - otherwise `factory/docs/README.md`
5. Open the workstation and worker `AGENTS.md` files for the step you will run.
6. Open the task-specific `you docs` topic from the [Topic router](#topic-router)
   table when you need normative contracts.

Do not guess `workTypeName`, inbox paths, or pipeline stages from repository
layout alone. Instance-specific names belong in factory-local docs, not in this
packaged guide.

## Read Order (Any Factory)

Use this order before changing files or submitting work:

| Step | Read |
|------|------|
| 1 | `factory.json` in the factory directory |
| 2 | [Config](config.md) (`you docs config`) for topology, work types, states, workers, workstations, resources, and portability |
| 3 | Factory-local `factory/docs/overview.md` or `factory/docs/README.md` when present |
| 4 | `factory/workstations/<name>/AGENTS.md` for the step you execute |
| 5 | `factory/workers/<name>/AGENTS.md` for the bound worker backend and system prompt |
| 6 | The task topic from the [Topic router](#topic-router) (for example [Work](work.md), [Batch Inputs](batch-inputs.md), or [Relationships](relationships.md)) |

[Authoring Factories](authoring-factories.md) walks through a full greenfield
setup when you are creating or restructuring a factory.

## Authoring Factories

When you are building or extending a factory rather than only submitting work:

| Need | Topic or file |
|------|----------------|
| End-to-end factory walkthrough with runnable commands | [Authoring Factories](authoring-factories.md) |
| `factory.json` topology and portability | [Config](config.md) |
| Workstation routing, guards, and runtime kinds | [Workstations](workstations.md) |
| Worker providers, models, and script workers | [Workers](workers.md) |
| Split `AGENTS.md` file shape and prompt composition | [Author AGENTS.md](authoring-agents-md.md) (repository doc; not a packaged CLI topic) |

Keep topology changes in `factory.json` and [Config](config.md). Keep prompt
bodies in worker and workstation `AGENTS.md` files per
[Author AGENTS.md](authoring-agents-md.md).

## Submitting Work

Work enters a running factory through one of these ingress paths:

| Ingress | When to use |
|---------|-------------|
| Watched `factory/inputs/**` JSON files | Steady-state factory already running; canonical batch and per-type inboxes |
| `you run --work <path>` | Submit a batch JSON file before or while starting a local factory run |
| `POST /work` | Single submitted work item against a running API |
| `you submit` / dashboard submit | Operator-driven single-work submission with UI validation |
| `PUT /work-requests/{id}` | Replace or update a staged work request where the API supports it |

Batch submissions use `FACTORY_REQUEST_BATCH`. Single submissions use explicit
`name` and `workTypeName` fields. See [Work](work.md) for `POST /work` and tag
flow; see [Batch Inputs](batch-inputs.md) for batch shape, inbox placement,
`DEPENDS_ON`, and `PARENT_CHILD`.

### Pre-Submit Checklist

- [ ] Read `factory.json` and factory-local `factory/docs/overview.md` or
  `factory/docs/README.md` when present.
- [ ] Confirm the `workTypeName` exists in `factory.json` ([Config](config.md)).
- [ ] Place batch files under the inbox paths your factory documents
  ([Batch Inputs](batch-inputs.md)).
- [ ] Set `requestId`, `name`, and relation fields with OpenAPI camelCase names
  (`requestId`, `workTypeName`, `sourceWorkName`, `targetWorkName`).
- [ ] Add `relations[]` when ordering or parent membership matters
  ([Relationships](relationships.md)).

## Is the factory running?

Before `you submit`, watched `factory/inputs/**`, or `POST /work`, confirm a factory
service is listening and has an open session. A local `factory.json` on disk does
not by itself mean a runtime is accepting work.

Use these checks in order:

### 1. `you session list` (primary)

`you session list` calls `GET /factory-sessions` on the running host. It is the
fastest liveness probe when you only need to know whether any live sessions exist.

- Default target is `http://localhost:7437` (`--port 7437` on session commands).
- An empty table (`No live factory sessions were found.`) means the service
  responded but no sessions are open yet.
- **Connection refused** or **endpoint not reachable** means nothing is listening
  on that host and port — start the factory with `you`, `you run --continuously`,
  or `you run --dir <factory>` before submitting.
- Use `you session list --json` for automation; use global `--server` on other HTTP
  client commands (`you factory query`, `you submit`, `you work list`) when the API
  base URI is not the default `http://localhost:7437`.

### 2. `you factory query`

`you factory query` reads the **active factory definition** from a live service
(`GET /factory-sessions/{session_id}/factory` for the selected session). It confirms
which factory name and topology the runtime loaded — not merely which
`factory.json` exists in a checkout.

- Run after `you session list` when you need the active factory name before routing
  `--session` on submit or work commands.
- Use `you --json factory query` for the API-shaped current-factory payload.

### 3. `GET /factory-sessions/{session_id}/status`

For deeper runtime health, call `GET /factory-sessions/{session_id}/status` (session
id is often `~default` on single-session hosts). The response separates:

| Field | Meaning |
|-------|---------|
| `factoryState` | Factory lifecycle phase — for example `IDLE`, `RUNNING`, `PAUSED`, `COMPLETED`, `FAILED`. |
| `runtimeStatus` | Whether the engine is actively processing — `IDLE`, `ACTIVE`, or `FINISHED`. |

`factoryState` can be `RUNNING` while `runtimeStatus` is `IDLE` when the factory is
up but no work is in flight. Use both fields together when deciding whether to
submit more work or wait for completion.

### 4. Dashboard

When the service was started via `you` or `you run` without `--quiet`, open the
dashboard at **`/dashboard/ui`** on the same host and port as the API (default
`http://localhost:7437/dashboard/ui`). The port follows `--server` / `--port` on
the process that bound the listener.

### 5. Run modes (keep the service up)

| How you start | Stays running for submissions? |
|---------------|--------------------------------|
| `you` (no args) | Yes — continuous mode; watches default inputs and keeps the service up. |
| `you run --continuously` | Yes — processes work until you stop the process. |
| `you run` (batch, no `--continuously`) | No — exits when the factory goes idle; not suitable for later `you submit` or watched inbox ingress unless you restart it. |

For steady `you submit`, dashboard submit, or `POST /work` against an already-running
host, prefer `you` or `you run --continuously`. Use batch `you run` when you want a
one-shot local run that shuts down on idle.

## Command Matrix

| Command | Purpose | Factory must already be running? |
|---------|---------|----------------------------------|
| `you run --dir <factory>` | Start (or attach to) a local factory from a directory | No — command starts runtime |
| `you run --factory <path> "<prompt>"` | One-shot CLI run with inline factory file and prompt | Depends on flags; see [Config](config.md) |
| `you run --work <batch.json>` | Submit batch JSON as part of startup | No when combined with `--dir` startup |
| `you submit` | Submit work through CLI/dashboard flows | Yes |
| Dashboard / `POST /work` | API submission against running service | Yes |
| `you docs <topic>` | Print packaged reference markdown | No |

Use [Mock Workers](mock-workers.md) and [Record and Replay](record-replay.md)
when you need deterministic runs without live provider calls.

## Planner vs Executor

Autonomous agent workflows usually split into two cooperating roles:

| Role | Responsibility | Typical artifacts |
|------|----------------|-------------------|
| **Planner / scheduler** | Reads topology, chooses the next work item, writes batch or single-work requests, and enqueues work without executing workstation prompts | Batch JSON under `inputs/`, planning prompts, `POST /work` bodies |
| **Executor** | Runs when a token reaches a workstation input place; loads worker + workstation `AGENTS.md`, calls the configured worker backend, and returns accept, continue, reject, or failure outcomes | Workstation and worker `AGENTS.md`, rendered templates ([Templates](templates.md)) |

Planners should prefer factory-local overview docs and [Config](config.md) before
submitting. Executors should read the workstation and worker `AGENTS.md` for the
active step before changing repository files or responding to review loops.

[Author AGENTS.md](authoring-agents-md.md) documents how worker and workstation
`AGENTS.md` files compose into system and user prompts. It does **not** define
planner versus executor scheduling policy—that operational split lives here and
in your factory's `factory/docs/overview.md`.

## Topic Router

| Intent | Command |
|--------|---------|
| Agent orientation (start here) | `you docs agents` |
| Factory authoring walkthrough | `you docs authoring-factories` |
| `factory.json` topology and portability | `you docs config` |
| Mock-worker test runs | `you docs mock-workers` |
| Record and replay CLI modes | `you docs record-replay` |
| Guards and loop breakers | `you docs guards` |
| Batch relations (`DEPENDS_ON`, `PARENT_CHILD`, `SPAWNED_BY`) | `you docs relationships` |
| Submitted work (`POST /work`, tags, tokens) | `you docs work` |
| Workstation routing and runtime fields | `you docs workstations` |
| Worker types and providers | `you docs workers` |
| Resource capacity | `you docs resources` |
| Model setup | `you docs models` |
| Batch ingress and inbox layout | `you docs batch-inputs` (alias: `you docs batch-work`) |
| Prompt template variables | `you docs templates` |

## Factory-Local Docs Discovery

Every factory may ship companion documentation beside `factory.json`:

| Path | Use |
|------|-----|
| `factory/docs/overview.md` | Preferred portable overview: pipeline, work types, inboxes, maintainer notes |
| `factory/docs/README.md` | Fallback overview when `overview.md` is absent |
| `factory/workstations/*/AGENTS.md` | Step prompts and routing-owned fields |
| `factory/workers/*/AGENTS.md` | Worker backends and system prompts |

Read `factory.json` plus `factory/docs/overview.md` or `factory/docs/README.md`
before choosing a `workTypeName` or inbox path. When those files disagree with
this packaged guide, the factory-local file wins for instance-specific names.

## Related Topics

- [Config](config.md) — `factory.json` topology
- [Work](work.md) — submitted-work contracts
- [Batch Inputs](batch-inputs.md) — batch ingress (alias: `batch-work`)
- [Authoring Factories](authoring-factories.md) — greenfield factory setup
- [Relationships](relationships.md) — batch and runtime relation semantics
