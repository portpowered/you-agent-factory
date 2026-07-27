---
author: Agent Factory Team
last-modified: 2026-06-03
doc-id: agent-factory/guides/agents
---

# Agents

Orient inside any you-agent-factory deployment: what to read first, how work
enters the factory, and which `you docs` topic answers the next question. Run
bare `you` for generated command discovery and `you docs` with no topic for the
packaged topic index. Bare `you` prints help successfully and does not load a
Factory, write configuration, start a Factory Session, contact a provider, bind
a listener, or open a browser.

## Read order

Before changing files or submitting work:

| Step | Read |
|------|------|
| 1 | `factory.json` in the factory directory |
| 2 | `you docs config` for topology, work types, states, workers, workstations, resources, and portability |
| 3 | Factory-local `factory/docs/overview.md` or `factory/docs/README.md` when present |
| 4 | `factory/workstations/<name>/AGENTS.md` for the step you execute |
| 5 | `factory/workers/<name>/AGENTS.md` for the bound worker backend and system prompt |
| 6 | The task topic from [Topic router](#topic-router) (for example `you docs work`, `you docs batch-inputs`, or `you docs relationships`) |

Then open the task-specific `you docs` topic from the router when you need normative
contracts. Do not guess `workTypeName`, inbox paths, or pipeline stages from repository
layout alone—instance-specific names belong in factory-local docs. For greenfield factory
setup, run `you docs authoring-factories`.

## CLI-only ingress

**Autonomous agents must submit work only through the CLI.** Use `you submit` for one
work item, `you submit batch` for a `FACTORY_REQUEST_BATCH` against a running factory,
and `you run --work <path>` **only** when starting a local factory run with batch JSON
(for example `you run --dir <factory> --work batch.json`). Do not treat watched inbox
files, dashboard submit, `POST /factory-sessions/{session_id}/work`, or
`PUT /factory-sessions/{session_id}/work-requests/{requestId}` as parallel agent
control paths.

| Command | When autonomous agents use it |
|---------|-------------------------------|
| `you submit` | Submit one work item to a **running** factory |
| `you submit batch` | Upsert batch JSON to a **running** factory session |
| `you run --work <path>` | Submit batch JSON as part of **local startup** only—not steady-state ingress while a factory is already running |

Operators who use watched `factory/inputs/**`, dashboard HTTP submit,
`POST /factory-sessions/{session_id}/work`, or
`PUT /factory-sessions/{session_id}/work-requests/{requestId}` should read
`you docs batch-inputs` and `you docs work` for inbox
layout, HTTP contracts, and operator workflows.

## Pre-submit checklist

- [ ] Confirm a factory service is running — `you session list` or [Is the factory running?](#is-the-factory-running?)
- [ ] Read `factory.json` and factory-local `factory/docs/overview.md` or `factory/docs/README.md` when present
- [ ] Confirm the `workTypeName` exists in `factory.json` (`you docs config`)
- [ ] For multi-item work, prepare `FACTORY_REQUEST_BATCH` JSON and submit with `you submit batch`—not inbox files (`you docs batch-inputs` for operator layout only)
- [ ] Choose a stable, non-empty `requestId` before the first batch submit; reuse it on retries ([Idempotency and duplicate work](#idempotency-and-duplicate-work))
- [ ] Set `name` and relation fields with OpenAPI camelCase (`requestId`, `workTypeName`, `sourceWorkName`, `targetWorkName`)
- [ ] Add `relations[]` when ordering or parent membership matters (`you docs relationships`)

## Batch submit for agents

When a factory is already running, upsert multi-item work with `you submit batch`. The
JSON body must set `"type": "FACTORY_REQUEST_BATCH"` and a **stable, non-empty `requestId`**.
The CLI validates locally, then issues `PUT` to
`/factory-sessions/{session_id}/work-requests/{requestId}`. For `DEPENDS_ON`, `PARENT_CHILD`,
relation field names, and full batch shape, read `you docs batch-inputs`.

```json
{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-auth",
      "workTypeName": "story",
      "payload": { "title": "Harden auth session handling" }
    }
  ]
}
```

```bash
you submit batch ./batches/release-story-set.json
cat batch.json | you submit batch
you submit batch --dry-run <path>
```

Run `you submit batch --help` for `--file`, stdin (`you submit batch -`), `--server`,
`--session`, and `--json`.

### Idempotency and duplicate work

The factory does **not** automatically deduplicate arbitrary retries:

| Path | Dedupes when you retry? |
|------|-------------------------|
| `you submit batch` with the same `requestId` and batch body | Yes — idempotent batch upsert |
| `you submit batch` with a **new** `requestId` | No — new batch submission |
| `you submit` (unary), repeated with the same flags and payload | No — each call enqueues **new** work |
| New inbox file under `factory/inputs/**` | No — new file, not CLI batch retry |
| `POST /factory-sessions/{session_id}/work` or dashboard submit | No — new work |

**Autonomous agents:** Reuse the same stable `requestId` on `you submit batch` when you mean
to refresh one batch without creating duplicate batches. Change `requestId` or call unary
`you submit` only when you want additional work. Do not use inbox or
`POST /factory-sessions/{session_id}/work` expecting
`you submit batch` idempotency.

## Is the factory running?

Before `you submit` or `you submit batch`, confirm a factory service is listening. A local
`factory.json` on disk does not mean a runtime is accepting work.

1. **`you session list`** (primary) — calls `GET /factory-sessions` on the running host
   (default `http://localhost:7437`). Empty table means no open sessions; connection refused
   means start a listening service with `you server` or a server-enabled run such as
   `you run --continuously --with-server`.
2. **`you factory query`** — active factory definition for the selected session when you need
   the loaded factory name before `--session` on submit or work commands.
3. **Deeper checks** — status API fields, dashboard URL, and continuous run modes: `you docs sessions`.

## Operator loop

1. **Check running** — [Is the factory running?](#is-the-factory-running?)
2. **Submit** — `you submit` or `you submit batch` (stable `requestId` for batches)
3. **Verify** — `you work show <work-id>` or `you work list --name <name>` (`you docs work`)

Pass the same `--server` and `--session` on submit and verify commands.

```bash
you submit \
  --name driver-incident-review \
  --work-type-name task \
  --payload request.md
```

Replace `task` with a `workTypeName` from your `factory.json` and `request.md` with the
payload file your factory expects.

## Command matrix

| Command | Purpose | Factory must already be running? |
|---------|---------|----------------------------------|
| `you` | Print generated root help without loading or starting a Factory | No — command is side-effect-free discovery |
| `you run --dir <factory>` | Start (or attach to) a local factory from a directory | No — command starts runtime |
| `you run --work <batch.json>` | Submit batch JSON as part of **local startup** | No when combined with `--dir` startup |
| `you submit` | Submit one work item (autonomous agent path) | Yes |
| `you submit batch` | Upsert batch JSON to a running session (autonomous agent path) | Yes |
| Dashboard / `POST /factory-sessions/{session_id}/work` | **Operator-only** API or UI submission | Yes (`you docs work`) |
| `you docs <topic>` | Print packaged reference markdown | No |

Use `you docs mock-workers` and `you docs record-replay` for deterministic runs without live
provider calls.

## Planner vs executor

| Role | Responsibility | Typical artifacts |
|------|----------------|-------------------|
| **Planner / scheduler** | Chooses the next work item, prepares batch or single-work JSON, enqueues via `you submit` or `you submit batch` without executing workstation prompts | Batch JSON files, unary `you submit` flags and payloads |
| **Executor** | Runs when a token reaches a workstation input place; loads worker + workstation `AGENTS.md`, calls the configured worker backend, returns accept, continue, reject, or failure | Workstation and worker `AGENTS.md`, rendered templates (`you docs templates`) |

`INFERENCE_RUN` workstations execute one bounded model operation through an
`INFERENCE_WORKER` backend. They are harnessless inference steps, not agent
loops. Use `you docs models` for OmniVoice TTS and other typed inference
authoring.

`AGENT_RUN` workstations execute prompt-rendered agent loops through an
`AGENT_WORKER` backend. Configure explicit `agentTools.policy` on the worker,
keep tool execution disabled unless you intend filesystem tools, and inspect
bounded agent diagnostics separately from final output (`you docs workers`,
`you docs workstations`, `you docs sessions`).

Planners should read factory-local overview docs and `you docs config` before submitting.
Executors should read the workstation and worker `AGENTS.md` for the active step before
changing repository files. Prompt composition rules live in `docs/reference/authoring-agents-md.md`
(repository doc; not a packaged CLI topic).

## Topic router

| Intent | Command |
|--------|---------|
| Agent orientation (start here) | `you docs agents` |
| Factory authoring walkthrough | `you docs authoring-factories` |
| Run a Factory | `you docs run` |
| `factory.json` topology and portability | `you docs config` |
| Mock-worker test runs | `you docs mock-workers` |
| Record and replay CLI modes | `you docs record-replay` |
| Guards and loop breakers | `you docs guards` |
| Batch relations (`DEPENDS_ON`, `PARENT_CHILD`, `SPAWNED_BY`) | `you docs relationships` |
| Submitted work (`POST /factory-sessions/{session_id}/work`, tags, tokens) | `you docs work` |
| Sessions, factory query, status API, dashboard | `you docs sessions` |
| Dynamic workflow authoring and execution | `you docs javascript-workflows` |
| Factory Session MCP host setup | `you docs mcp` |
| Workstation routing and runtime fields | `you docs workstations` |
| Worker types and providers | `you docs workers` |
| Resource capacity | `you docs resources` |
| Harnessless model operations (`INFERENCE_WORKER`, `INFERENCE_RUN`, TTS) | `you docs models` |
| Agent-loop workers, tool policy, and failure classes | `you docs workers` |
| `AGENT_RUN` workstations and agent-loop routing | `you docs workstations` |
| Agent-run dispatch inspection | `you docs sessions` |
| Batch ingress and inbox layout | `you docs batch-inputs` (alias: `you docs batch-work`) |
| Prompt template variables | `you docs templates` |

## Factory-local docs discovery

| Path | Use |
|------|-----|
| `factory/docs/overview.md` | Preferred portable overview: pipeline, work types, inboxes |
| `factory/docs/README.md` | Fallback overview when `overview.md` is absent |
| `factory/workstations/*/AGENTS.md` | Step prompts and routing-owned fields |
| `factory/workers/*/AGENTS.md` | Worker backends and system prompts |

Read `factory.json` plus factory-local overview before choosing a `workTypeName` or inbox path.
When factory-local files disagree with this packaged guide, the factory-local file wins for
instance-specific names.
