# PRD: Wire real child execution into the JavaScript durable session runtime

---
author: Codex
last modified: 2026, june, 16
status: draft
---

## Introduction

Wire one bounded real child-execution path into durable JavaScript `FactorySession` execution so `agent.run`, `parallel`, and `pipeline` can record real child dispatch behavior through the shared session-backend inspection model instead of always reporting fake execution. The change should preserve the existing fake child path for deterministic tests and for lanes that do not need provider-backed execution, while proving that one live bridge can expose real execution mode, stable dispatch identity, provider-session references when present, artifact references when produced, and typed failure detail.

This project is the `LIVE-DISPATCH_BRIDGE` deliverable cell. It depends on the completed real backend API session-parity, dispatch-artifact, and lifecycle-control cells plus the existing `workflowruntime.Hooks.NewChildExecutor` seam and durable dispatch or artifact projection in `pkg/factorysessionexecution`.

## Context

### Customer ask

Use the real JavaScript runtime path with one bounded live child-execution adapter so durable JavaScript sessions that call `agent.run`, `parallel`, or `pipeline` no longer report `executionMode: fake` for the bridged child path, and instead expose real execution details through shared session-backend inspection.

### Problem

The JavaScript host API currently has a fake child execution path that is useful for deterministic fanout tests, but it cannot prove provider-backed child execution or durable dispatch detail parity. That leaves a customer-visible gap in shared session inspection: the durable session can show child records, but the bridged path still looks fake even when the backend stack is ready to route one child through a real execution path. Without a bounded live bridge, downstream CLI, MCP, and inspection parity work cannot trust session dispatch reads to represent real execution mode, provider-session correlation, artifact production, or typed child failure behavior.

### High-level solution

Replace the empty `workflowruntime.Hooks{}` child-executor usage inside the JavaScript durable session runtime service with a narrow adapter that maps `workflowruntime.ChildExecutionRequest` onto an existing real executor path and writes the resulting child lifecycle back into the shared durable dispatch and artifact projection model. Keep stable dispatch identity reservation, execution ordering, provider-session correlation, artifact references, and typed failure projection inside the existing `FactorySession`, `Dispatch`, `ProviderSession`, and `FactoryArtifact` vocabulary. Preserve the deterministic fake child path as a selectable coexistence mode, and prove the bridge with focused backend tests for one successful real child and one failed real child.

## Project-Level Acceptance Criteria

- [x] A real-runtime durable JavaScript session using an `agent.run` fixture no longer reports `executionMode: fake` for the bridged child path and instead records a distinct real execution mode through shared dispatch inspection.
- [x] At least one bridged child execution produces durable dispatch detail with stable dispatch ID, status transitions, provider-session refs when present, and artifact refs or output artifacts when produced by the underlying execution path.
- [x] A failed bridged child execution returns typed failure detail through shared session-backend reads without corrupting unrelated child records or final session inspection.
- [x] Existing fake-child runtime tests remain stable, proving the real bridge is an incremental swap-in path rather than a breaking replacement.
- [x] The lane does not implement website inspection, graph editor work, MCP install packaging, or another broad API surface.
- [x] **Quality gate:** typecheck, lint, and focused tests pass, including `go test ./pkg/orchestrators/javascript/... ./pkg/factorysessionexecution/...` and `go test ./pkg/workers/executor/... ./pkg/workers/provider/...`.

## Goals

- Record one real child execution path through durable JavaScript session inspection.
- Preserve canonical `FactorySession`, `Dispatch`, `ProviderSession`, and `FactoryArtifact` vocabulary.
- Keep fake child execution available for deterministic tests and non-live lanes.
- Surface stable real-child dispatch identity, status progression, provider-session references, artifact references, and typed failure details through shared backend reads.
- Keep the bridge narrow enough that later CLI, MCP, and website parity work can consume it without redefining session inspection semantics.

## User Stories

### dynamic-workflows-cell-live-provider-dispatch-bridge-001: Run one real `agent.run` child through the durable dispatch bridge

**Description:** As a maintainer validating live JavaScript session execution, I want one `agent.run` child to route through a real child executor so shared session inspection shows real dispatch behavior instead of a fake execution mode.

**Acceptance Criteria:**

- [ ] A durable JavaScript session fixture that calls `agent.run` can select the real child-executor path without changing workflow source syntax.
- [ ] The resulting child dispatch read shows a non-fake execution mode that is distinct from the deterministic fake-child path.
- [ ] The bridged child preserves a stable reserved dispatch ID from queueing through terminal completion.
- [ ] Shared session inspection continues to expose the child under existing `FactorySession` and `Dispatch` reads rather than a workflow-run-specific surface.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-live-provider-dispatch-bridge-002: Persist real-child dispatch lifecycle, provider-session refs, and artifact refs

**Description:** As a caller inspecting durable session dispatches, I want successful bridged child executions to expose stable lifecycle detail, provider-session correlation, and produced artifacts so real child work can be inspected through the shared backend model.

**Acceptance Criteria:**

- [ ] At least one successful bridged child execution records ordered dispatch status transitions that are visible through shared session-backend reads.
- [ ] When the underlying execution path creates or correlates a provider session, the durable dispatch detail includes the related `ProviderSession` reference without introducing new child-execution nouns.
- [ ] When the underlying execution path produces artifacts or output artifacts, the durable dispatch detail exposes stable artifact refs or artifact summaries through the existing session and artifact inspection surfaces.
- [ ] Successful bridged child inspection remains compatible with `agent.run`, `parallel`, and `pipeline` semantics by reusing one shared child-executor bridge rather than separate one-off projection paths.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-live-provider-dispatch-bridge-003: Return typed failed-child detail without corrupting sibling or final session inspection

