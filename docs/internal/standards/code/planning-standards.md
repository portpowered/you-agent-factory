# Planning Standards

---
author: andreas abdi
last modified: 2026, august, 14
doc-id: STD-018
---

This document defines the baseline standards for planning work into PRDs, acceptance criteria, and user stories in this repository. It is intended for agents and contributors who turn customer asks into executable work.

## Usage

Every contributor or agent who creates or updates a PRD, `prd.json`, or work-story breakdown **MUST** review this standard before planning.

## Quick Rules

- Plan around observable behavior, not around source files, layers, or refactor impulses.
- Each work story **SHOULD** map to roughly one independently understandable observable behavior.
- Keep stories vertically sliced and independently reviewable, implementable, and testable whenever practical.
- Acceptance criteria **MUST** describe outcomes a reviewer can verify, not hidden implementation details.
- Every plan **MUST** reflect the repository's review and engineering standards, including correctness, architecture fit, readability, and test evidence.
- Complex frontend plans **MUST** identify canonical state, operation/service boundaries, projection boundaries, and the evidence that proves each layer.
- Frontend plans **SHOULD** prefer existing shared UI primitives and concise action/copy patterns unless a new reusable primitive is justified.
- Avoid bundling unrelated cleanup, opportunistic refactors, or broad topology changes into a behavior-focused lane.
- Structural change such as a refactor, migration, extraction, or removal **MUST** be decomposed into steps that each merge on their own and each leave `main` releasable.
- Replacing an existing path **SHOULD** follow a strangler shape: introduce the new path, migrate callers in reviewable batches, then delete the old path in a dedicated final step.
- When coverage of the behavior being restructured is insufficient, characterization tests **MUST** land first as their own step, before the structure changes.
- Bundle changes only when they must merge together, never because they happen to touch the same file.
- Call out quality gates directly when the work touches backend, frontend, contracts, or generated artifacts.
- Every implementation plan **MUST** state a two-stage delivery loop:
  implementation marks its delivery criterion satisfied and stops after its
  final head is pushed, the PR is open, CI has started, and all blocking review
  feedback is addressed; review then owns terminal-and-passing CI, conflict
  resolution, and merge, with merge remaining the lane-wide completion
  boundary.
- Implementation **MUST NOT** poll or re-check CI after its finish line, and
  CI-run evidence **MUST** go in a PR comment, never a commit.

### Task shaping
- Every task when creating tasks for submission into sub workers/the You Agent Factory must match the template structure denoted in docs/internal/standards/templates/task-templates.md
- Every task subshape that you implement that is given to a subagent should be small enough that we can submit it to an agent that is very dumb and they should be able to roughly do it.

## Review Checklist

Before a PRD or story breakdown is accepted, reviewers **SHOULD** confirm:

- The plan describes the customer problem, the specific behavior gap, and the intended outcome.
- Each story corresponds to one primary observable behavior or one tightly bounded enabling behavior.
- Stories are sequenced so they can be implemented and reviewed in a stable order.
- Acceptance criteria are concrete, behavior-focused, and testable.
- The plan names the right verification surfaces such as unit, integration, functional, contract, UI, or stress coverage where relevant.
- Complex frontend plans distinguish canonical data from projected UI-library state and name the operations that mutate canonical state.
- The plan does not widen into unrelated cleanup, broad rewrites, or inventory work unless the customer ask explicitly requires it.
- Structural change is broken into steps that each merge independently, and no step depends on a later one to restore behavior it breaks.
- Replacement work names the canonical path, the caller migration batches, and the step that deletes the old path.
- The plan states measured coverage of the behavior being restructured, and adds characterization tests as an earlier step when that coverage is insufficient.
- Stories are separable in review, revert, and scheduling, and shared surfaces name an owning lane.
- The work respects repository architecture and dependency boundaries.

## Regulations

### 1. Plan Around Observable Behavior

Plans **MUST** be organized around externally observable behavior, user-visible outcomes, or reviewer-verifiable system behavior.

Rules:

- A story **SHOULD** describe one primary behavior change.
- If a change cannot be expressed as observable behavior, the planner **MUST** explain why it is a necessary enabling step.
- Acceptance criteria **MUST NOT** rely only on internal helper creation, file motion, or source reorganization as proof of completion.
- Behavioral wording **SHOULD** dominate over implementation wording.

Examples of good planning units:

- a CLI command reports the right status for a defined input case
- an API surface rejects an invalid contract shape with a specific outcome
- a dashboard view renders the corrected summary for a known regression case

Examples of weak planning units:

- move code into three files
- create helper types for parser cleanup
- refactor module ownership without a concrete behavior target

### 2. Keep Stories Narrow, Cohesive, and Vertically Sliced

Work stories **MUST** stay small enough to understand quickly and broad enough to produce a coherent result.

Rules:

- Each story **SHOULD** target roughly one observable behavior.
- Stories **SHOULD** be vertically sliced across layers when that is the smallest way to deliver the behavior safely.
- Splitting by backend-only, frontend-only, and tests-only lanes **SHOULD NOT** be the default when one behavior spans those layers.
- Separate stories **SHOULD** be used when behaviors are independently valuable, independently reviewable, or carry different risk.
- Opportunistic cleanups, naming sweeps, or broad debt removal **MUST NOT** be attached unless they are required for the target behavior.

### 3. Decompose Structural Change Into Independently Mergeable Steps

Refactors, migrations, extractions, removals, and renames **MUST** be planned as a sequence of steps that each land on their own. A structural change that is only correct once every part of it is finished **MUST NOT** be planned as a single work item.

A plan is too large and **SHOULD** be split before implementation starts when any of these is true:

- No step in it can be merged without the others.
- It cannot be described without listing several distinct behaviors it must preserve.
- Reverting it would revert unrelated behavior.
- A reviewer cannot hold the before and after shapes in mind at the same time.

Rules:

- Each step **MUST** leave `main` releasable with every existing behavior intact.
- Each step **MUST** be correct on its own. A step **MUST NOT** depend on a later step to restore behavior it breaks.
- Steps **SHOULD** be ordered so the riskiest structural move is the smallest and most isolated one.
- A plan **MUST NOT** contain a step whose purpose is to repair fallout from an earlier step. Fallout is evidence the decomposition was wrong, not a work item.
- The plan **SHOULD** state how many steps it expects and what each one delivers, so an implementation that stalls or expands is visible early rather than at review.

### 4. Prefer Strangler-Style Replacement Over In-Place Rewrites

When a plan replaces an existing implementation, path, or boundary, it **SHOULD** introduce the replacement alongside the original and migrate to it incrementally rather than rewriting in place.

Rules:

- The plan **SHOULD** be shaped as: introduce the new path, migrate callers in reviewable batches, then delete the old path in a final dedicated step.
- Introducing a new path and deleting the old one **MUST NOT** be planned as a single step when callers exist outside the changed package.
- While both paths exist, the plan **MUST** name which one is canonical so implementers do not add callers to the path being retired.
- The deletion step **MUST** be planned explicitly. A migration with no scheduled removal leaves two live paths and **MUST NOT** be described as complete.
- When a temporary adapter, shim, or compatibility layer is required, the plan **MUST** name what removes it and in which step.
- If a change genuinely cannot be staged behind a parallel path, the plan **MUST** say so and justify it rather than leaving that choice implicit.

### 5. Establish Behavioral Coverage Before Changing Structure

A plan **MUST NOT** depend on tests written after a restructure when existing coverage is insufficient to detect a regression in the behavior being restructured.

Rules:

- The plan **MUST** state the current coverage of the affected behavior and how that was measured.
- When coverage is insufficient, the plan **MUST** add characterization tests that pin the existing behavior as an earlier, separate step, merged before the structural change begins.
- Characterization tests **MUST** describe behavior as it is today, including behavior that looks wrong. Correcting that behavior is separate work with its own story and its own acceptance criteria.
- Structural steps **SHOULD** be reviewable as changes that leave the test suite untouched. A structural step that also rewrites many test expectations **SHOULD** be replanned as a behavior change.
- When a step does change what a test asserts, the plan or the change **MUST** state which assertions changed and why that change was intended, so a silently relaxed contract is not mistaken for a passing refactor.

### 6. Prefer Clean Boundaries Over Coupled Work

Plans **SHOULD** separate work along real boundaries rather than bundling changes because they happen to touch the same code at the same time.

Rules:

- Stories **SHOULD** be separable in review, in revert, and in scheduling. Two changes that must merge together **SHOULD** be one story; two that need not **SHOULD NOT** be.
- A plan **MUST NOT** couple an unrelated behavior change to a structural step because the structural step already touches the file.
- When a step requires a new abstraction, the plan **MUST** state what that abstraction hides and which callers depend on it, so review can judge whether it earns its place.
- New abstractions **SHOULD** be introduced with the behavior that needs them, not ahead of it in a speculative enabling story.
- When several lanes will touch one surface, the plan **MUST** name the owning lane for shared changes so the same fix is not implemented independently in each.

### 7. Make Acceptance Criteria Reviewable

Acceptance criteria **MUST** be specific enough that a reviewer or implementing agent can tell when the story is done.

Rules:

- Criteria **MUST** describe outcomes, not vague intent.
- Criteria **SHOULD** mention concrete regression cases, paths, or surfaces when known.
- Criteria **SHOULD** describe both happy-path and relevant failure-path behavior when the risk warrants it.
- Criteria **MUST** avoid ambiguous language such as "clean up," "improve," or "fix" without naming the observable result.
- Quality gates such as `Tests pass`, `Typecheck passes`, generated-artifact verification, or lint checks **SHOULD** appear when relevant, but they **MUST NOT** be the only acceptance criteria.

### 8. Reflect Repository Standards in the Plan

Planning **MUST** encode the expectations that downstream implementation and review will enforce.

Rules:

- Plans **MUST** align with `docs/internal/standards/code/code-review-standards.md`.
- Backend-affecting plans **MUST** account for architecture, state, contract, and test expectations from `docs/internal/standards/code/general-backend-standards.md`.
- Frontend-affecting plans **MUST** account for state, accessibility, responsive behavior, and testing expectations from `docs/internal/standards/code/general-website-standards.md`.
- When a change touches generated artifacts or public contracts, the plan **MUST** call out contract alignment and generated-output expectations explicitly.
- AI-authored plans **MUST** be written with the expectation of extra implementation and review scrutiny.

### 9. Plan Complex Frontend Data Boundaries

Complex frontend plans **MUST** define source-of-truth and projection boundaries before implementation starts.

Rules:

- Plans **MUST** name the canonical API or domain model when one exists.
- Plans **MUST** distinguish durable or editable state from UI-library projection state, such as graph nodes, table rows, chart series, canvas geometry, or drag-surface state.
- Plans **MUST** name the feature operations or service methods that own mutations when the feature includes behaviors such as add, remove, connect, disconnect, reorder, validate, filter, or save.
- Plans **SHOULD** describe how components or hooks consume those operations without turning component state into the domain source of truth.
- Plans **SHOULD** call out replacement or removal expectations for old compatibility paths when a new canonical path is introduced.
- Frontend UI plans **SHOULD** name existing shared primitives to reuse for standard actions, dialogs, popovers, form controls, tables, shells, and status treatments. New bespoke controls should be justified as reusable primitives.

### 10. Prefer Dependency-Aware Sequencing

Stories **MUST** be ordered so implementation can proceed without unnecessary blocking or churn.

Rules:

- Early stories **SHOULD** establish the canonical behavior or contract that later stories depend on.
- Later stories **SHOULD** extend that behavior into adjacent surfaces or regression proof.
- The plan **SHOULD NOT** force reviewers to approve speculative later work before the core behavior is defined.
- If a story is purely enabling, it **MUST** be narrowly justified and kept smaller than the dependent behavior stories where possible.

### 11. Prove Behavior with the Right Evidence

Plans **MUST** name the evidence needed to trust the change.

Rules:

- Every non-trivial behavior change **MUST** identify the verification layer that best proves it.
- Observable regressions **SHOULD** be proven through direct behavioral tests rather than topology or inventory assertions.
- Plans **SHOULD** prefer focused regression coverage over broad unrelated suite churn.
- When concurrency, contracts, browser behavior, or dependency failure are part of the risk, the plan **MUST** name that verification need explicitly.
- Complex frontend plans **SHOULD** include operation tests, projection tests, hook or mutation tests, focused component tests, and a small number of integration tests for high-risk UI-library behavior when those layers are applicable.
- Plans **SHOULD NOT** rely on mounted component tests as the only proof for domain mutations that can be tested as pure operations.
- Plans involving third-party UI libraries **SHOULD** prove both the pure projected state and at least one user interaction path where the library dispatches the expected operation.
- CI evidence cited by a plan **MUST** come from a run on the change's own pull request. Evidence from another branch, commit, or `main` **MUST NOT** substitute for that run.
- A cited gate **MUST** emit property-specific output; absence of output about the property **MUST NOT** be treated as a pass. When a gate can fail before observing the property, the plan **MUST** require confirmation that measurement occurred before citing the result.
- Counted-ratchet evidence **MUST** report the observed count against the recorded baseline; matching failing-target identity **MUST NOT** substitute for that comparison.
- Acceptance criteria for instrument-specific metrics **MUST** name the instrument or lane that produces the relevant measurement.

