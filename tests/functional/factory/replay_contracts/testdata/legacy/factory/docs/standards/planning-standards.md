# Factory planning standards

---
author: andreas abdi
last modified: 2026, august, 31
doc-id: FSTD-001
---

This standard governs plans, PRDs, behavior lanes, and task packets produced by
the agent factory. Authors **MUST** use [plan-template.md](./plan-template.md),
[task-template.md](./task-template.md), and the test-layer rules in
[testing-standards.md](./testing-standards.md).

## Quick rules

- Plan around customer-observable or reviewer-verifiable behavior, not files,
  layers, services, or implementation phases.
- Prefer narrow vertical slices. A task may cross contracts, backend, UI,
  tests, and documentation when those changes jointly deliver one behavior.
- Treat service, package, contract, and file inventories as impact analysis,
  not automatic task boundaries.
- Establish a narrow executable spine early and preserve it throughout the
  lane. Build narrowly, but test as deeply as practical.
- Put characterization coverage before structural change when current behavior
  is not adequately protected.
- Every task proves its own increment. Final loopback confirms integration and
  usability; it is not the first serious test.
- Acceptance criteria state observable outcomes and named evidence. Phrases
  such as "works," "nice UX," "stable," or "makes sense" are not criteria.
- Every plan covers relevant failure, compatibility, rollout, rollback,
  observability, security, privacy, accessibility, localization, performance,
  and cost concerns.
- Each structural step leaves `main` releasable and independently revertible.
- The implementation stage stops after its final head is pushed, the PR is
  open, CI has started, and blocking feedback is addressed. Review owns
  terminal CI, conflict resolution, and merge.

## 1. Required problem and decision framing

A plan **MUST** state:

1. the customer problem in one sentence;
2. current behavior and the specific behavior gap;
3. the desired observable outcome and measurable success conditions;
4. the recommended approach in no more than three introductory sentences;
5. scope, non-goals, assumptions, constraints, and unresolved decisions;
6. estimated task or agent-deployment count and the factors that could cause
   replanning.

Alternatives are a decision record, not a quota. Record only credible options,
why they were rejected, and evidence supporting the decision. Do not invent
filler alternatives.

## 2. Customer behavior specification

For customer-facing work, the plan **MUST** describe actors, permissions,
journeys, and the default, loading, empty, success, error, and permission
states. It **MUST** identify accessibility, keyboard, focus, responsive, and
localization behavior when applicable.

Visual references **MUST** use versioned or stable artifact identifiers. A
description such as "make it look like the screenshot" is insufficient without
the referenced artifact and the observable differences from the current UI.

## 3. Contracts, architecture, and state

The plan **MUST** inventory affected contracts and classify each as additive,
breaking, deprecated, or unchanged. Use the native contract format:

- OpenAPI for HTTP APIs;
- command grammar and examples for CLI behavior;
- JSON Schema or the repository's canonical schema for configuration;
- named schemas for events, messages, and persisted records.

Every changed interface or configuration shape **MUST** be rendered in the plan
as concrete `Current` and `Proposed` triple-backticked blocks using its native
language (`yaml`, `json`, `toml`, `graphql`, `proto`, or `text`). Prose, field
lists, tables, and phrases such as "add a property" **MUST NOT** substitute for
the actual before-and-after shapes.

Each pair of blocks **MUST**:

- name the authored source file or contract component it represents;
- include enough surrounding structure to make nesting, required fields,
  types, defaults, validation, and response/error shapes unambiguous;
- preserve unchanged context needed to compare the two versions;
- use `# Not present` in the `Current` block for an addition and `# Removed` in
  the `Proposed` block for a deletion rather than omitting one side;
- show examples as valid examples in the native format rather than prose; and
- be followed by compatibility, migration, consumer, generated-artifact, and
  rollout consequences.

A focused triple-backticked `diff` block **MAY** follow the two canonical blocks
to make a large shape easier to scan, but it does not replace either block.
Generated contracts **MUST** be specified through their authored source shape;
the plan then lists every generated output and consumer that must be refreshed.
When a Markdown plan is converted into structured task JSON, the conversion
**MUST** preserve the exact current and proposed excerpts as separate fields;
it must not collapse them into a prose summary.

OpenAPI is not a universal modeling format. Plans that change a public contract
**MUST** name compatibility behavior, generated artifacts, consumers,
migration, and rollback in addition to the concrete shapes.

Non-trivial plans **MUST** describe current and target runtime flow, service or
module dependencies, canonical state, projections, mutation ownership,
transaction or consistency boundaries, and the removal owner for temporary or
legacy paths. Diagrams are required only when they materially clarify these
relationships. Use normal styling for unchanged elements and a documented
highlight style for additions or modifications.

## 4. Behavior lanes and task decomposition

One parent behavior lane represents one coherent customer or system outcome.
Tasks within the lane **SHOULD** either:

