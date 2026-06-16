# PRD: Expose Durable JavaScript Factory Session Inspection In The Website

## Introduction

Customers can now run durable JavaScript `FactorySession` executions through the
shared backend, but the website still falls back to incomplete or generic
inspection when that session is viewed in the existing Factory Session detail
surface. This leaves customers dependent on CLI or MCP output to understand
live status, phase progress, child dispatch activity, warnings, results, and
artifacts.

This lane adds bounded website parity by extending the existing Factory Session
detail experience to render durable JavaScript session inspection through shared
typed session data, shared dashboard/detail adapters, and shared UI state
treatments. The scope stays narrow: inspect an existing session well, prove the
visible states, and defer broader dashboard, replay, and graph-editor work.

## Context

### Customer Ask

Expose durable JavaScript `FactorySession` inspection in the website so
customers can see shared session status, orchestrator kind, script phase or
status, child dispatch counts, warnings, and available artifact or result
references without needing CLI or MCP output.

### Problem

The durable backend projection and supporting API/session work are available,
but the current website detail surface does not yet provide customer-usable
inspection parity for durable JavaScript sessions. The result is either missing
runtime metadata or fallback states that are too generic, while loading,
not-found, and error handling can leave visible gaps in this inspection lane.

### High-Level Solution

Reuse the existing `FactorySession` detail widget, generated OpenAPI types, and
shared dashboard/session adapters as the canonical UI path. Add only the
smallest missing projection and rendering behavior needed for durable
JavaScript sessions so the detail surface can show status, orchestrator kind,
phase, warnings, child dispatch summaries, and artifact or result references,
while using existing shared UI treatments for loading, missing, and error
states.

## Goals

- Make durable JavaScript `FactorySession` inspection usable in the existing
  website detail surface.
- Keep the customer-facing model aligned to `FactorySession`, `Dispatch`,
  `ProviderSession`, and `FactoryArtifact`.
- Reuse shared typed data, hooks, adapters, and message treatments instead of
  introducing a website-only workflow inspection model.
- Prove both runtime-detail rendering and state handling with focused UI tests
  and one browser verification path.
- Keep the lane bounded away from graph editor, transport redesign, replay
  expansion, or broad dashboard restyling.

## Project-Level Acceptance Criteria

- [ ] A durable JavaScript `FactorySession` shown through the existing website
      detail surface renders shared session status, orchestrator kind, script
      phase or runtime status, warning summaries, child dispatch counts, and
      available artifact or result references without requiring CLI or MCP
      output.
- [ ] The inspection surface uses shared typed `FactorySession` data and shared
      dashboard/detail adapters rather than a website-only dynamic workflow run
      model or feature-local synthetic runtime semantics.
- [ ] Dispatch-backed or artifact-backed runtime details are visible in at least
      one durable JavaScript session success case so customers can understand
      child activity and available outputs from the website alone.
- [ ] Loading, not-found, and error states render through existing shared UI
      treatments with no blank or workflow-specific fallback gaps.
- [ ] Focused UI tests prove durable JavaScript inspection behavior from shared
      typed data, including one runtime-detail case and one non-success state
      case.
- [ ] A browser verification path confirms the bounded inspection UI is usable
      in the local app or a deterministic story for the targeted durable
      JavaScript session state.
- [ ] Typecheck, lint, and focused UI tests pass for the changed website
      surfaces.

## User Stories

### dynamic-workflows-cell-website-session-inspection-001: Render Durable JavaScript Session Summary In The Existing Detail Surface

**Description:** As a customer inspecting a durable JavaScript factory session
in the website, I want the existing session detail surface to show the core
runtime summary so that I can understand what the session is doing without
leaving the dashboard.

**Acceptance Criteria:**

- [ ] A durable JavaScript `FactorySession` detail view renders shared session
      status, orchestrator kind, and current script phase or runtime status in
      the existing detail surface.
- [ ] The success-state summary uses shared `FactorySession` typed data and the
      existing detail hook or adapter path rather than a website-only workflow
      detail model.
- [ ] The rendered summary remains aligned with customer-facing terminology from
      the shared data model.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using the Browser plugin.

### dynamic-workflows-cell-website-session-inspection-002: Show Dispatch, Warning, Result, And Artifact Inspection Details

**Description:** As a customer reviewing a durable JavaScript session, I want
to see child dispatch activity, warnings, and available outputs so that I can
inspect what happened from the website alone.

**Acceptance Criteria:**

- [ ] The detail surface renders child dispatch counts for a durable JavaScript
      session using shared session or dashboard projection data.
- [ ] The detail surface renders available warnings and result or artifact
      references when the durable session includes them.
- [ ] At least one durable JavaScript success scenario shows a dispatch-backed
      or artifact-backed runtime detail case in the website inspection UI.
- [ ] Result, artifact, and dispatch presentation stays within the shared
      `FactorySession`, `Dispatch`, `ProviderSession`, and `FactoryArtifact`
      vocabulary.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using the Browser plugin.

