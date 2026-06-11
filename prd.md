# PRD: UI CI Acceleration and Test Rationalization

## Introduction

The current required CI path spends too much wall clock time in UI verification, especially covered Vitest runs and browser integration tests. Because the required lanes run serially today, browser integration failures can arrive only after a long covered UI pass completes. This slows merge feedback, makes flaky failures more expensive, and makes it harder for maintainers to tell whether a failing test is proving a unique product contract or repeating assertions already covered elsewhere.

This project will make browser failures surface earlier, reduce required UI CI wall clock time, use the existing sharded coverage runner, add stable timing visibility, and rationalize redundant UI tests while preserving defect detection. The intended outcome is a faster, more trustworthy PR verification path where each test lane has a clear purpose, clear rerun command, and clear failure output.

## Context

### Customer Ask

Accelerate the UI-heavy portions of CI and rationalize redundant tests so browser integration regressions fail faster, covered UI runs complete sooner, and maintainers have enough timing evidence to keep the suite from regressing.

### Concrete Problem

The current `make ci` flow waits for long covered UI verification before browser integration can report failures. The covered UI runner already has sharding support, but the canonical PR path does not use it. Several slow tests are broad app-shell or React Flow-heavy suites that repeatedly mount expensive dashboard state for narrower behaviors. Browser, jsdom integration, and unit-style tests also overlap in places, increasing runtime without adding proportional confidence.

### High-Level Solution

Introduce a canonical PR verification flow that either runs browser integration before UI coverage or runs both through a repo-owned orchestrator with fast-fail semantics. Use sharded UI coverage in CI with deterministic merged coverage output. Add stable slow-test reporting for covered UI and browser lanes. Define lane boundaries and split the highest-cost redundant suites into lower-cost feature-scoped tests where that preserves the same observable confidence.

## Project-Level Acceptance Criteria

- [ ] Browser integration no longer waits for the full covered UI lane to finish before starting in the canonical PR verification flow.
- [ ] The required UI-related CI wall clock time is reduced by at least 30% from the current measured baseline, or the closeout artifact explains the measured blocker and next tuning step.
- [ ] Sharded UI coverage preserves merged coverage reports, threshold enforcement, replay coverage checking, and clear failure output when a shard fails.
- [ ] Covered UI and browser integration lanes emit stable slow-file summaries that maintainers can compare across runs.
- [ ] Lane boundaries explain what belongs in unit/jsdom coverage, covered feature integration, and browser integration, with duplicate assertion patterns removed from at least one high-cost area.
- [ ] Browser integration remains deterministic: failures identify the lane and rerun command, and concurrent execution avoids port, preview server, and shared download conflicts.
- [ ] Typecheck, lint, and relevant tests pass for CI orchestration, coverage sharding, reporting, and changed UI test behavior.

## Goals

- Reduce required UI-related CI wall clock time by at least 30%.
- Surface browser integration failures before or alongside covered UI failures.
- Cut median time-to-first-actionable browser regression failure to under 10 minutes.
- Use existing UI coverage sharding and merge behavior instead of weakening coverage thresholds.
- Establish stable timing visibility for the slowest covered UI and browser integration files.
- Reduce redundant UI coverage across app-shell, replay, React Flow, and import/export scenarios.
- Preserve or improve defect detection quality while decreasing runtime and flake exposure.

## User Stories

### prd-ui-ci-acceleration-and-test-rationalization-001: Fast-Fail Required UI CI Flow

**Description:** As an engineer merging a UI change, I want browser integration to start before or alongside covered UI verification so that browser regressions become actionable sooner.

**Acceptance Criteria:**

- [ ] The canonical PR verification flow starts browser integration before covered UI completion, either by ordering browser first or by running browser and coverage through one orchestrated concurrent flow.
- [ ] Failure output names the failed lane and includes the target or command an engineer should rerun locally.
- [ ] Concurrent lanes, if used, keep lane logs attributable and avoid shared-state conflicts between browser-oriented processes.
- [ ] The chosen ordering or concurrency behavior is documented in the developer-facing CI target help or adjacent CI documentation.
- [ ] Tests pass.
- [ ] Typecheck passes.

### prd-ui-ci-acceleration-and-test-rationalization-002: Sharded Covered UI Verification

**Description:** As an engineer waiting on CI, I want covered UI tests to run in shards and merge coverage afterward so that wall clock time drops without weakening coverage enforcement.

