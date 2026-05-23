# PRD: Multi-Runner Model Support

---
author: Codex
last modified: 2026, may, 18
status: draft
---

## Context

### Customer Ask

Extend infinite-you beyond the current Codex-oriented model runner so customers can orchestrate work across additional model runner ecosystems such as Gemini, Kiro, Cursor CLI, and OpenCode. The first release should include a generic pluggable runner interface and initial support for those named runners across the backend, CLI, API surface, and UI visibility or configuration flows.

### Problem

The current model execution path is too tightly coupled to one runner family. Teams that already standardize on Gemini, Kiro, Cursor CLI, or OpenCode cannot use infinite-you as their scheduling and orchestration layer without changing their toolchain. That creates adoption friction, increases vendor lock-in, and prevents the factory from routing work to the best available runner for a given environment. The system also lacks a clear contract for which runner capabilities are shared, which are optional, and how operator-facing surfaces should describe runner differences.

### Solution

Introduce a runner abstraction layer that separates infinite-you's orchestration logic from any one model runner implementation. Define a common baseline capability contract that all supported runners must satisfy, then allow optional runner-specific capabilities to be represented without forcing full parity across every provider. In v1, ship the abstraction and built-in support for Codex, Gemini, Kiro, Cursor CLI, and OpenCode, with matching backend runtime behavior, CLI configuration and selection flows, API contract support, and UI visibility for available or selected runners.

## Project Acceptance Criteria

- [ ] The system supports a generic pluggable runner interface instead of hard-coding orchestration behavior to a single runner implementation.
- [ ] The first release includes runner implementations or adapters for Codex, Gemini, Kiro, Cursor CLI, and OpenCode.
- [ ] Backend scheduling and execution flows can target any configured supported runner without forking the core orchestration path.
- [ ] CLI, API, and UI surfaces expose runner identity and selection behavior consistently enough for an operator to understand which runner will execute work.
- [ ] The system defines a documented baseline capability set that every supported runner must implement.
- [ ] The system supports optional runner-specific capabilities without breaking the baseline orchestration flow for runners that do not implement them.
- [ ] Lack of full feature parity across all runners does not block v1, provided unsupported capabilities are surfaced clearly and safely.
- [ ] Billing, provider account provisioning, and external procurement workflows remain out of scope for this feature.
- [ ] Typecheck, lint, generated contract checks, and relevant backend, CLI, API, and UI tests pass.

## Goals

- Reduce lock-in to the current Codex-only runner path.
- Let customers keep their preferred model runner ecosystem while using infinite-you for orchestration.
- Create one reusable runner abstraction that supports current and future model runners.
- Add first-class support for Codex, Gemini, Kiro, Cursor CLI, and OpenCode in the product's main surfaces.
- Make shared versus runner-specific capabilities explicit so operators can reason about behavior safely.
- Avoid requiring full feature parity across all supported runners in the first release.

## User Stories

### US-001: Define a pluggable runner contract (P1)
**Description:** As a backend developer, I want a stable runner interface so that orchestration logic can execute work through different model runners without embedding runner-specific behavior into the scheduler core.

**Acceptance Criteria:**
- [ ] A shared runner contract exists in a backend-owned package with explicit request, response, lifecycle, and error semantics.
- [ ] The scheduler and execution layers depend on the runner contract rather than directly on Codex-specific implementation details.
- [ ] The contract defines the minimum baseline capabilities required for a runner to participate in standard orchestration flows.
- [ ] Optional capabilities are modeled explicitly so unsupported features do not require fake or misleading implementations.
- [ ] Contract documentation explains which behavior is required, optional, or runner-specific.
- [ ] Typecheck passes
- [ ] Tests pass

### US-002: Register and resolve multiple runner implementations (P1)
**Description:** As a platform operator, I want the system to resolve runner implementations by ID so that work can be routed to the intended model runner predictably.

