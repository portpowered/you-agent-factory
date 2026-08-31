# Factory Overview

Planning and delivery work in this factory is governed by the canonical
[factory standards](./standards/README.md). Workers must read the standard for
their role before creating plans, implementing tasks, reviewing changes, or
acting on validation loopback results.

This factory coordinates autonomous work for **you-agent-factory**: the Go,
OpenAPI, and React system for scheduling and orchestrating concurrent AI workers
through the `you` CLI, backend runtime, and dashboard. The **ideafy** workstation
is the meta-planner: it supervises factory health, Project admission, and stalled
Project Leads. Each **project-lead** owns one substantial outcome, generates
ordinary `idea` Work as a dependency graph, and independently validates completion.
The **plan** workstation still turns each idea into a PRD. **process**, **ci-wait**,
and **review** still implement and gate work in isolated worktrees.

## Read First

Before submitting work, read:

* `factory/factory.json`
* `factory/workstations/ideafy/AGENTS.md`
* `docs/temp/customer-ask.md` — current customer authorization and goals
* `docs/temp/progress.md`, `docs/temp/checklist.md`, and `docs/temp/meta.md` —
  live planner state files (local, not checked in)
* `factory/docs/batch-inputs.md`
* `factory/docs/batch-input-example.json`
* `factory/docs/projects.md`
* `docs/temp/board-lessons.md` — operator-local board-shape admonitions
  (repo-root only; not materialized into worktrees)
* `factory/docs/decision-envelope.md`
* `you docs agents`
* `you docs batch-inputs`

Repository context that shapes planner batches:

* root `AGENTS.md` — architecture, package map, and verification expectations
* `docs/architecture/data-model.md` — public vocabulary (`Factory`, `Factory
  Session`, `Work`, `Work Request`)
* `docs/reference/` — packaged `you docs <topic>` contracts

## Project and Planner Loops

The preferred outer loop for substantial independent outcomes is:

```txt
project:init -> project-lead -> project:waiting
                              + idea:init (all currently well-scoped Work)
                              + project-cycle:init (depends on every idea:complete)

project:waiting + project-cycle:continue -> project:init
project:waiting + project-cycle:complete -> project:complete
project:waiting + project-cycle:failed   -> project:init
project:waiting + project-cycle:blocked  -> project:blocked
```

On first dispatch, the Project Lead bootstraps its working memory under
`docs/temp/<project-name>/` from the Project payload and source plan. Admission
never pre-creates that directory. The lead completes a Project only after blind
clean-room probes pass. See `factory/docs/projects.md`.

The meta-planner operates above Project Leads rather than implementing every
feature directly:

1. Check session, provider, resource, automation, and dispatch liveness.
2. Admit Project Work with one dedicated Project root and acceptance contract.
3. Inspect active Projects for stale leads, repeated failure, or shared-surface
   contention without taking over their healthy child Work.
4. Repair a recoverable blocked Project or factory-level fault once and record
   the evidence.
5. Retain `thoughts` loopbacks for small legacy/unowned work and periodic
   supervision.

Always dry-run a batch before real submission:

```sh
you submit batch --dry-run <path> --session <session_id>
```

Do not submit a real batch until the customer ask, checklist, and live queue
state agree the next slice of work is ready.

## Work Types

Configured work types:

```txt
thoughts       meta-planner loopback work
project        independently owned end-to-end outcome
project-cycle  dependency-held Project Lead loopback/decision
idea           product/implementation idea submitted by ideafy
plan           PRD planning output from an idea
task           executor/review implementation work
cron-triggers  runtime trigger type
```

Use `idea`, singular, for implementation proposals.
Use `thoughts`, plural, for ideafy loopback.

## Workstation Flow

```txt
thoughts:init -> ideafy -> thoughts:complete

project:init -> project-lead -> project:waiting + idea:init + project-cycle:init
project-cycle:init -> decide-project-cycle -> continue|complete|blocked
project:waiting + same-name project-cycle -> project:init|complete|blocked

idea:init -> plan -> idea:to-complete + plan:init
plan:init -> setup-workspace -> plan:complete + task:init
task:init -> process -> task:awaiting-ci
task:awaiting-ci -> ci-wait -> task:in-review
task:in-review + review:init with the same name -> review -> task:to-complete
idea:to-complete + task:to-complete with the same name -> consume
```

The **ci-wait** workstation is an agentless CI gate: a script
(`factory/scripts/ci-wait.py`) waits until every required PR check on the
lane's head is terminal (pass or fail — terminal-ness, not verdict), so
reviewer agent sessions never spend time or review visits watching CI. A
reviewer `<CONTINUE>` hold routes the task back to `awaiting-ci`, re-entering
the same gate instead of burning a review visit.

### Process and review visit budget

The process and review loop uses one logical cycle for each paired traversal.
Its two loop-breaker guards set `maxVisits` to `12` and `maxRawVisits` to
`24`. Seven review rejections therefore use fourteen raw visits without
reaching the logical limit.

