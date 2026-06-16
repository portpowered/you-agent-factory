# PRD: Prove Live-Provider-Backed JavaScript Session Inspection Through the CLI

## Context

The durable JavaScript `FactorySession` backend, lifecycle control reads, and dispatch/artifact API inspection slices are now complete enough to prove one narrower customer outcome: a maintainer can use the existing `you workflow` CLI to start a real-backend JavaScript session that exercises the live-provider child bridge and then inspect the same session's terminal state through existing CLI commands.

The concrete gap is that current coverage proves fake-child and fixture-backed inspection paths, but it does not yet prove that the shared execution-service CLI path can drive one real-provider child-execution scenario and expose stable runtime facts from that live bridge path. Without that smoke proof, maintainers cannot confidently claim that CLI inspection surfaces show real dispatch/provider-session/artifact evidence rather than only fake-path projections.

This work adds one focused CLI live-dispatch smoke lane. It must stay on the existing `FactorySession`, `Dispatch`, `Provider Session`, and artifact inspection vocabulary, use the direct shared execution-service CLI path instead of widening into HTTP-only smoke, and explicitly defer MCP host setup and website inspection to later follow-up work.

## Project-Level Acceptance Criteria

- [ ] A focused CLI smoke path can start a durable JavaScript `FactorySession` through the existing `you workflow` commands using one live-provider child-execution scenario backed by the real backend.
- [ ] The same session ID and terminal outcome are observable across CLI run, status, and result reads for that smoke path.
- [ ] CLI dispatch inspection proves the execution used the non-fake bridged child path by exposing stable evidence such as execution mode, provider-session correlation, and artifact linkage when the runtime produced those records.
- [ ] If current CLI output hides needed shared dispatch or artifact fields, the lane exposes only the smallest existing CLI rendering or JSON-backed assertion surface required for customer-testable inspection.
- [x] Existing fake-child or fixture-backed CLI inspection coverage continues to pass, proving the live-provider smoke path is additive and does not regress prior run, dispatch, or artifact behavior.
- [ ] The lane does not add MCP host setup work, website inspection, HTTP-only CLI smoke, or a broader multi-surface parity sweep; deferred next follow-up is recorded explicitly as either MCP live smoke or website inspection.
- [ ] Typecheck, lint, and focused CLI/backend tests pass, including the customer-provided verification commands where applicable.

## Goals

- Prove one real-backend CLI loopback path for a live-provider-backed JavaScript `FactorySession`.
- Show stable session lifecycle inspection through existing `you workflow` run, status, and result commands.
- Show dispatch inspection evidence that differentiates the bridged child path from fake-child coverage.
- Preserve transport-neutral `FactorySession` and dispatch/artifact semantics across CLI and shared backend services.
- Keep scope narrow enough to land as one additive smoke lane.

## User Stories

### dynamic-workflows-cell-cli-live-dispatch-smoke-001: Start and Re-read a Live-Provider JavaScript Session Through the CLI

**Description:** As a maintainer validating dynamic workflows from the command line, I want one focused CLI smoke path that starts a durable JavaScript `FactorySession` against the real backend and then re-reads its status and result so that I can prove the existing `you workflow` commands work end to end for a live-provider child-execution scenario.

**Acceptance Criteria:**

- [ ] A focused smoke scenario starts a durable JavaScript `FactorySession` through the existing shared execution-service CLI path instead of a fake-only path.
- [ ] The smoke scenario uses one live-provider child-execution fixture or focused mock provider that exercises the live-provider child bridge without requiring MCP host startup.
- [ ] CLI reads for run, status, and result all resolve to the same session ID.
- [ ] CLI status and result output show a stable terminal outcome for the started session.
- [ ] Focused tests prove the CLI path uses real backend session execution state rather than a fixture-only command stub.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-cli-live-dispatch-smoke-002: Expose Bridged Dispatch and Artifact Proof in CLI Inspection

**Description:** As a maintainer inspecting a live-provider-backed session, I want CLI dispatch and artifact inspection to surface enough shared data to prove the dispatch came from the bridged child path so that provider-session correlation and output linkage are customer-testable.

**Acceptance Criteria:**

- [ ] CLI dispatch inspection for the smoke session shows evidence of the non-fake bridged child path, such as live execution mode, provider-session references, or equivalent shared dispatch markers when produced by the runtime.
- [ ] CLI inspection preserves `FactorySession`, `Dispatch`, `Provider Session`, and artifact vocabulary instead of inventing a live-smoke-specific model.
- [ ] When the runtime produces output artifacts or linkage, CLI inspection exposes stable artifact linkage for the related dispatch without leaking transport-specific internals.
- [ ] If existing human-readable output does not expose enough shared fields, the implementation extends only the smallest existing CLI rendering or JSON-backed assertion surface needed for reviewer-verifiable inspection.
- [ ] Focused tests assert the shared dispatch/artifact proof path through existing CLI commands.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-cli-live-dispatch-smoke-003: Preserve Existing CLI Inspection Coverage While Adding the Live Smoke Lane

**Description:** As a maintainer guarding the current CLI inspection contract, I want fake-child and fixture-backed coverage to stay green while the new live-provider smoke path is added so that the new lane proves additive behavior rather than replacing existing regression protection.

**Acceptance Criteria:**