- establish the executable spine;
- extend behavior available through the spine;
- increase dependency fidelity;
- close a defined failure or quality risk; or
- promote the path into a more realistic environment.

Backend-only, frontend-only, contract-only, test-only, or documentation-only
tasks **MUST NOT** be the default split for behavior spanning those surfaces.
A horizontal enabling task is allowed only when it is independently safe and
useful—for example characterization coverage, an additive compatibility seam,
a reusable migration primitive, or test-harness infrastructure. Its task packet
**MUST** explain why it cannot be part of a behavior slice.

Task size is governed by cognitive load, independently verifiable outcome, and
revertability—not a line-count target. A task is too large when it contains
multiple primary behaviors, cannot be reviewed in one focused pass, or depends
on later work to become correct.

## 5. Sequencing structural and replacement work

When existing behavioral coverage is insufficient, characterization tests
**MUST** land before restructuring and preserve current behavior even when the
current behavior appears wrong. Behavior correction is a separate task.

Replacement work **SHOULD** use this sequence:

1. introduce the new path alongside the old path;
2. declare which path is canonical;
3. migrate callers in reviewable batches;
4. remove adapters, flags, and the old path in a dedicated final step.

Every step **MUST** leave `main` releasable and must not rely on a future repair
task. Plans **MUST** name shared-surface ownership and genuine semantic
dependencies. Shared-file contention alone is not a semantic dependency.

## 6. Progressive verification

Each behavior lane **MUST** define an evidence progression from local logic to
the highest practical end-to-end proof. Evidence has four independent
properties:

Test layers, execution boundaries, suite placement, parallelism, artifact
ownership, and case-selection rules **MUST** follow
[testing-standards.md](./testing-standards.md). A plan **MUST NOT** relabel a
test to avoid those constraints.

| Property | Required description |
| --- | --- |
| Scope | unit, functional, integration, or end-to-end |
| Dependency fidelity | none, controlled, schema mock, emulator, local real, remote real, or remote paid |
| Cadence | per change, per PR, risk-triggered, scheduled, or release |
| Cost | free, bounded resource use, or paid with an explicit budget |

Definitions:

- A unit test proves one package-owned component in isolation.
- A functional test proves customer-observable behavior through a stable
  public application boundary while controlling external effects.
- An integration test exercises an already compiled deliverable across a real
  production boundary. Compilation belongs to the invoking build or release
  lane, not to the test.
- A paid integration test crosses a real billable remote boundary and proves
  only properties that require it, such as credentials, endpoint/model
  availability, serialization compatibility, and response decoding.
- An end-to-end test starts at the actual customer entry point and reaches the
  customer-visible outcome through the assembled system.

The first feasible slice **SHOULD** establish a narrow executable spine through
the real customer transport and internally owned components, with substitutes
only at unavailable, unsafe, unresolved, or expensive edges. Final end-to-end
validation **MUST NOT** be the first time major components are assembled.

Contract and schema validation **MUST** accompany every applicable level using
the canonical schema format for that boundary. A higher-level pass does not
replace focused lower-level evidence when the lower level provides faster or
more precise regression localization.

Every task **MUST** state:

- its behavioral witness;
- its executable-spine effect: `establish`, `preserve`, `extend`,
  `increase_fidelity`, or `promote`;
- the exact evidence it will produce and what each item proves;
- the highest feasible verification level;
- remaining unproven edges and the later gate that owns each edge.

Paid verification **MUST** declare its trigger, maximum calls, maximum cost,
maximum duration, fixture, output validator, and evidence-reuse key. The reuse
key includes the commit or build, contract version, provider and API version,
model or deployment, region, fixture, target environment, and relevant
configuration hash. Failure matrices normally use deterministic fault injection
rather than paid calls. Final end-to-end validation may reuse a paid invocation
only when it exercises the same build, environment, real dependency, and actual
customer entry point and produces both provider and customer-visible evidence.
Evidence **MUST NOT** claim a property beyond its tested scope or dependency
fidelity.

## 7. Acceptance criteria

Acceptance criteria **MUST** use concrete observable conditions. Given/when/then
wording is recommended for behavior and failure cases. Each criterion **MUST**
name either direct evidence or the verification gate that owns it.

Project criteria **MUST** distinguish:

- product behavior;
- relevant failure behavior;
- measurable non-functional thresholds;
- operational, security, accessibility, and compatibility outcomes;
- required quality gates;
- integrated clean-room validation; and
- delivery state.

Quality gates are necessary when relevant but cannot be the only criteria.
"Tests pass" is incomplete without naming the test or suite and the property it
measures. Evidence about CI **MUST** come from the change's own PR and must be
recorded in a PR comment, never a commit.

## 8. Failure modes and operational readiness

