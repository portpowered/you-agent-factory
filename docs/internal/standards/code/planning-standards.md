# Planning Standards

---
author: andreas abdi
last modified: 2026, may, 27
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
- Call out quality gates directly when the work touches backend, frontend, contracts, or generated artifacts.

## Review Checklist

Before a PRD or story breakdown is accepted, reviewers **SHOULD** confirm:

- The plan describes the customer problem, the specific behavior gap, and the intended outcome.
- Each story corresponds to one primary observable behavior or one tightly bounded enabling behavior.
- Stories are sequenced so they can be implemented and reviewed in a stable order.
- Acceptance criteria are concrete, behavior-focused, and testable.
- The plan names the right verification surfaces such as unit, integration, functional, contract, UI, or stress coverage where relevant.
- Complex frontend plans distinguish canonical data from projected UI-library state and name the operations that mutate canonical state.
- The plan does not widen into unrelated cleanup, broad rewrites, or inventory work unless the customer ask explicitly requires it.
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

### 3. Make Acceptance Criteria Reviewable

Acceptance criteria **MUST** be specific enough that a reviewer or implementing agent can tell when the story is done.

Rules:

- Criteria **MUST** describe outcomes, not vague intent.
- Criteria **SHOULD** mention concrete regression cases, paths, or surfaces when known.
- Criteria **SHOULD** describe both happy-path and relevant failure-path behavior when the risk warrants it.
- Criteria **MUST** avoid ambiguous language such as "clean up," "improve," or "fix" without naming the observable result.
- Quality gates such as `Tests pass`, `Typecheck passes`, generated-artifact verification, or lint checks **SHOULD** appear when relevant, but they **MUST NOT** be the only acceptance criteria.

### 4. Reflect Repository Standards in the Plan

Planning **MUST** encode the expectations that downstream implementation and review will enforce.

Rules:

- Plans **MUST** align with `docs/internal/standards/code/code-review-standards.md`.
- Backend-affecting plans **MUST** account for architecture, state, contract, and test expectations from `docs/internal/standards/code/general-backend-standards.md`.
- Frontend-affecting plans **MUST** account for state, accessibility, responsive behavior, and testing expectations from `docs/internal/standards/code/general-website-standards.md`.
- When a change touches generated artifacts or public contracts, the plan **MUST** call out contract alignment and generated-output expectations explicitly.
- AI-authored plans **MUST** be written with the expectation of extra implementation and review scrutiny.

### 5. Plan Complex Frontend Data Boundaries

Complex frontend plans **MUST** define source-of-truth and projection boundaries before implementation starts.

Rules:

- Plans **MUST** name the canonical API or domain model when one exists.
- Plans **MUST** distinguish durable or editable state from UI-library projection state, such as graph nodes, table rows, chart series, canvas geometry, or drag-surface state.
- Plans **MUST** name the feature operations or service methods that own mutations when the feature includes behaviors such as add, remove, connect, disconnect, reorder, validate, filter, or save.
- Plans **SHOULD** describe how components or hooks consume those operations without turning component state into the domain source of truth.
- Plans **SHOULD** call out replacement or removal expectations for old compatibility paths when a new canonical path is introduced.
- Frontend UI plans **SHOULD** name existing shared primitives to reuse for standard actions, dialogs, popovers, form controls, tables, shells, and status treatments. New bespoke controls should be justified as reusable primitives.

### 6. Prefer Dependency-Aware Sequencing

Stories **MUST** be ordered so implementation can proceed without unnecessary blocking or churn.

Rules:

- Early stories **SHOULD** establish the canonical behavior or contract that later stories depend on.
- Later stories **SHOULD** extend that behavior into adjacent surfaces or regression proof.
- The plan **SHOULD NOT** force reviewers to approve speculative later work before the core behavior is defined.
- If a story is purely enabling, it **MUST** be narrowly justified and kept smaller than the dependent behavior stories where possible.

### 7. Prove Behavior with the Right Evidence

Plans **MUST** name the evidence needed to trust the change.

Rules:

- Every non-trivial behavior change **MUST** identify the verification layer that best proves it.
- Observable regressions **SHOULD** be proven through direct behavioral tests rather than topology or inventory assertions.
- Plans **SHOULD** prefer focused regression coverage over broad unrelated suite churn.
- When concurrency, contracts, browser behavior, or dependency failure are part of the risk, the plan **MUST** name that verification need explicitly.
- Complex frontend plans **SHOULD** include operation tests, projection tests, hook or mutation tests, focused component tests, and a small number of integration tests for high-risk UI-library behavior when those layers are applicable.
- Plans **SHOULD NOT** rely on mounted component tests as the only proof for domain mutations that can be tested as pure operations.
- Plans involving third-party UI libraries **SHOULD** prove both the pure projected state and at least one user interaction path where the library dispatches the expected operation.

### 8. Keep Planning Output Clean and Actionable

Planning artifacts **MUST** remain implementation-ready and reviewer-friendly.

Rules:

- Titles and descriptions **MUST** be specific enough to stand alone in a queue.
- Story text **SHOULD** name the actor, desired outcome, and reason in plain language.
- Notes **SHOULD NOT** become a dumping ground for speculative implementation detail.
- Plans **MUST NOT** require hidden context that exists only in the original chat when the artifact could state it directly.

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
- Story order supports incremental implementation and review.
