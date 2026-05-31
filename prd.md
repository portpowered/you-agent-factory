# PRD: Consolidate Petri Net Test Assertions

## Introduction

Petri-net mapping tests in `pkg/config/maptests` and `pkg/replay/configtests` each define nearly identical helpers for asserting that mapped nets contain no `TransitionExhaustion` transitions and that guarded loop-breaker transitions have the expected `VisitCountGuard` wiring. The duplication diverged only in failure-message wording and minor guard assertion formatting.

This change consolidates those helpers into `pkg/testutil` (alongside existing fluent marking assertions on `petri.MarkingSnapshot`) and retargets both test packages to import the shared surface. **Production mapper and replay behavior are unchanged**; only test maintenance and failure-message consistency at call sites are in scope.

## Context

### Customer ask

As part of backend `pkg/` duplication cleanup (customer ask `11`), eliminate duplicate Petri transition assertion helpers so maintainers update guard and exhaustion checks in one place.

### Concrete problem

- `pkg/config/maptests/config_mapper_resources_test.go` defines `assertNoTransitionExhaustion` and `assertGuardedLoopBreakerTransition`.
- `pkg/replay/configtests/effective_config_test.go` defines near-identical `assertReplayHasNoTransitionExhaustion` and `assertReplayGuardedLoopBreakerTransition`.
- Call sites in `pkg/config/maptests/config_mapper_test.go` and `pkg/replay/configtests/effective_config_test.go` depend on those locals.
- `pkg/testutil/assertions.go` already hosts Petri marking assertions; the shared transition helpers belong in the same package (new file such as `petriassert.go` only if file-size lint requires it).

### High-level solution

Add exported helpers (e.g. `AssertNoTransitionExhaustion`, `AssertGuardedLoopBreakerTransition`) under `pkg/testutil` with a small options value for exhaustion-context phrasing so maptests and replay tests preserve their distinct failure messages. Replace local definitions and call sites, then delete unused local helpers. Verify with focused `go test` on the two affected test packages.

## Goals

- Maintain exactly one implementation of each Petri transition assertion helper under `pkg/testutil`.
- Keep all existing behavioral expectations: same transitions, guard IDs, max-visit counts, and place IDs verified at each call site.
- Preserve distinct exhaustion failure context strings (`customer-authored mapping` vs `replay-mapped customer config`).
- Remove duplicate local helper definitions from `maptests` and `replay/configtests`.
- Confirm no regression via focused package tests.

## Project-level acceptance criteria

- [ ] Exactly one implementation of `AssertNoTransitionExhaustion` and `AssertGuardedLoopBreakerTransition` lives under `pkg/testutil`.
- [ ] `pkg/config/maptests` and `pkg/replay/configtests` import the shared helpers; no duplicate local definitions remain.
- [ ] Call sites still assert the same transitions, guard transition/workstation IDs, max visits, and input/output place IDs as before consolidation.
- [ ] Exhaustion failures still name the offending transition and include the correct context phrase for maptests vs replay tests.
- [ ] `go test ./pkg/config/maptests/...` and `go test ./pkg/replay/configtests/...` pass.
- [ ] Typecheck, lint, and tests pass for all touched backend areas.

## User Stories

### US-001: Shared Petri transition assertion helpers in testutil

**Description:** As a backend maintainer, I want shared Petri transition assertion helpers in `pkg/testutil` so exhaustion and loop-breaker checks are defined once and reused across mapping and replay tests.

**Acceptance Criteria:**

- [ ] `AssertNoTransitionExhaustion` accepts a transitions map and options (e.g. `ExhaustionContext` string) and fails when any transition has type `petri.TransitionExhaustion`, using the supplied context in the fatal message.
- [ ] `AssertGuardedLoopBreakerTransition` asserts: non-nil transition, type `TransitionNormal`, exactly one input arc with the expected place ID and `*petri.VisitCountGuard`, guard `TransitionID` and `MaxVisits` match arguments, exactly one output arc with the expected place ID.
- [ ] Replay call sites can pass options so exhaustion messages still read `replay-mapped customer config`; maptests messages still read `customer-authored mapping`.
- [ ] Helpers call `t.Helper()` and live in `pkg/testutil` (extend `assertions.go` or add `petriassert.go` in the same package if needed for size).
- [ ] Typecheck passes

### US-002: maptests use shared Petri transition assertions

**Description:** As a config-mapping test author, I want `maptests` to call shared testutil helpers so local duplicate definitions are removed without changing what the mapper tests verify.

**Acceptance Criteria:**