**Acceptance Criteria:**
- [ ] The system supports registering multiple runner implementations under stable IDs.
- [ ] The runtime can resolve a configured runner by ID during work planning or execution.
- [ ] Unknown runner IDs fail with a clear validation or configuration error before execution begins.
- [ ] The default runner behavior remains intentional and documented for environments that do not specify a runner explicitly.
- [ ] Integration coverage proves that two or more runner implementations can coexist in the same process configuration.
- [ ] Typecheck passes
- [ ] Tests pass

### US-003: Add Gemini runner support (P1)
**Description:** As a customer already using Gemini, I want infinite-you to execute eligible work through a Gemini runner so that I can keep my existing model workflow.

**Acceptance Criteria:**
- [ ] A Gemini runner implementation or adapter can be configured and selected by stable runner ID.
- [ ] Baseline orchestration flows succeed when targeting the Gemini runner.
- [ ] Unsupported Gemini-specific gaps relative to the baseline are rejected or surfaced clearly instead of failing silently.
- [ ] Integration or functional coverage proves an end-to-end work execution path using the Gemini runner.
- [ ] Typecheck passes
- [ ] Tests pass

### US-004: Add Kiro runner support (P1)
**Description:** As a customer already using Kiro, I want infinite-you to execute eligible work through a Kiro runner so that I can keep my existing model workflow.

**Acceptance Criteria:**
- [ ] A Kiro runner implementation or adapter can be configured and selected by stable runner ID.
- [ ] Baseline orchestration flows succeed when targeting the Kiro runner.
- [ ] Unsupported Kiro-specific gaps relative to the baseline are rejected or surfaced clearly instead of failing silently.
- [ ] Integration or functional coverage proves an end-to-end work execution path using the Kiro runner.
- [ ] Typecheck passes
- [ ] Tests pass

### US-005: Add Cursor CLI runner support (P1)
**Description:** As a customer already using Cursor CLI, I want infinite-you to execute eligible work through Cursor CLI so that I can keep my existing model workflow.

**Acceptance Criteria:**
- [ ] A Cursor CLI runner implementation or adapter can be configured and selected by stable runner ID.
- [ ] Baseline orchestration flows succeed when targeting the Cursor CLI runner.
- [ ] Unsupported Cursor CLI-specific gaps relative to the baseline are rejected or surfaced clearly instead of failing silently.
- [ ] Integration or functional coverage proves an end-to-end work execution path using the Cursor CLI runner.
- [ ] Typecheck passes
- [ ] Tests pass

### US-006: Add OpenCode runner support (P1)
**Description:** As a customer already using OpenCode, I want infinite-you to execute eligible work through OpenCode so that I can keep my existing model workflow.

**Acceptance Criteria:**
- [ ] An OpenCode runner implementation or adapter can be configured and selected by stable runner ID.
- [ ] Baseline orchestration flows succeed when targeting the OpenCode runner.
- [ ] Unsupported OpenCode-specific gaps relative to the baseline are rejected or surfaced clearly instead of failing silently.
- [ ] Integration or functional coverage proves an end-to-end work execution path using the OpenCode runner.
- [ ] Typecheck passes
- [ ] Tests pass

### US-007: Preserve Codex support behind the shared abstraction (P1)
**Description:** As an existing customer, I want Codex support to continue working through the new abstraction so that multi-runner adoption does not regress current behavior.

**Acceptance Criteria:**
- [ ] The current Codex execution path is moved behind the shared runner contract.
- [ ] Existing Codex-backed workflows continue to function without requiring operator migration to a new runner by default.
- [ ] Regression coverage proves Codex can still execute the baseline orchestration flow after the abstraction lands.
- [ ] Typecheck passes
- [ ] Tests pass

### US-008: Configure runner selection through CLI and config (P1)
**Description:** As an operator, I want to configure and select runners from CLI and factory configuration so that I can control which runner executes work without editing source code.

