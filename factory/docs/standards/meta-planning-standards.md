# Factory meta-planning standards

---
author: andreas abdi
last modified: 2026, august, 24
doc-id: FSTD-004
---

Meta-planning turns customer goals and factory evidence into the next smallest
useful batch of behavior work. It coordinates the system; it does not create
activity for its own sake.

## Quick rules

- Maintain an explicit model of customer goals, current system state, active
  behavior lanes, dependencies, risks, and evidence.
- Reconcile failed, blocked, or stranded work before submitting new work.
- Submit narrow behavior slices or justified enabling tasks, not horizontal
  layer batches.
- Optimize throughput by limiting work in progress, exposing real bottlenecks,
  and avoiding shared-surface contention—not by maximizing worker utilization.
- Use loopback as feedback. Reassess the plan from current evidence rather than
  automatically dispatching the next prewritten batch.
- Prefer mechanisms that prevent recurring failure over checklists that merely
  detect it, while retaining short unambiguous checklists for human/agent state.
- Record special-cause failures separately from common workflow variation.
- Stop when no safe, useful, dependency-ready work exists.

## 1. Maintain the planning model

The meta-planner **MUST** keep concise state for:

- customer asks and measurable outcomes;
- current architecture and operational constraints;
- parent behavior lanes and their executable-spine state;
- work in progress, semantic dependencies, and shared-surface owners;
- verified facts, assumptions, risks, unproven edges, and later gates;
- failures, their classification, retry history, and corrective action; and
- completed outcomes and lessons that change future planning.

Transient factory state files remain untracked and must not enter feature PRs.
State should be compacted when it stops helping the next decision.

## 2. Shape work around behavior and evidence

Each submitted idea **MUST** identify:

1. the parent customer/system behavior;
2. one observable outcome or bounded enabling capability;
3. the behavioral witness;
4. expected executable-spine effect;
5. highest feasible verification scope and dependency fidelity;
6. remaining real or expensive edges and their future gates;
7. semantic prerequisites and shared-surface ownership; and
8. applicable cost, call, duration, safety, and authority constraints.

Do not submit separate API, backend, UI, test, and documentation ideas for one
behavior merely because different files or services are involved. Horizontal
enablers require the justification defined by the planning standard.

## 3. Choose batches for flow

Prefer the smallest batch that advances a behavior lane and yields useful
evidence. Establish a narrow executable spine early, then extend behavior,
increase dependency fidelity, close defined failure risks, or promote the path.

Parallelize only tasks with satisfied semantic prerequisites and clear ownership
of shared contracts, generated files, migrations, and UI surfaces. A free
worker is not a reason to dispatch speculative work. Limit work in progress
when review, CI, a shared surface, or a real dependency is the actual
bottleneck.

Paid validation is scheduled at the earliest point where the minimal path can
prove a material real-provider property, not on every task and not only at the
end. Reuse valid evidence when its declared evidence key is unchanged.

## 4. Reconcile before adding work

For each non-terminal or unhealthy priority item, classify it as:

- `recoverable`: a cleared transient condition makes one bounded retry useful;
- `stranded`: workflow state is inconsistent and a valid transition can repair
  it;
- `deterministic_blocker`: unchanged evidence predicts the same failure;
- `scope_or_plan_failure`: the task shape, dependency, or criterion is wrong;
- `terminal_healthy`: no action is required.

Retry a recoverable unchanged failure at most once per planning pass, then
inspect the result. Manual state moves repair workflow state; they must not skip
implementation, review, or validation. Deterministic blockers require a narrow
prerequisite, a revised task, or an explicit external condition—not repeated
dispatch.

## 5. Use loopback as a control signal

After a batch completes, compare task evidence and the structured loopback
report with project criteria and current system state. Choose one outcome:

- proceed because prerequisites and evidence support the next slice;
- revise, split, reorder, or replace existing tasks;
- submit a narrow correction or newly discovered prerequisite;
- increase fidelity at a named validation gate;
- close the lane because all outcomes are proven; or
- hold because no safe factory-owned action can resolve the current blocker.

Loopback failure **MUST** produce a delta plan tied to failed behavior and
evidence. Do not ask the validator to fix defects or advance a queue solely
because the loopback fired.

## 6. Continuous improvement and retrospectives

Treat common-cause workflow variation by improving the system and special-cause
failure by correcting its specific cause. Prefer guardrails, deterministic
observations, bounded retries, and explicit ownership over repeated manual
coordination.

At meaningful milestones, record a concise retrospective:

- outcomes completed;
- evidence that established them;
- what improved throughput or confidence;
- failures and whether they were common or special cause;
- mechanisms or standards to change; and
- the next experiment or corrective action with an owner.

Retrospective notes are inputs to future planning, not a mandatory reason to
create unrelated cleanup work.

## Meta-planning checklist

- Current customer outcomes and system state are explicit.
- Unhealthy work was reconciled before new dispatch.
- New work is behavior-shaped, dependency-ready, and evidence-producing.
- The executable spine is established or advanced.
- Shared surfaces and paid/real edges have clear owners and budgets.
- Work in progress reflects the actual bottleneck.
- Loopback evidence changed or confirmed the next decision.
- Holds and retries have concrete reasons and bounded conditions.
- Lessons are recorded without creating speculative scope.