- [ ] `config_mapper_test.go` call sites use `testutil.AssertNoTransitionExhaustion` and `testutil.AssertGuardedLoopBreakerTransition` with maptests exhaustion context and the same transition/place/guard arguments as today.
- [ ] Local `assertNoTransitionExhaustion` and `assertGuardedLoopBreakerTransition` are deleted from `config_mapper_resources_test.go`.
- [ ] `TestConfigMapping_RejectionLoopWithGuardedLoopBreaker` and `TestConfigMapping_GuardedLogicalMoveLoopBreaker` still verify the same reviewer/process loop-breaker transitions, places (`task:init`, `task:failed`), watched transition IDs, and max visit count `3`.
- [ ] `go test ./pkg/config/maptests/...` passes.
- [ ] Typecheck passes
- [ ] Tests pass

### US-003: replay configtests use shared Petri transition assertions

**Description:** As a replay-mapping test author, I want `replay/configtests` to call the same shared helpers so replay effective-config tests stay aligned with maptests without duplicate assertion code.

**Acceptance Criteria:**

- [ ] `effective_config_test.go` call sites use shared testutil helpers with replay exhaustion context (`replay-mapped customer config`) and the same loop-breaker arguments (`review-story-loop-breaker`, `story:init`, `story:failed`, `review-story`, max visits `3`).
- [ ] Local `assertReplayHasNoTransitionExhaustion` and `assertReplayGuardedLoopBreakerTransition` are deleted.
- [ ] Replay exhaustion helper behavior retains nil-transition safety (only non-nil transitions with type `TransitionExhaustion` fail).
- [ ] `go test ./pkg/replay/configtests/...` passes.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** Export `AssertNoTransitionExhaustion(t *testing.T, transitions map[string]*petri.Transition, opts ...)` (or equivalent options struct) from `pkg/testutil`.
- **FR-2:** Export `AssertGuardedLoopBreakerTransition(t *testing.T, transition *petri.Transition, inputPlace, outputPlace, watchedTransitionID string, maxVisits int)` from `pkg/testutil`.
- **FR-3:** Maptests must pass exhaustion context phrasing equivalent to `customer-authored mapping`.
- **FR-4:** Replay configtests must pass exhaustion context phrasing equivalent to `replay-mapped customer config` and preserve non-nil transition checks before type comparison.
- **FR-5:** No changes to `ConfigMapper`, replay mapping, or production Petri net construction logic.

## Non-Goals

- Changing mapper or replay runtime behavior.
- Broader `pkg/testutil` refactors unrelated to Petri transition assertions.
- Meta-tests about file layout, import graphs, or helper registration inventories.
- UI, API contract, or OpenAPI changes.
- Unifying maptests and replay failure-message wording beyond what is required to parameterize exhaustion context (call-site-specific guard message style may remain as implemented in the shared helper).

## High-level technical design

1. **Package ownership:** New helpers belong in `pkg/testutil`, the existing home for cross-package test assertions including `AssertMarking` on `petri.MarkingSnapshot`.
2. **Options surface:** Use a small struct, e.g. `PetriTransitionAssertOptions{ ExhaustionContext string }`, passed to `AssertNoTransitionExhaustion` so fatal messages remain reviewer-verifiable per package.
3. **Loop-breaker helper:** Implement one guard assertion path that satisfies both call sites’ structural checks (type, arc counts, place IDs, `VisitCountGuard` fields). Prefer clear, stable fatal messages; if maptests and replay wording cannot be shared without behavior change, document the chosen unified messages in helper godoc—**structural assertions must not change**.
4. **Migration order:** Add helpers (US-001), migrate `maptests` (US-002), then `replay/configtests` (US-003) to avoid intermediate broken builds.
5. **Verification:** Run focused package tests only; no full-repo test mandate beyond the project quality gate.

## Supporting technical considerations

- `maptests` and `replay/configtests` are separate packages; shared helpers must be exported (`Assert*` prefix).
- `config_mapper_test.go` and `config_mapper_resources_test.go` share package `maptests`; helpers currently live in the resources test file but are used from `config_mapper_test.go`.
- Size target: roughly 150–220 lines touched (helpers + two test-package migrations).
- Follow `docs/internal/standards/code/general-backend-standards.md` for Go naming and test helper conventions.

## Success metrics

- Zero duplicate local definitions of exhaustion or guarded loop-breaker assertions in `maptests` and `replay/configtests`.
- Focused package tests green on first CI run after migration.
- Future Petri assertion changes require editing a single testutil location.

## Open Questions

None. Exhaustion context parameterization and migration scope are fully specified by the customer ask.