**Measurement and failure conditions:** A gate's measurement condition **MUST** equal its failure condition. A result cannot establish a passing property when the gate can fail before measuring it; this distinction addresses the silent and early-exit patterns that motivated this rule.

### 12. Keep Planning Output Clean and Actionable

Planning artifacts **MUST** remain implementation-ready and reviewer-friendly.

Rules:

- Titles and descriptions **MUST** be specific enough to stand alone in a queue.
- Story text **SHOULD** name the actor, desired outcome, and reason in plain language.
- Notes **SHOULD NOT** become a dumping ground for speculative implementation detail.
- Plans **MUST NOT** require hidden context that exists only in the original chat when the artifact could state it directly.

### 13. Make Merge the Delivery Boundary

Implementation plans **MUST** make the end-to-end delivery condition explicit
while writing the project-level delivery criterion from the perspective of the
stage that evaluates it.

Rules:

- The project-level delivery criterion **MUST** lead with this implementation
  finish line: the implementation stage marks the criterion satisfied and
  stops after its final head is pushed, the PR is open, CI has started, and all
  blocking review feedback is addressed.
- The criterion **MUST** state that implementation does not poll or re-check CI
  after that finish line. The review stage owns driving CI to
  terminal-and-passing, resolving merge conflicts, and merging the PR; merge
  remains the lane-wide delivery boundary.
- The implementation/review cycle **MUST** continue through those review-owned
  outcomes until the PR is merged. The implementation stage's finish line is
  not lane-wide completion: opening a PR, pushing the latest implementation,
  obtaining approval, or reaching green CI without merge **MUST NOT** be
  described as completing the lane.
- A delivery criterion **MUST NOT** open with “delivery continues until ...
  merged” or equivalent merge-first wording. The implementation stage
  evaluates the criterion but cannot merge, so a merge-first condition makes it
  wait on a review-owned outcome and can trigger repeated redispatch until the
  executor-loop breaker ends the lane.
- CI-run evidence **MUST** go in a PR comment and never in a commit.
- Shared-file or baseline churn **MUST** be reconciled through the same delivery
  loop when the change remains in scope; it is not by itself a reason to hold an
  otherwise dependency-ready plan.
- The merge condition belongs in project-level acceptance criteria or an
  equivalent delivery section. It **SHOULD NOT** be modeled as a fake product
  behavior story.

Canonical criterion example:

> Implementation-stage delivery criterion: The implementation stage marks this criterion satisfied and stops after its final head is pushed, the PR is open, CI has started, and all blocking review feedback is addressed. It does not poll or re-check CI after this finish line. The review stage owns driving CI to terminal-and-passing, resolving merge conflicts, and merging the PR; merge remains the lane-wide delivery boundary. CI-run evidence goes in a PR comment and never in a commit.

## Delivery Checklist

Before handing a plan to implementation, authors **SHOULD** confirm:

- The problem statement, behavior gap, and intended outcome are explicit.
- Each story approximates one observable behavior or one tightly bounded enabling step.
- Acceptance criteria are concrete and reviewer-verifiable.
- The plan names the right quality gates and test evidence.
- Backend, frontend, contract, and generated-artifact expectations are called out where relevant.
- Complex frontend plans identify canonical state, operations, projections, component wiring, and old-path cleanup where applicable.
- UI plans reuse shared primitives or justify new reusable primitives.
- Scope stays narrow and avoids unrelated cleanup.
- Structural change is decomposed into independently mergeable steps, with the expected step count and each step's deliverable stated.
- Replacement work is staged behind a parallel path, or the plan justifies why it cannot be.
- Characterization tests for insufficiently covered behavior are scheduled before the structural steps that depend on them.
- Story order supports incremental implementation and review.
- The plan explicitly continues through terminal green CI, resolved blocking
  feedback and conflicts, and verified PR merge.