**Acceptance Criteria:**

- [ ] The canonical PR verification flow uses the repo-owned UI coverage shard and merge behavior with a documented default shard count.
- [ ] The merged coverage report preserves threshold enforcement and replay coverage checking after all expected shards finish.
- [ ] A missing or failed shard produces a clear failure that identifies the shard and does not silently produce partial coverage.
- [ ] Shard count can be tuned through a documented environment variable or CI setting without changing test semantics.
- [ ] Tests pass.
- [ ] Typecheck passes.

### prd-ui-ci-acceleration-and-test-rationalization-003: Stable UI Test Cost Reporting

**Description:** As a maintainer, I want CI to report the slowest covered UI and browser integration files with cost categories so that runtime regressions are visible and actionable.

**Acceptance Criteria:**

- [ ] Covered UI verification emits a stable top-slowest file summary after the run or after shard merge.
- [ ] Browser integration emits per-file timing or an equivalent top-slowest summary after the run.
- [ ] The report categorizes slow files into app-shell integration, React Flow graph tests, replay/timeline tests, import/export tests, script-style tests, or uncategorized.
- [ ] The report format is suitable for closeout notes and can be compared across CI runs without requiring source topology assertions.
- [ ] Tests pass.
- [ ] Typecheck passes.

### prd-ui-ci-acceleration-and-test-rationalization-004: Lane Boundary and Redundancy Policy

**Description:** As a maintainer, I want explicit test-lane boundaries so that unit, covered jsdom, and browser tests each verify distinct behavior.

**Acceptance Criteria:**

- [ ] The test strategy defines what observable contracts belong in unit tests, covered feature/jsdom integration tests, and browser integration tests.
- [ ] Browser integration guidance prioritizes durable behavior such as saved payloads, network effects, downloads, imports, and final visible state over repeated copy-only assertions.
- [ ] Export/import and graph-editing flows have a documented minimum browser contract that avoids repeating every jsdom UI assertion.
- [ ] At least one existing duplicate assertion pattern is removed or moved to the cheaper lane while preserving the durable behavior check.
- [ ] Tests pass.
- [ ] Typecheck passes.

### prd-ui-ci-acceleration-and-test-rationalization-005: Split One High-Cost App-Shell Suite

**Description:** As a maintainer, I want at least one oversized app-shell test suite replaced with smaller feature-focused coverage so that the same behavior is proven with less setup cost.

**Acceptance Criteria:**

- [ ] One high-cost app-shell suite is selected from the current slow-file inventory and its unique observable behaviors are listed before changes.
- [ ] Equivalent behavior is covered by smaller feature-owned tests where whole-dashboard mounting is not required.
- [ ] Any remaining app-shell coverage is limited to cross-feature behavior that cannot be proven reliably at a lower layer.
- [ ] Before/after timing for the selected suite or replacement tests is captured in the closeout artifact.
- [ ] Tests pass.
- [ ] Typecheck passes.

### prd-ui-ci-acceleration-and-test-rationalization-006: Browser Integration Stability and Runtime Boundaries

**Description:** As an engineer debugging browser failures, I want browser integration tests to be isolated and deterministic so that failures are attributable and retries remain rare.

**Acceptance Criteria:**

- [ ] Browser integration documentation states which suites must remain sequential because of shared preview servers, ports, downloads, or global browser state.
- [ ] Any browser test concurrency introduced by this project uses isolated ports, preview state, and download locations per worker or file.
- [ ] Shared browser helpers expose or document a recommended wait pattern for durable network and UI checkpoints.
- [ ] At least one flaky or over-constrained browser assertion is simplified to assert durable behavior rather than transient copy, animation, or timing state.
- [ ] Direct browser verification confirms changed browser-visible test flows still exercise the intended final UI state.
- [ ] Tests pass.
- [ ] Typecheck passes.

## High-Level Technical Design

The canonical PR verification path should remain easy to run by target name. If the implementation chooses concurrency, use a repo-owned orchestration target or script that prefixes or buffers lane output so failures remain attributable. If the implementation chooses ordering, run browser integration as an earlier fast-fail lane and keep covered UI as the authoritative coverage lane.

Covered UI verification should use the existing shard and merge model. Each shard must produce an identifiable artifact, and merge must fail when an expected shard result is missing. The merged report remains the only source for coverage threshold enforcement and replay coverage checking.

