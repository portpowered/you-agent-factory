# Repository Maintainer Factory Overview

This checked-in `./factory/` directory is the repository-maintainer workflow
for infinite-you itself. It automates ideation, planning, task execution, and
review loops while keeping human-facing maintainer state under
`factory/internal/`.

## Quick Start

From the repository root:

```bash
you run --dir ./factory
```

Use the dashboard or CLI to submit work into the inboxes below. For portable
agent orientation that applies to any factory, run `you docs agents` first.

## How Work Moves

The pipeline below matches the live `factory/factory.json` topology.

```text
thoughts:init
  -> ideafy -> thoughts:complete

idea:init
  -> plan -> idea:to-complete
           + plan:init

plan:init
  -> setup-workspace -> plan:complete
                      + task:init

task:init
  -> process (REPEATER) -> task:in-review
  -> review              -> task:to-complete
  -> consume (LOGICAL_MOVE, SAME_NAME guard) -> idea:complete + task:complete

cron-triggers:init
  -> cleaner (CRON hourly) -> cron-triggers:complete
```

Guarded loop breakers bound repeated retries:

| Workstation | Guard | Effect |
| --- | --- | --- |
| `executor-loop-breaker` | `VISIT_COUNT` on `process` (max 50) | `task:init` -> `task:failed` |
| `review-loop-breaker` | `VISIT_COUNT` on `review` (max 10) | `task:in-review` -> `task:failed` |

`process` and `review` use per-task git worktrees under
`.claude/worktrees/{{ task name }}`. `setup-workspace` runs
`factory/scripts/setup-workspace.py` to prepare those worktrees.

## Work Types And Initial States

| Work type | Initial state | Terminal / failure states |
| --- | --- | --- |
| `thoughts` | `init` | `complete`, `failed` |
| `idea` | `init` | `to-complete`, `complete`, `failed` |
| `plan` | `init` | `complete`, `failed` |
| `task` | `init` | `in-review`, `to-complete`, `complete`, `failed` |
| `cron-triggers` | `init` | `complete`, `failed` |

Shared resource: `executor-slot` (capacity 10). Model workstations consume one
slot each.

## Read Before You Submit

1. **`factory/factory.json`** — valid `workTypeName` values, state names, and
   routing for batch or single-work submissions.
2. **`factory/workstations/*/AGENTS.md`** — planner (`plan`), executor
   (`process`), and reviewer (`review`) prompts; read the workstation that will
   handle your submission.
3. **`you docs batch-inputs`** — when authoring `FACTORY_REQUEST_BATCH` JSON
   under `factory/inputs/BATCH/default/`.
4. **`you docs config`** and **`you docs work`** — topology versus submitted-work
   contracts for any factory.

Do not guess `workTypeName` or state names from other repositories; use this
factory's `factory.json` and the tables above.

## Input Layout

Seed checked-in repository work under:

| Inbox | Use |
| --- | --- |
| `factory/inputs/thoughts/default/` | Ideation seeds for `thoughts:init` |
| `factory/inputs/idea/default/` | Standalone idea markdown (default for new asks) |
| `factory/inputs/plan/default/` | Planning artifacts when needed |
| `factory/inputs/task/default/` | Standalone task markdown |
| `factory/inputs/BATCH/default/` | `FACTORY_REQUEST_BATCH` JSON when you need `DEPENDS_ON`, `PARENT_CHILD`, or mixed work types in one file |

Each canonical inbox is kept in git by a tracked `.gitkeep` sentinel so the
directory exists in clean checkouts. Actual work items inside those folders are
repository-local working state, not the default starter from `agent-factory init`.

Authoring guidance:

- Default to one standalone markdown idea file under `inputs/idea/default/`.
- Use `inputs/BATCH/default/` only when the request needs dependency ordering
  or mixed work types in one canonical batch file.

## Maintainer Notes

- **Control surface:** `factory/internal/meta.md` (theory of mind),
  `factory/internal/asks.md`, `factory/internal/view.md`, and
  `factory/internal/progress.md` — keep these aligned when updating maintainer
  guidance.
- **Replay samples:** `factory/logs/agent-fails.json` is the checked-in
  event-stream sample for replay conversion; `factory/logs/agent-fails.replay.json`
  is the paired replay artifact for replay smoke.

Portable export picks up every file under `factory/docs/` (including this
overview) via the supported bundled-file collector in `pkg/config/layout.go`.
No extra `supportingFiles` entry is required while the file remains on disk
under `factory/docs/`.

## Related CLI Topics

| Topic | Command |
| --- | --- |
| Agent orientation (any factory) | `you docs agents` |
| `factory.json` topology | `you docs config` |
| Submitted work and API flow | `you docs work` |
| Batch JSON contracts | `you docs batch-inputs` (alias `you docs batch-work`) |
| Relations and guards | `you docs relationships`, `you docs guards` |
| Templates and authoring walkthrough | `you docs templates`, `you docs authoring-factories` |