The raw backstop still stops an imbalanced or unchanged route. It sums visits
from `process` and `review`, then fails the Work when it reaches `24`. A
`VISIT_COUNT` guard without `logicalRoundTrip` keeps the legacy raw behavior.

Executor and review workstations run in worktrees under
`.claude/worktrees/<work-item-name>/`, created by
`factory/scripts/setup-workspace.py`.

For a plan-to-task handoff, `setup-workspace.py` reads exactly one
`tasks/todo/<work-item-name>.json` from the main checkout or a Git-registered
worktree. The packet must be a JSON object whose `branchName` exactly matches
the requested Work; a non-root packet must also be in an existing attached
worktree on `refs/heads/<work-item-name>` with positive live-lane evidence
(otherwise an abandoned same-name lane is refused). Missing, invalid,
mismatched, stale, or ambiguous candidates fail before root synchronization,
pruning, worktree preparation, or copying. The root packet and its existing
destination behavior remain supported.

The **review** workstation runs the dedicated `reviewer` worker
(codex `gpt-5.6-luna`, reasoning effort `max`). Planning (`plan`) stays on the
`planner` worker (codex `gpt-5.6-sol`).

### Review quality probes

Reviews run on luna at max reasoning effort for cost reasons: sol review
sessions were the factory's largest spend bucket, and luna runs at roughly
1/25th of sol's token rates. Because review-evaluation quality on luna is a
known risk, the operator dispatches an independent "review quality probe"
subagent on luna-reviewed PRs. The probe re-reviews the same PR head at high
capability and grades the factory review as one of:

* `CONCUR` — the probe agrees with the factory review's verdict
* `MISSED_BLOCKER` — the factory review approved despite a real blocker
  (false approve)
* `FALSE_REJECTION` — the factory review rejected without a real blocker

A `MISSED_BLOCKER` on a merged PR, or a low concur rate over the first ~10
luna reviews, reverts the review worker to sol.

## Batch Submission

Use the canonical `FACTORY_REQUEST_BATCH` shape from `you docs batch-inputs`.
Human-readable notes live in `factory/docs/batch-inputs.md`.

For a running factory, prefer:

```sh
you submit batch <path> --session <session_id>
```

Always dry-run first:

```sh
you submit batch --dry-run <path> --session <session_id>
```

For watched-folder operator ingress, use:

```txt
factory/inputs/BATCH/default/<request_id>.json
```

The checked-in example is:

```txt
factory/docs/batch-input-example.json
```

Each batch should include several concrete `idea` items plus one `thoughts`
loopback item connected through `DEPENDS_ON` relations so the meta-planner
re-enters after the ideas complete.

## State Inspection

Before submitting new work, inspect the current queue and active sessions.

Use:

```sh
you work list --session <session_id>
```

to see current work items, work types, states, names, and whether previous
batches are still running, blocked, failed, or ready to be consumed.

Use:

```sh
you session list
```

to enumerate active and recent factory sessions. Check both commands before
deciding that work is stuck or before submitting a new batch. Session list
answers whether the runtime is alive; work list answers what the queue is doing
inside a session.

Replace `<session_id>` with a live id from `you session list` (for example
`c803e7f7-1361-4ba6-bb2b-b5c9cfeb2754` on a long-running host).

## Repair

Use:

```sh
you work move --session <session_id>
```

only for deliberate workflow repair. Record every manual move in
`docs/temp/progress.md` with the work item, old state, new state, reason, and
expected next workstation. Do not use work moves to skip implementation,
review, or validation.

## Local State Files

Planner-owned state under `docs/temp/`:

```txt
docs/temp/customer-ask.md  current customer authorization and goals
docs/temp/progress.md      append-only meta-planner progress log
docs/temp/checklist.md     high-level customer-ask and phase tracking
docs/temp/meta.md          lightweight world-state notes for long-running passes
```

These files are local planner state. Keep them out of version control when
possible. The meta-planner creates and maintains them during planning passes.
Task executors append to the worktree `progress.txt` at the repository root
during implementation batches.

## Quality Gates

Before opening or merging reconciliation PRs, run from the repository root:

```sh
make verify-fast   # dashboard typecheck, short UI/unit tests, short Go tests
make lint          # broader repository lint when touching shared surfaces
```

For higher-risk runtime, API, or UI changes, use `make verify-pr` or the
focused targets described in root `AGENTS.md`.

When changing factory-local planner docs or the checked-in batch example, also
run the narrow verification recipe documented in `factory/docs/batch-inputs.md`:

```sh
go test ./pkg/services/workers/prompting -run TestPromptRenderer_ResolvesCheckedInPlannerFactoryDocs -count=1
go test ./pkg/services/work/transports/cli/submit -run TestSubmitBatch_DryRunFactoryDocsBatchInputExample -count=1
```
