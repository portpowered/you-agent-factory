# Factory review standards

---
author: andreas abdi
last modified: 2026, august, 31
doc-id: FSTD-003
---

This standard governs factory code review and independent validation. It extends
the repository code-review standard with workflow, evidence, and convergence
rules. Reviews that touch tests **MUST** enforce
[testing-standards.md](./testing-standards.md).

## Quick rules

- Review correctness and customer behavior before style.
- Read the plan, task, PR history, diff, affected code, and relevant standards.
- Verify project and task criteria one by one using independent evidence.
- Personally exercise runtime-observable behavior with the delivered artifact
  at the highest safe and authorized fidelity.
- Confirm evidence scope, dependency fidelity, cadence, and cost; reject claims
  beyond what was tested.
- Classify findings as blocking or non-blocking and make every requested change
  specific and actionable.
- Review must converge: repeat passes re-check prior blockers and new changes,
  not continuously expand the scope of an unchanged PR.
- Review owns terminal required CI, conflict resolution requests, and merge.
- Final loopback confirms the integrated customer journey and does not silently
  repair defects.

## 1. Review inputs and order

The reviewer **MUST** inspect, in order:

1. merged state and current PR head;
2. parent plan, task packet, project criteria, and declared unproven edges;
3. prior PR conversation feedback and its resolution state;
4. the complete diff and affected surrounding code;
5. applicable architecture, backend, frontend, contract, testing, and writing
   standards;
6. terminal CI results for the current head; and
7. independent behavioral evidence.

A merged PR is terminal for its lane. A defect found afterward becomes a new
work item rather than reopening the merged lane.

## 2. Evidence and acceptance criteria

For each project and task criterion, the reviewer **MUST** record `PASS`,
`FAIL`, or `BLOCKED`, the evidence, and any remaining unproven edge.

The reviewer **MUST** reject:

- criteria satisfied only by structural presence, compilation, or a generic
  suite pass when observable behavior is required;
- test evidence from another branch, commit, or stale PR head;
- integration claims based only on mocked internal collaborators;
- real-dependency claims proven only with a substitute;
- success claims that omit the relevant output, threshold, or property-specific
  measurement;
- contradictory criteria that both require and exempt the same real edge; and
- weakened or deleted assertions that conceal a regression.

For every changed public interface or configuration shape, the reviewer
**MUST** compare the implementation with the plan's concrete `Current` and
`Proposed` fenced blocks. A prose-only description, field inventory, or diff
without both canonical shapes is a blocking planning defect. The reviewer also
confirms the authored contract source—not a generated file—matches the proposed
shape and that all declared generated outputs and consumers were refreshed.

For runtime-observable CLI, API, UI, event, or lifecycle changes, the reviewer
**MUST** independently exercise a safe, isolated delivered artifact. The proof
records exact commands or steps, output, exit status, environment, and real or
substituted edges. If runtime proof is genuinely not applicable, record the
specific reason.

Paid or externally mutating checks require plan authorization and must remain
within declared budgets. Review does not gain authority to contact production
systems merely because a runtime check exists.

## 3. Review architecture and quality

Reviewers **MUST** check correctness, failure behavior, security, privacy,
accessibility, localization, performance, observability, compatibility,
migration, rollback, generated outputs, and cleanup when those concerns are in
scope. They **MUST** enforce repository package and service boundaries and
reject unrelated broad cleanup or hidden side effects.

Tests should prove behavior at the closest reliable surface. Meta tests that
only scan source topology, docs links, bundle internals, or inventories are not
a substitute for runtime, contract, CLI, API, UI, or event behavior unless the
topology itself is the public contract.

Reviewers **MUST** verify test classification and execution topology: unit
tests remain component-isolated; functional tests assert customer-observable
behavior through public boundaries, use Factory Sessions and a shared root
process where possible, never build a binary, and run in parallel; integration
tests consume a prebuilt artifact and stay small; load/stress and repository
shape checks live in their dedicated suites. Exceptions are blocking unless
the testing standard explicitly permits them and the plan records the required
justification.

## 4. Findings and convergence

Every finding **MUST** state:

- `BLOCKING` or `NON-BLOCKING`;
- the violated behavior, criterion, or standard;
- concrete evidence and reproduction;
- the required correction; and
- why the correction matters.

Blocking findings include correctness defects, security or privacy defects,
missing required behavioral evidence, failed required quality gates caused by
the change, standards violations, and unsatisfied project criteria.

On repeat review, blocking scope **MUST** converge. Re-check unresolved prior
blockers and defects introduced since the last reviewed head. A newly noticed
issue in unchanged code that survived a prior pass is normally a non-blocking
follow-up unless it is a critical security or data-loss defect. Do not repost an
unchanged blocker set when the implementer has received it and the head has not
moved.

## 5. CI, conflicts, and merge

Review owns terminal-and-passing required CI for the current head. Pending CI is
a wait state, not executor rework. Failures caused by untouched baseline code
must be distinguished from failures introduced by the PR and handled through
the repository's baseline-flake policy.

When the PR conflicts with its base, review requests the exact reconciliation
needed and returns it to implementation. When all blocking criteria pass,
required CI is terminal and green, conflicts are resolved, and policy permits,
review merges the PR. Approval or green CI without merge is not lane-wide
completion.

## 6. Validation loopback

The integrated validation loopback uses
[validation-loopback-template.md](./validation-loopback-template.md) from a
clean environment and evaluates the complete customer journey, cross-task
integration, documentation, permissions, usability, persistence, and applicable
non-functional outcomes.

Loopback **MUST** be read-only by default. It reports defects and requests a
delta plan; it does not silently patch implementation. A loopback pass does not
replace task-owned tests, and task-owned tests do not replace loopback's
integrated customer proof.

## Review checklist

- Current head, plan, task, prior feedback, diff, and standards were inspected.
- Every criterion has independent `PASS`, `FAIL`, or `BLOCKED` evidence.
- Evidence matches the claimed scope and dependency fidelity.
- Runtime proof used an isolated delivered artifact when applicable.
- Architecture, contracts, failure behavior, and operational concerns fit.
- Tests are behavioral, focused, and not silently weakened.
- Test layer, boundary, parallelism, artifact ownership, and suite placement
  conform to the factory testing standard.
- Findings are classified, actionable, and convergent.
- CI and conflict status belong to the current head.
- The PR is merged only after all blocking conditions are cleared.
