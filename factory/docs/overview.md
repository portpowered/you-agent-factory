# Factory Overview

This Factory coordinates autonomous work for you-agent-factory, the Go,
OpenAPI, and React system for scheduling and orchestrating concurrent AI
workers through the you CLI, backend runtime, and dashboard.

The Factory has four operating roles:

- the GPT-6.1 Sol Portfolio Supervisor uses medium reasoning with high
  autonomy to watch the whole repository, admit Projects, reconcile health,
  and choose the next priority every eight hours or when a significant
  exception occurs;
- a GPT-6.1 Sol Project Lead at high reasoning owns one Project's immutable
  acceptance contract and chooses its next behavior slice;
- GPT-6.1 Sol planning workers shape one bounded plan at high reasoning;
- GPT-6.1 Sol delivery and review workers execute and gate one local Work item
  at medium reasoning; and
- GPT-6.1 Sol validation workers run fresh, read-only customer, engineering,
  or retrospective missions at medium reasoning against immutable artifacts.

The executable Factory definition is the authority for Work types, states,
Workstations, relations, resources, and worker selection. The presentation
layout is metadata: it makes the system legible without changing runtime
behavior. Read [operating-policy.md](./operating-policy.md) for the decision
policy and [projects.md](./projects.md) for Project admission and lead
responsibilities.

## Read first

Before submitting or changing Work, read:

- `factory/factory.json`;
- `factory/docs/operating-policy.md`;
- `factory/docs/projects.md`;
- `factory/workstations/ideafy/AGENTS.md`;
- `factory/workstations/project-lead/AGENTS.md`;
- `factory/docs/batch-inputs.md` and its checked-in example;
- `docs/temp/customer-ask.md`, when present;
- `docs/temp/progress.md`, `docs/temp/checklist.md`, and `docs/temp/meta.md`,
  when present; and
- `docs/temp/board-lessons.md`, when present.

Also read root `AGENTS.md`, the architecture notes relevant to the requested
surface, and the applicable factory and repository standards.

The canonical local Factory server is `http://127.0.0.1:7437`. Workers must pass
it explicitly to every API-backed you command. This Factory endpoint is separate
from any unrelated local service.

## Control loop

The Factory observes current runtime state, chooses the smallest useful
behavior slice or proof, dispatches through the existing Work graph, and
reconciles the resulting evidence. It does not manufacture activity when the
queue is quiet.

The outer Project loop is:

```text
project:init -> project-lead -> project:waiting
                              + idea:init       (tags.project = <project tag>)
                              + validation:init (tags.project = <project tag>)

tagged idea/validation reaches complete or failed
  -> report-* (deterministic) -> child terminal state + project-report:pending
project:waiting + project-report:pending (same project tag)
  -> project-lead-wake -> project:waiting + project-report:delivered

project:waiting + project-cycle:complete -> project:complete
project:waiting + project-cycle:blocked  -> project:blocked

every 15m project:waiting -> project-lead-checkin -> project:waiting
```

Lead batches carry no loopback item. Each tagged child that reaches
`complete` or `failed` wakes its own Project Lead exactly once, and the wake
names that child; peer Projects are never woken because the wake matches on
the `project` tag. Reports that arrive while the lead is busy queue in
`project-report:pending`. A failed child wakes the lead, which inspects its
evidence and makes a cause-corrected repair or escalates the missing route.
Failure must not be silently converted into Project success. The same-name
`project-cycle` is now only the relation-free terminal decision.

The Project Lead emits only the immediate behavior and proof Work justified by
current evidence. Local Work may complete before the Project acceptance
criteria are proven. The lead then emits another behavior slice or validation
mission, or records a concrete external hold.

The check-in ticks every 15 minutes and each tick visits one waiting Project,
so with N waiting Projects each is visited roughly every N quarter-hours. It
inventories
current and retained Work/PRs, repairs evidenced failures through supported
controls, and may admit an independent ready idea or validation through the
CLI. It is the safety net beside child wakes for stalled nonterminal Work,
untagged Work, and Projects without a `project` tag.

## Work types

The current operating vocabulary is:

| Work type | Purpose |
| --- | --- |
| `thoughts` | Portfolio Supervisor trigger and legacy unowned-work loopback |
| `project` | One substantial outcome with one Project Lead |
| `project-cycle` | Same-name Project Lead terminal decision (complete or blocked) |
| `project-report` | One finished child's wake notice for its Project Lead |
| `idea` | One behavior slice or justified bounded enabler |
| `plan` | PRD planning generated from an idea |
| `task` | Implementation/review delivery Work generated by the inner graph |
| `validation` | First-class read-only customer, engineering, or retrospective mission |

Use `idea` for an implementation proposal and `validation` for independent
evidence. Do not use an informal subagent call as a substitute for either.

## Presentation shape

The Factory layout has two layers:

1. executable topology: Work types, states, Workstations, workers, resources,
   and runtime relations; and
2. presentation metadata: authored node geometry, edge geometry, semantic
   groups, named flows, annotations, viewport, and display preferences.

Presentation groups should organize the canvas by responsibility and decision
boundary, for example:

