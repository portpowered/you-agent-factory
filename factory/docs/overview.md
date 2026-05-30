# Repository Maintainer Factory Overview

This directory (`./factory/`) is the checked-in **infinite-you** repository
maintainer workflow. Read this file and `factory.json` before submitting work;
do not guess `workTypeName`, inbox paths, or pipeline stages from repository
layout alone.

For generic agent orientation across any factory, run `you docs agents`. For
`factory.json` field contracts and portability, run `you docs config`. For
submitted-work and batch ingress contracts, run `you docs work` and
`you docs batch-inputs`.

## Quick start

From the repository root:

```bash
you run --dir ./factory
```

To submit a batch file while starting or against a running factory:

```bash
you run --work ./factory/inputs/BATCH/default/<request_id>.json --dir ./factory
```

Use `you docs record-replay` when you need recording or replay flags for local
runs.

## How work moves

Pipeline derived from the live `factory.json` in this directory:

```
thoughts:init → [ideafy] → thoughts:complete
idea:init → [plan] → idea:to-complete + plan:init
plan:init → [setup-workspace] → plan:complete + task:init
task:init → [process] → task:in-review → [review] → task:to-complete
                 ↑                         |
                 └──── onRejection ────────┘ (back to task:init)
[task:in-review + idea:to-complete] → [consume] → task:complete + idea:complete
```

Guarded `LOGICAL_MOVE` loop breakers bound retries:

| Workstation | Guard | Effect |
|-------------|-------|--------|
| `executor-loop-breaker` | `VISIT_COUNT` on `process`, max 50 | `task:init` → `task:failed` |
| `review-loop-breaker` | `VISIT_COUNT` on `review`, max 10 | `task:in-review` → `task:failed` |

The `cleaner` workstation runs on a cron schedule and completes
`cron-triggers:complete` work for housekeeping.

## Work types and initial states

| Work type | Initial state | Terminal / failure states |
|-----------|---------------|---------------------------|
| `thoughts` | `init` | `complete`, `failed` |
| `idea` | `init` | `to-complete`, `complete`, `failed` |
| `plan` | `init` | `complete`, `failed` |
| `task` | `init` | `in-review`, `to-complete`, `complete`, `failed` |
| `cron-triggers` | `init` | `complete`, `failed` |

Workstation prompts live under `factory/workstations/<name>/AGENTS.md`. Worker
backends live under `factory/workers/<name>/AGENTS.md`.

## Read before you submit

1. Run `you docs agents` for the end-to-end agent playbook.
2. Open `factory.json` in this directory.
3. Read this overview and the workstation `AGENTS.md` for the step you will run.
4. Confirm the target `workTypeName` and inbox path from the tables below — do
   not infer names from other repositories or examples.
5. For batch JSON, follow `you docs batch-inputs` (`you docs batch-work` is a
   byte-identical alias).

Planner workstations enqueue work; executor workstations run inference or
scripts at bound workers. See `you docs agents` § Planner vs Executor and
`docs/reference/authoring-agents-md.md` for prompt file shape.

## Input layout

Seed checked-in repository work under:

| Inbox | Use when |
|-------|----------|
| `factory/inputs/thoughts/default/` | Ideation markdown for `thoughts` work |
| `factory/inputs/idea/default/` | Standalone idea markdown (default for new ideas) |
| `factory/inputs/plan/default/` | Standalone plan markdown |
| `factory/inputs/task/default/` | Standalone task markdown |
| `factory/inputs/BATCH/default/` | `FACTORY_REQUEST_BATCH` JSON when you need dependency ordering or mixed work types in one request |

Each canonical inbox is kept in git by a tracked `.gitkeep` sentinel so the
directory exists in clean checkouts. Files inside those folders are
repository-local working state, not generated starter payloads from
`you init`.

Authoring guidance:

- Default to **one standalone markdown idea file** under `factory/inputs/idea/default/`.
- Use `factory/inputs/BATCH/default/` only when the request needs `DEPENDS_ON`,
  `PARENT_CHILD`, or multiple work types in one batch (see `you docs
  relationships`).

## Maintainer notes

- Maintainer control surface: `factory/internal/{asks,view,progress,meta}.md`
  (may be gitignored locally; align all four when updating maintainer state).
- Process/review semantics: `docs/internal/development/process-review-loop-contract.md`.
- Replay samples: `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` exercise event-stream → replay conversion.
- Resource capacity: `executor-slot` (capacity 10) gates model workstations.

## Related `you docs` topics

| Need | Command |
|------|---------|
| Agent orientation | `you docs agents` |
| `factory.json` topology and portability | `you docs config` |
| Submitted work and `POST /work` | `you docs work` |
| Batch ingress and inboxes | `you docs batch-inputs` |
| Relations in batch JSON | `you docs relationships` |
| Guards and loop breakers | `you docs guards` |
| Authoring a new factory | `you docs authoring-factories` |