**Description:** As a caller inspecting failed child work, I want a bridged child failure to surface typed durable failure detail so I can diagnose the failure without losing unrelated child records or the parent session inspection state.

**Acceptance Criteria:**

- [ ] A fixture that forces a real bridged child failure returns typed failure detail through shared session-backend dispatch reads.
- [ ] The failed child keeps its own dispatch ID, lifecycle history, and terminal failed status without overwriting successful sibling child records.
- [ ] Parent session inspection remains readable after the failed child path, including final session result or failure inspection according to existing session semantics.
- [ ] Failure projection does not corrupt unrelated artifact refs, provider-session refs, or dispatch summaries that belong to other children.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-live-provider-dispatch-bridge-004: Keep fake-child execution available as a stable coexistence path

**Description:** As a maintainer supporting parallel dynamic-workflow lanes, I want fake-child execution to remain available so the real bridge can land as an incremental swap-in path without breaking deterministic tests or non-live scenarios.

**Acceptance Criteria:**

- [x] Existing fake-child fixtures and runtime tests continue to pass without requiring provider-backed execution.
- [x] Selecting the fake child path still reports fake execution mode and preserves existing deterministic dispatch semantics.
- [x] The live bridge and fake path can coexist behind the shared child-executor seam without changing workflow source syntax for `agent.run`, `parallel`, or `pipeline`.
- [x] The lane records deferred follow-ups such as CLI or MCP live smoke and website inspection as later cells instead of widening this implementation batch.
- [x] Typecheck passes
- [x] Tests pass

## High-Level Technical Design

The implementation should keep JavaScript session orchestration inside the existing runtime and session-backend boundaries. The runtime already exposes `workflowruntime.Hooks.NewChildExecutor`; this cell should supply a real child-executor factory from `factorysessionexecution.JavaScriptRuntimeService` instead of leaving the hook empty. That executor should accept the existing `workflowruntime.ChildExecutionRequest`, reserve or reuse the canonical dispatch identity early, and route the request into the smallest existing real executor or provider path that can already produce durable dispatch, provider-session, and artifact outcomes.

The bridge should not invent workflow-specific child payloads. The runtime may keep internal adapter types, but durable reads must remain expressed as `FactorySession`, `Dispatch`, `ProviderSession`, and `FactoryArtifact` data. The session-backend projection layer should remain the shared owner of durable inspection shape, with any required field extensions made there rather than in one-off runtime records or UI-only adapters.

Fake-child execution must remain a first-class alternate dependency path. Tests should prove both modes without changing fixture source syntax, which keeps the live bridge a bounded swap-in behind the existing host API primitives.

## Functional Requirements

1. FR-1: The JavaScript durable session runtime must provide a non-empty child-executor hook that can route at least one `agent.run` child through a real execution path.
2. FR-2: A bridged real child must record a distinct non-fake execution mode in shared durable dispatch inspection.
3. FR-3: The durable dispatch record for a bridged child must preserve one stable dispatch ID across queued, running, and terminal states.
4. FR-4: The bridge must project provider-session references when the underlying execution path creates or exposes them.
5. FR-5: The bridge must project artifact refs or output artifacts when the underlying child execution path produces them.
6. FR-6: Failed bridged children must surface typed durable failure detail through shared session-backend reads.
7. FR-7: A failed bridged child must not corrupt sibling child records, unrelated artifact refs, or final session inspection for the parent session.
8. FR-8: `agent.run`, `parallel`, and `pipeline` must continue to use one shared child-executor seam so live and fake paths can be swapped without changing workflow source syntax.
9. FR-9: Existing fake-child runtime coverage must remain valid and keep reporting fake execution mode when the fake path is selected.
10. FR-10: If durable dispatch or artifact read models need additional fields for real-child parity, the owning shared contract must be updated in place and generated artifacts synchronized only if the public schema actually changes.

## Non-Goals

- No website or dashboard inspection work.
- No graph editor or current-selection UI work.
- No MCP host install or packaging changes.
- No broad API or event-stream parity sweep beyond the shared session-backend inspection shape already owned by this lane.
- No renaming of public resource vocabulary away from `FactorySession`, `Dispatch`, `ProviderSession`, or `FactoryArtifact`.

## Supporting Technical Considerations

- Follow `docs/architecture/data-model.md` and keep public naming on shared factory-session resources.
- Preserve the service or session boundary described in `docs/architecture/architecture.md`: `FactoryService` coordinates, while per-session runtime state belongs to the session runtime.
- Use the existing `workflowruntime.Hooks.NewChildExecutor` seam rather than adding a second child-dispatch integration path.
- Prefer extending shared `factorysessionexecution` projection types over adding bridge-only payloads in API or UI layers.
- Verification should stay focused on behavioral backend evidence: successful real child execution, failed real child execution, dispatch inspection reads, artifact reads, and fake-path non-regression.
- Deferred follow-ups such as CLI or MCP live smoke and website inspection should be recorded as later cells, not absorbed into this batch.

## Success Metrics

- A real-runtime `agent.run` fixture produces durable child dispatch inspection with non-fake execution mode.
- At least one successful real child exposes stable dispatch identity, lifecycle detail, provider-session correlation when present, and artifact references when produced.
- At least one failed real child exposes typed failure detail without damaging sibling dispatches or parent session inspection.
- Existing fake-child runtime tests remain green, proving the bridge is additive.

## Open Questions

None. The customer ask fixes the scope on one bounded live provider-dispatch bridge and explicitly defers broader CLI, MCP, website, and graph-editor follow-up work.