**Acceptance Criteria:**
- [ ] CLI commands and configuration files support specifying a runner ID for relevant execution flows.
- [ ] CLI help and configuration docs list supported runner IDs and explain default behavior.
- [ ] Invalid runner selections fail with actionable error messages.
- [ ] Configuration parsing validates required per-runner settings before execution starts.
- [ ] Typecheck passes
- [ ] Tests pass

### US-009: Expose runner selection and visibility in the API (P1)
**Description:** As an API consumer, I want runner information represented in the public contract so that automation and external clients can inspect or specify runner selection predictably.

**Acceptance Criteria:**
- [ ] The OpenAPI contract includes runner-related request and response fields where runner selection or visibility is required.
- [ ] Generated API artifacts remain aligned with the new runner contract fields.
- [ ] API validation rejects unknown or unavailable runner IDs consistently.
- [ ] API responses expose enough runner metadata for clients to understand which runner is configured or was used.
- [ ] Typecheck passes
- [ ] Tests pass

### US-010: Show runner availability and selection in the UI (P2)
**Description:** As a website user, I want to see available runners and the selected runner so that I can understand and control execution targets from the product UI.

**Acceptance Criteria:**
- [ ] The UI shows available runners wherever runner selection is part of the relevant user flow.
- [ ] The UI shows the currently selected or configured runner clearly.
- [ ] Loading, empty, error, and success states are present for runner-backed UI surfaces.
- [ ] Unsupported runner-specific options are hidden or disabled when the selected runner does not implement them.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill
- [ ] Tests pass

### US-011: Surface capability differences safely (P1)
**Description:** As an operator, I want the system to tell me when a selected runner does not support an optional capability so that I can avoid ambiguous failures.

**Acceptance Criteria:**
- [ ] The system defines a machine-readable capability model for baseline and optional runner features.
- [ ] Selecting a flow that requires an unsupported optional capability fails early with a clear explanation.
- [ ] UI and CLI messaging distinguish between supported, unsupported, and unavailable capabilities where relevant.
- [ ] Tests cover at least one positive and one negative capability-resolution case.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

1. FR-1: The system must define a stable runner abstraction that decouples orchestration logic from any one runner implementation.
2. FR-2: The system must preserve Codex support under the shared runner abstraction.
3. FR-3: The system must support runner registration and resolution by stable runner ID.
4. FR-4: The system must provide first-release support for Codex, Gemini, Kiro, Cursor CLI, and OpenCode.
5. FR-5: The system must define a baseline capability set required for standard orchestration participation.
6. FR-6: The system must model optional runner-specific capabilities separately from the baseline capability set.
7. FR-7: The backend must route work execution through the selected runner without duplicating scheduler logic per runner.
8. FR-8: The system must validate runner selection before execution begins.
9. FR-9: Unknown or unavailable runner IDs must fail with clear operator-facing errors.
10. FR-10: CLI surfaces must allow relevant commands to specify or inspect runner selection.
11. FR-11: Factory or runtime configuration must allow runner-specific setup without leaking provider-specific logic into unrelated core config paths.
12. FR-12: The API contract must expose runner-related request and response fields where runner selection, inspection, or execution reporting is relevant.
13. FR-13: Generated API artifacts must remain aligned with the runner-related contract changes.
14. FR-14: The UI must show runner availability and selected-runner state where users configure or inspect execution targets.
15. FR-15: The UI must hide, disable, or explain optional features that are unsupported by the selected runner.
16. FR-16: The system must represent runner capability metadata in a way backend, CLI, API, and UI surfaces can consume consistently.
17. FR-17: Baseline orchestration flows must execute successfully across all first-release supported runners.
18. FR-18: The system must not require full optional feature parity across all runners in v1.
19. FR-19: Unsupported optional features must fail early and clearly rather than producing silent degradation.
20. FR-20: Test coverage must include runner resolution, configuration validation, baseline execution, capability gating, and regression coverage for the existing Codex path.