### dynamic-workflows-cell-website-session-inspection-003: Handle Loading, Missing, And Error States With Shared Treatments

**Description:** As a customer opening a durable JavaScript session from the
website, I want clear loading, missing, and failure states so that inspection
does not collapse into blank space or ambiguous fallback copy.

**Acceptance Criteria:**

- [ ] The bounded inspection surface renders an explicit loading state using
      existing shared UI treatments.
- [ ] When the requested session is not found, the detail surface renders an
      existing shared missing-state treatment instead of a blank panel or
      workflow-specific fallback.
- [ ] When session loading fails, the detail surface renders an existing shared
      error treatment with recoverable or diagnosable messaging consistent with
      current dashboard patterns.
- [ ] Non-success states stay inside the existing Factory Session detail surface
      rather than redirecting to a new workflow-specific page.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using the Browser plugin.

### dynamic-workflows-cell-website-session-inspection-004: Prove Bounded Website Parity Without Widening Scope

**Description:** As a maintainer landing durable JavaScript website inspection,
I want focused verification and explicit scope boundaries so that this lane
proves customer-visible parity without turning into a broader dashboard
redesign.

**Acceptance Criteria:**

- [x] Focused UI tests cover at least one durable JavaScript success case with
      runtime metadata plus dispatch or artifact-backed detail rendering.
- [x] Focused UI tests cover at least one loading, not-found, or error state
      for the same bounded inspection surface.
- [x] Browser verification confirms the local app or deterministic story is
      usable for the targeted durable JavaScript inspection state.
- [x] Any richer drilldown, replay-specific website work, or broader dashboard
      refinement is recorded as deferred follow-up rather than implemented in
      this lane.
- [x] Typecheck passes.
- [x] Tests pass.

## High-Level Technical Design

The canonical state for this lane remains the shared typed `FactorySession`
payload and associated dashboard/session projections already produced by the
backend and consumed by the website. The website should extend the existing
Factory Session detail widget and its current hooks or adapters first, using the
smallest adapter or mapper change needed to expose durable JavaScript runtime
fields already present in the shared contract.

UI rendering should remain inside the established session-detail composition
instead of creating a new top-level workflow page or feature-local runtime
shape. Runtime-detail display should project from shared session data into a
customer-usable summary of status, phase, warnings, child dispatch counts, and
artifact or result references. Loading, missing, and error handling should
reuse existing message and shell primitives so the state model remains explicit
and consistent with current dashboard behavior.

If a required parity field is still missing at the projection boundary, the fix
should happen in the shared mapper or session projection path rather than by
inventing UI-only derived semantics.

## Functional Requirements

- FR-1: The website must render durable JavaScript `FactorySession` inspection
  through the existing Factory Session detail surface.
- FR-2: The detail surface must show shared session status, orchestrator kind,
  and script phase or runtime status for durable JavaScript sessions.
- FR-3: The detail surface must show child dispatch counts for durable
  JavaScript sessions when that data exists in the shared typed session or
  dashboard projection.
- FR-4: The detail surface must show available warnings and result or artifact
  references when present.
- FR-5: The website must use shared `FactorySession` typed data and shared
  adapters rather than a website-only dynamic workflow inspection model.
- FR-6: The detail surface must explicitly handle loading, not-found, and error
  states with existing shared UI treatments.
- FR-7: Focused UI tests must prove one durable JavaScript success case and at
  least one non-success state for the bounded inspection surface.
- FR-8: One browser verification path must confirm the layout-sensitive
  inspection state in the local app or a deterministic story.

## Non-Goals

- No factory graph editor changes.
- No current-selection cleanup unrelated to durable JavaScript session
  inspection.
- No replay, resume, or persistence-specific website surfaces.
- No transport redesign, new top-level workflow page, or another broad
  dashboard parity sweep.
- No website-only dynamic workflow run model.

## Supporting Technical And UX Considerations

- Prefer the existing `ui/src/features/factory-session-detail/*` surface and
  shared dashboard/session projections before creating any new feature path.
- Keep state handling explicit for loading, empty or missing, error, and
  success states in line with the website standards.
- Preserve accessible semantics and keyboard usability for any links, buttons,
  disclosure controls, or result-reference affordances shown in the detail
  surface.
- Keep copy concise and aligned with the public `FactorySession` vocabulary from
  the architecture docs.
- If browser verification uses a deterministic story instead of the local app,
  the story should model the same durable JavaScript session state the focused
  tests cover.

## Success Metrics

- A customer can inspect a durable JavaScript session from the website without
  needing CLI or MCP output to understand its state and outputs.
- The session detail surface shows enough runtime summary to explain status,
  phase, dispatch activity, and available result or artifact references.
- Non-success states are explicit and visually consistent with existing website
  behavior.
- The lane lands with focused verification and without widening into unrelated
  dashboard work.

## Open Questions

No unresolved product questions are required for this lane. If implementation
finds that a shared projection field is still missing, it should be repaired in
the shared mapper boundary without broadening scope.