- [x] Existing fake-child or fixture-backed CLI inspection scenarios for run, dispatch, result, and artifacts still pass after the live-provider smoke path lands.
- [x] The live-provider smoke path is additive and does not remove or weaken prior inspection assertions for fake-backed scenarios.
- [x] Focused regression coverage proves the new lane does not change prior CLI behavior for sessions that do not use the live-provider bridge.
- [x] The customer-provided focused verification commands are included in the implementation evidence or updated to equivalent narrower commands if package ownership changes during implementation.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-cli-live-dispatch-smoke-004: Record Explicit Scope Boundaries and Deferred Follow-Up

**Description:** As a reviewer approving a narrow dynamic-workflows cell, I want the implementation and verification scope to record what this lane proves and what it intentionally defers so that the batch does not silently widen into MCP or dashboard parity work.

**Acceptance Criteria:**

- [ ] Implementation notes, test names, or nearby planning comments make clear that this lane proves CLI live-dispatch smoke through the shared execution-service path and does not add MCP host setup or website inspection.
- [ ] Deferred follow-up is recorded explicitly as either MCP live smoke on `you mcp serve` or website inspection, rather than broadening this batch into both.
- [ ] Any projection or mapper changes remain transport-neutral in shared session/dispatch/artifact boundaries instead of forking CLI-only dispatch semantics.
- [ ] Reviewable evidence shows the lane stayed on direct CLI-observable behavior and avoided unrelated cleanup or broader parity sweeps.
- [ ] Typecheck passes.

## High-Level Technical Design

The implementation should stay on the existing CLI-to-shared-service path. `you workflow` commands should continue to call the shared session execution path backed by `pkg/cli/sessionexecution` and the durable real backend. The smoke scenario should use one live-provider child-execution fixture or focused mock provider only to the extent needed to drive the live-provider child bridge and produce inspectable session, dispatch, and artifact state.

Any needed output expansion belongs in the smallest existing CLI inspection surface, ideally through the existing JSON path or minimal human-readable rendering, so that the CLI exposes already-available shared dispatch or artifact fields rather than introducing a CLI-only transport model. If shared projections need adjustment, those changes should remain transport-neutral in the session execution or API-surface mapping layers so the CLI is consuming canonical session/dispatch/artifact semantics rather than a forked shape.

Verification should sit where the behavior becomes observable: focused CLI smoke tests for run/status/result/dispatch/artifact reads, plus regression coverage for existing fake-child scenarios. The lane should use the direct shared execution-service CLI path first and should not expand into HTTP CLI smoke, MCP host startup, or dashboard/browser proof.

## Functional Requirements

- FR-1: The existing `you workflow` CLI must be able to start one durable JavaScript `FactorySession` that exercises the live-provider child bridge through the real backend.
- FR-2: The CLI must allow status and result inspection of that same session through existing commands without changing the session ID between reads.
- FR-3: The CLI must expose a stable terminal outcome for the smoke session.
- FR-4: The CLI must expose enough shared dispatch inspection data to prove the execution path used the bridged live-provider child path rather than a fake-child path.
- FR-5: When the runtime produces provider-session references or dispatch-to-artifact linkage, the CLI must expose those shared fields through the minimal existing inspection surface needed for reviewer-verifiable proof.
- FR-6: Existing fake-child and fixture-backed CLI inspection behavior must remain intact.
- FR-7: Shared session, dispatch, and artifact semantics must remain transport-neutral; the lane must not introduce a separate live-smoke transport model or CLI-only dispatch semantics.
- FR-8: The implementation must explicitly defer MCP host setup and website inspection follow-up rather than widening this lane.

## Non-Goals

- No MCP host install work or live MCP smoke in this batch.
- No website or dashboard inspection parity.
- No HTTP-only CLI smoke path or broad API/event parity sweep.
- No new `DynamicWorkflowRun` or live-smoke-specific transport model.
- No unrelated cleanup, refactors, or multi-surface expansion outside the focused CLI live-dispatch smoke behavior.

## Supporting Technical Considerations

- Use canonical public vocabulary from the data model: `FactorySession`, `Dispatch`, `Provider Session`, and artifacts.
- Prefer the already-landed direct shared execution-service CLI path before any wider transport proof.
- Stable proof should come from observable session IDs, terminal state, dispatch fields, provider-session correlation, and artifact linkage rather than from implementation-specific helper assertions.
- If the underlying runtime path can legitimately omit artifacts for a given scenario, the smoke proof should still verify dispatch/provider-session evidence and only assert artifact linkage where the runtime produced it.
- The lane touches backend and CLI behavior, so evidence should include direct focused tests rather than meta-tests or inventory assertions.

## Success Metrics

- Maintainers can run one focused CLI smoke path that proves a live-provider-backed JavaScript session through the real backend.
- Session ID and terminal outcome stay stable across CLI run, status, and result inspection.
- Dispatch inspection visibly distinguishes the bridged child path from existing fake-child coverage.
- Existing fake-backed CLI inspection regression coverage stays green.

## Open Questions

No unresolved product questions. If implementation discovers that the current live-provider scenario cannot yet emit stable provider-session references or artifact linkage through shared projections, the lane should expose the smallest customer-testable proof available, document the missing shared field as a follow-up, and keep MCP or website work deferred.