- Portfolio supervision: trigger, liveness, admission, and priority;
- Project control: Project Lead, Project state, and Project cycle;
- Delivery: idea, plan, task, CI, review, and consume;
- Validation and learning: validation missions, reports, and retrospective
  feedback; and
- Inputs and outputs: customer requests, source plans, acceptance contracts,
  artifacts, and terminal results.

Named presentation flows should show the customer-relevant routes through those
groups, such as Project admission, behavior delivery, failure escalation,
independent validation, and retrospective learning. A group or flow is
presentation metadata only; it must reference stable canonical topology IDs and
must not become a second source of runtime state. Coordinates are never a
semantic dependency. The layout should remain readable when a group is empty,
a route is failed, or a Project is waiting.

## Delivery flow

The ordinary implementation flow remains:

```text
idea:init
  -> plan
  -> plan:init
  -> setup-workspace
  -> task:init
  -> process
  -> task:awaiting-ci
  -> ci-wait
  -> task:in-review
  -> review
  -> task:to-complete
  -> consume
```

`ci-wait` classifies confirmed merged PRs as `merged` and sends the task
straight to `task:to-complete`. The existing `consume` join completes the task
and reports idea completion, releasing its dependents without another review
visit. Other successful observations select `review` and retain the route
above. Both loop breakers keep their existing limits for unmerged work.

The Factory invokes `ci-wait.py` with `--classification`: stdout contains only
`merged` or `review`; the JSON receipt goes to stderr. Direct invocations without
that option retain JSON stdout and the existing exit status contract.

To activate this route, the operator must refresh the materialized Factory
configuration and script together, validate that materialized configuration
with `you factory config validate <materialized-factory.json>`, and restart the
daemon. A source merge alone does not update the running Factory Session.
Observe the next naturally arriving merged lane for task and idea completion,
dependent release, and absence of a breaker failure. To roll back, restore the
previous materialized configuration and script together, validate, and restart;
retain the session history.


The validation flow is first-class and uses the same Work lifecycle:

```text
validation:init
  -> prepare-validation
  -> validation:ready
  -> validate
  -> validation:complete
```

A failed or rejected validation reaches `validation:failed` and wakes its
Project Lead. A validation mission never edits the repository,
advances another Work item, weakens a rubric, or marks product acceptance by
itself.

Every idea and validation mission a Project Lead emits carries the Project's
`project` tag; the conceptual invariant is one lead wake per finished tagged
child, with no loopback item in the batch.

## Validation and acceptance

When a Project is near completion, its lead emits two complementary validation
missions:

- customer: a fresh source-blind journey from the public entry point using only
  the acceptance contract, mission, rubric, and immutable build/fixture identity;
- engineering: a fresh independent check of regression, failure behavior,
  persistence/recovery, performance, LocalAI/model fidelity, or another named
  quality property.

Both are fresh, read-only, and independently reported. A
FAIL or BLOCKED result cannot be outvoted by a pass. The Project Lead enqueues
the smallest correction supported by the evidence.

A retrospective is a validation mission with role `retrospective`. It reports a
common or special cause and proposes an owner, evidence, verification
procedure, and rollback/stop condition. Retrospective output informs the Sol
Portfolio Supervisor; it does not mark product acceptance complete.

## Project working memory

Each Project has a separate durable root:

```text
docs/temp/projects/<project-name>/
  source-plan.md
  request.md
  acceptance.md
  state.md
  progress.md
  validation/
```

The operator or admission path supplies the immutable source plan, request, and
acceptance contract. The Project Lead maintains only mutable state, progress,
and validation reports. Runtime Work and Factory Events remain authoritative.
The supervisor and leads must keep all of these files out of feature branches.

The supervisor's own local state stays directly under `docs/temp/`:

```text
docs/temp/progress.md
docs/temp/checklist.md
docs/temp/meta.md
```

## Submission and no-action rule

Use the canonical `FACTORY_REQUEST_BATCH` shape from
`factory/docs/batch-inputs.md`. Dry-run every batch before submission:

```sh
you --server http://127.0.0.1:7437 submit batch --dry-run <path> --session <session_id>
you --server http://127.0.0.1:7437 submit batch <path> --session <session_id>
```

The checked-in example is `factory/docs/batch-input-example.json`; use it to
verify the batch envelope and dependency relation shape before preparing a
project-specific batch.

Before submitting or repairing Work, inspect the live queue and Factory
Session:

```sh
you work list --session <session_id>
you session list
```

When the CLI is not already configured for the canonical local server, use
`--server http://127.0.0.1:7437` with these inspection commands as well.

Admit a Project only when its source plan, contract revision, ownership, and
capacity are clear. Emit behavior slices instead of a complete speculative
graph. Use package or package-family ownership to avoid collisions, then
parallelize only when semantic prerequisites are satisfied.

When all active Projects are progressing or held on named external conditions
and no priority class has a safe, dependency-ready action, record a hold with
the next eight-hour review or exception trigger and stop. Do not create
placeholder Work, duplicate validation, restart healthy Projects, or weaken
acceptance criteria.