Every plan **MUST** include a failure-mode matrix covering applicable bad input,
authorization, dependency timeout or outage, partial completion, concurrency,
cancellation, capacity, persistence, migration, and recovery cases. For each
case, name detection, customer-visible behavior, state outcome, retry or
recovery policy, telemetry, and proof.

The plan **MUST** define applicable performance and scale assumptions,
reliability targets, security and privacy boundaries, cost constraints,
structured logs, metrics, traces, alert conditions, rollout stages, stop
conditions, rollback procedure, compatibility interval, and cleanup owner.

## 9. Task graph and validation loopback

Plans **MUST** include a task dependency graph or explicit dependency table.
Dependencies represent semantic prerequisites, not a preferred layer order.
Parallel work must name shared-surface ownership.

Each task proves its own increment at the closest reliable surface. The final
validation loopback independently runs the full customer journey from a clean
environment, checks cross-task integration, documentation usability, and
non-functional project criteria, and emits the structured report in
[validation-loopback-template.md](./validation-loopback-template.md).

The loopback is read-only by default. It **MUST NOT** silently fix defects. A
failure produces evidence and a delta-plan request that the meta-planner uses
to revise, split, reorder, or add work.

## 10. Delivery loop

Every plan **MUST** include this responsibility split:

> Implementation-stage delivery criterion: The implementation stage marks this criterion satisfied and stops after its final head is pushed, the PR is open, CI has started, and all blocking review feedback is addressed. It does not poll or re-check CI after this finish line. The review stage owns driving CI to terminal-and-passing, resolving merge conflicts, and merging the PR; merge remains the lane-wide delivery boundary. CI-run evidence goes in a PR comment and never in a commit.

Implementation completion is a handoff, not lane completion. Review and the
factory workflow continue until merge or a structured blocked outcome.

## 11. Source-plan alignment

When work derives from a governing source plan (a Project's `sourcePlan` or an
idea payload that names one), that plan file is the source of truth and the
PRD is a derived execution artifact:

- The PRD **MUST** name the source plan path in `context.sourcePlan`.
- Every task **MUST** carry a `sourcePlanRef` naming the plan section or
  requirement it implements. Work no plan section accounts for is an explicit
  open question or contract-delta request, never a silent addition.
- The PRD **MUST NOT** weaken, reinterpret, or drop source-plan requirements.
  A genuine conflict between the plan and repository reality is recorded and
  surfaced for the Project Lead or operator to resolve; the planner does not
  resolve it by rewording the requirement.
- Reviewers verify delivered behavior against the referenced plan sections,
  not only against the PRD's own restatement.

## 12. Functional test-case specification

A task that adds or changes functional tests **MUST** enumerate its complete
intended case matrix in the plan, not delegate case discovery to the
implementer:

The matrix contains distinct customer behaviors, not every internal branch or
inventory entry. The plan **MUST** also name the Factory Session strategy,
shared `root.BuildProcess` ownership, testable external boundaries, parallel
isolation model, and the customer-visible invariant behind any required
serialization, as required by
[testing-standards.md](./testing-standards.md).

- every materially distinct happy customer behavior, actor, and journey with
  its observable outcome; input shapes that produce the same behavior use a
  representative case rather than an inventory;
- every materially distinct customer-visible unhappy behavior that applies,
  such as authorization, dependency failure/timeout, partial completion,
  cancellation, persistence, or recovery; pure validation branches stay in
  unit tests; and
- boundary cases such as empty, minimum, maximum, duplicate, ordering, and
  idempotency only where the public contract gives that boundary distinct
  customer behavior.

Each case is written given/when/then with the observable result it asserts.
"Add functional tests for X" without the selected behavioral matrix is not a
plannable task. The matrix is the review contract: a delivered test suite is
measured against the selected distinct behaviors, and an intentionally omitted
behavior is named with its owning lower or later gate rather than silently
skipped.

## Plan review checklist

- The problem, current behavior, desired outcome, scope, and decision are clear.
- Tasks are behavior slices or justified bounded enablers.
- The executable spine appears early and every task preserves it.
- Acceptance criteria are observable, measurable, and evidence-backed.
- Current coverage is measured and characterization precedes structural change.
- Contracts, state, ownership, compatibility, rollout, and removal are explicit.
- Failure, security, privacy, accessibility, localization, performance, cost,
  and observability concerns are handled when applicable.
- Each unproven edge has a named later gate.
- Paid validation is bounded and risk-triggered.
- Task dependencies and shared-surface ownership are explicit.
- Task, reviewer, and loopback responsibilities do not overlap ambiguously.
- Delivery responsibility matches the canonical implementation/review split.
- Every task traces to its source-plan section when a source plan governs.
- Functional-test work enumerates its full happy/unhappy/boundary case matrix.
- Every test is classified and designed according to the factory testing
  standard, including sessions, process reuse, parallelism, artifact ownership,
  and dedicated load/static-check placement.