## Non-Goals

- No billing integration, vendor procurement, or provider account setup flows.
- No fine-tuning, custom model training, or model lifecycle management beyond selecting and executing through supported runners.
- No requirement that every supported runner expose identical optional features in v1.
- No automatic provider recommendation or dynamic optimization engine that chooses the "best" runner without explicit product rules.
- No marketplace or third-party plugin ecosystem for arbitrary external runners in the first release unless needed to support the named built-in runners.
- No full redesign of unrelated work scheduling, work modeling, or factory orchestration concepts beyond changes required for multi-runner support.

## Design Considerations

- Runner selection should feel operationally clear rather than provider-marketing driven. Operators should quickly understand what will run where.
- The UI should show runner differences in a structured way, not as scattered warning text.
- Terminology should be consistent across CLI flags, API field names, docs, and UI labels. Prefer one canonical term such as `runner`.
- Default behavior for users who do not choose a runner should remain simple and backward-compatible.

## Technical Considerations

### Architecture Guidance

- Follow the backend standards by isolating side effects, keeping runner-specific process or network behavior behind narrow interfaces, and avoiding leakage of provider-specific rules into core scheduling logic.
- Keep the runner abstraction deep enough to hide execution complexity while exposing simple orchestration-facing semantics.
- Treat OpenAPI, generated code, backend runtime wiring, CLI configuration surfaces, and UI selection flows as one coordinated contract change.
- Design capability negotiation so it can support future runners without reopening the core abstraction each time.

### Capability Model Guidance

- The baseline capability set should be intentionally small and represent only what every supported runner can reliably do in standard orchestration flows.
- Optional capabilities should be named, inspectable, and testable rather than inferred from runner IDs in scattered branches.
- Runner-specific settings should be validated close to the runner boundary while still surfacing actionable errors to top-level operators.

### Integration Guidance

- Prefer a registry or factory pattern for runner resolution over hard-coded conditional trees distributed across the codebase.
- Ensure process-backed runners and API-backed runners can both fit the same orchestration contract if the named integrations differ in transport style.
- If certain runners require local binary presence, auth tokens, or environment checks, those prerequisites should fail during setup or preflight instead of mid-run when possible.

### Testing Guidance

- Add regression coverage for the current Codex path before or alongside abstraction work so parity issues are easier to catch.
- Include integration evidence for at least one successful baseline flow per supported runner.
- Add negative-path coverage for unsupported capability requests, missing prerequisites, and invalid runner IDs.
- Include UI verification for runner visibility and disabled-state behavior where the website exposes runner selection.

## Success Metrics

- Customers can configure and execute work with any first-release supported runner without changing core orchestration code.
- Existing Codex users see no functional regression in baseline execution flows after the abstraction is introduced.
- Operators can tell from CLI, API, and UI surfaces which runner is selected and whether required capabilities are available.
- Adding a future runner after this feature requires a new adapter and registration work, not a scheduler rewrite.
- Integration and contract tests prevent drift between backend behavior and runner-related API or UI surfaces.

## Open Questions

- Confirm whether Kiro and Cursor CLI should be treated as directly supported first-party runner IDs in public contracts or grouped under a broader CLI-runner family with aliases.
-> directly supported. 
- Define the exact baseline capability set for v1: for example prompt submission, streaming or non-streaming output, tool execution, file access expectations, structured result reporting, and cancellation semantics.
-> prompt submission, tool execution, we leave the rest out for now. 
- Decide whether runner selection belongs on each work item, each workstation or factory, or both with precedence rules.
-> each workstation/factory with precedence rules. 
- Decide how much execution metadata about the runner should be persisted on work records for auditability and debugging.
-> we should keep them on the workstation dispatch request/response but that's about it. 
- Confirm whether the UI should allow changing runner selection everywhere relevant in v1 or only expose read visibility in some surfaces while configuration remains CLI or config-first.
-> only where relevant. 