Slow-test observability should be generated by the test lanes themselves or by repo-owned wrappers, not by a one-time spreadsheet. Reports should include top slow files, elapsed time, lane name, and a likely cost category. Categories are for maintainer triage and should not become a brittle source-inventory test.

Test rationalization should start with the slowest app-shell and React Flow-heavy suites. Implementers should preserve user-visible or maintainer-visible behavior while moving repeated setup-heavy assertions into feature-scoped tests where possible. Browser tests should keep durable end-to-end contracts and avoid duplicating copy-only assertions already covered in jsdom.

## Functional Requirements

- FR-1: The canonical PR verification flow must start browser integration before the covered UI lane completes.
- FR-2: If test lanes run concurrently, lane logs must remain attributable, buffered or prefixed, and rerunnable by target name.
- FR-3: Browser integration failures must provide a fast-fail signal without waiting for the full covered UI lane to complete.
- FR-4: The covered UI lane must support shard-based execution in CI using the repo-owned shard and merge behavior.
- FR-5: The default coverage shard count and tuning mechanism must be documented.
- FR-6: Coverage merge must fail clearly when expected shard artifacts are missing.
- FR-7: The covered UI lane must preserve merged coverage reports, coverage thresholds, and replay coverage checks.
- FR-8: Covered UI and browser integration lanes must emit stable slow-file summaries.
- FR-9: Slow-file summaries must include top N files, lane name, elapsed time, and a likely optimization category.
- FR-10: The maintained slow-test inventory must include current-selection app shell, layout/graph app shell, replay stream, React Flow edit integration, import/export browser flows, and layout performance tests when present in the measured run.
- FR-11: The test strategy must define lane boundaries for unit tests, covered jsdom integration tests, and browser integration tests.
- FR-12: Browser integration tests must prioritize durable assertions such as saved payloads, network requests, downloaded or imported artifacts, and final visible state.
- FR-13: Repeated whole-dashboard setup patterns must be replaced with lower-cost feature harnesses where the behavior under test is feature-local.
- FR-14: At least one broad app-shell suite must be split or reduced without removing coverage of its unique observable behavior.
- FR-15: Browser integration sequencing and any safe concurrency rules must be documented with port, preview server, and download-state constraints.
- FR-16: The project must produce a closeout artifact or note with before/after timing for CI wall clock time, covered UI time, browser lane time, and top slow files.

## Non-Goals

- Rewriting the entire UI test stack.
- Eliminating browser integration tests.
- Lowering coverage thresholds as the primary speed strategy.
- Parallelizing browser tests in a way that knowingly increases flakiness or port conflicts.
- Replacing targeted end-to-end coverage with only unit tests.
- Fixing every historical React `act(...)` warning or unrelated console warning.
- Performing broad unrelated cleanup while changing CI and test strategy.

## Supporting Technical and UX Considerations

- CI operator experience matters: required lanes should remain easy to run locally and failures should identify the exact rerun command.
- Faster CI is only valuable if failures remain deterministic and attributable.
- Browser integration logs must stay readable when run near other long-running lanes.
- Sharding should respect CI executor capacity; too many shards may add overhead without improving wall clock time.
- Browser tests that use preview servers, downloads, ports, or global browser state need explicit isolation before any file-level concurrency.
- Frontend-visible changes to test fixtures or browser flows should still be verified in a browser, including loading, success, empty, and failure states when those states are affected.
- Test reports should measure runtime behavior and outcomes, not enforce brittle source-file inventories unless the report itself is the product behavior under test.

## Success Metrics

- Required UI-related `make ci` wall clock time drops by at least 30% from the current measured baseline.
- Browser integration failures surface before full UI coverage completion in at least 90% of failing browser-regression cases.
- Main covered UI wall clock time drops by at least 25%.
- The top 10 slowest UI test files are remeasured after changes, and at least 5 improve materially or have documented blockers.
- The number of broad app-shell tests above 10 seconds decreases release over release.
- Browser integration retries due to flake do not increase after CI orchestration and sharding changes.

## Open Questions

- Should the canonical PR path run browser integration first, or run browser integration and UI coverage concurrently under a shared orchestrator?
- Should the layout performance test remain in the required covered lane, or move to a separate performance verification lane with its own runtime budget?
- What CI executor capacity is available for coverage sharding, and what default shard count gives the best wall clock improvement without overhead dominating?
