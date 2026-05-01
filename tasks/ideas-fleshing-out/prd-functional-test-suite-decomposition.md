# PRD: Functional Test Suite Decomposition

## Context

**Customer Ask:** "As part of the functional tests, can you help decompose the tests into reasonable packages? Right now the tests are taking too long, and the number of files in that directory are too ornery for me to reason about. We ideally want total test time to be 10 seconds or less. We want the functional tests structured across reasonable layers. Stress tests are separate from functional coverage, and functional coverage is broken down into reasonable segmentations."

**Problem Statement:** The current functional suite is concentrated in a single `tests/functional_test` package with a very large number of unrelated files and shared helpers mixed together. This makes it hard to understand ownership, hard to run focused subsets, and expensive to keep the whole suite fast. A real baseline run of the current functional package takes roughly 74 seconds, which is far above the desired under-10-second target. Slow replay, provider/runtime, fixture sweep, and runtime projection tests are mixed into the same default lane as ordinary functional coverage.

**Solution:** Reorganize the functional suite using a hybrid model: keep one default functional command that runs all non-long functional tests, split those tests into behavior-oriented packages with transport prefixes for discoverability, centralize shared harness code in `tests/functional/internal/support`, and move slow or broad-sweep coverage into an explicitly opt-in long-test lane gated behind a dedicated flag such as `-long`.

## Project Acceptance Criteria

- [ ] The repository has a new functional test package structure under `tests/functional/` with clear, documented package purposes instead of a single `tests/functional_test` bucket.
- [ ] All non-long functional tests are runnable through one documented default command and complete in 10 seconds or less on the agreed developer baseline environment.
- [ ] Long-running or broad-sweep functional coverage is excluded from the default functional command and is runnable only through an explicit long-test flag or equivalent opt-in mechanism.
- [ ] Shared functional test harnesses, custom executors, and reusable assertions are centralized under `tests/functional/internal/support` rather than duplicated across packages or hidden inside unrelated `*_test.go` files.
- [ ] File naming guidance is documented and applied so transport-oriented tests remain discoverable via prefixes such as `cli_`, `api_`, `replay_`, and `watcher_`.
- [ ] Quality checks pass (typecheck, lint, tests).

## Goals

- Reduce the default non-long functional runtime to 10 seconds or less.
- Make the functional suite easier to navigate by splitting it into a small number of durable packages.
- Preserve discoverability for CLI, API, replay, and watcher paths through consistent filename prefixes.
- Separate long or high-cost coverage from the default lane without losing that coverage.
- Create a structure that supports future test additions without returning to one large undifferentiated folder.

## User Stories

### US-001: Create the new functional package layout
**Description:** As a developer, I want the functional tests grouped into clear packages so I can find the right tests without scanning one oversized directory.

**Acceptance Criteria:**
- [ ] Create `tests/functional/` as the new root for non-long functional tests.
- [ ] Create the agreed package directories for the hybrid model.
- [ ] Each package includes a short purpose statement in package-level docs or a local README.
- [ ] Existing functional tests are assigned to one package according to documented rules.
- [ ] Default `go test` package discovery succeeds for the new layout.

### US-002: Establish the hybrid package taxonomy
**Description:** As a developer, I want tests organized mainly by behavior/domain so related scenarios stay together even when exercised through different surfaces.

**Acceptance Criteria:**
- [ ] The default package taxonomy uses behavior/domain boundaries as the primary split.
- [ ] Cross-cutting infrastructure tests are assigned to the closest behavior package unless they are clearly replay/runtime/provider specific.
- [ ] The package layout includes at least: `workflow`, `guards_batch`, `runtime_api`, `providers`, `replay_contracts`, `bootstrap_portability`, and `smoke`.
- [ ] Stress and long-duration suites remain outside the default functional lane.
- [ ] Package assignment rules are documented for future contributors.

### US-003: Add transport-based file prefixes
**Description:** As a developer, I want filename prefixes that show whether a test exercises CLI, API, replay, or watcher behavior so I can quickly search the suite by surface area.

**Acceptance Criteria:**
- [ ] Tests that primarily validate CLI flows use a `cli_` filename prefix.
- [ ] Tests that primarily validate live API behavior use an `api_` filename prefix.
- [ ] Tests that primarily validate replay behavior use a `replay_` filename prefix.
- [ ] Tests that primarily validate watcher/file-drop behavior use a `watcher_` filename prefix.
- [ ] Prefix rules are documented, including when a behavior-first package should still contain mixed surface-area tests.

### US-004: Centralize shared functional test support
**Description:** As a developer, I want common harness code in one internal support area so package splits do not cause fragile duplication or hidden coupling.

**Acceptance Criteria:**
- [ ] Create `tests/functional/internal/support`.
- [ ] Move reusable harness constructors, fixture helpers, custom executors, and common assertions into the support package.
- [ ] Tests in multiple functional packages compile against the shared support package.
- [ ] Package-local helpers remain local only when they are truly specific to one package.
- [ ] No package depends on unrelated helpers buried in another package's `*_test.go` file.

### US-005: Separate the long-test lane from default functional coverage
**Description:** As a developer, I want slower functional coverage to be opt-in so the default suite stays fast without dropping valuable regression checks.

**Acceptance Criteria:**
- [ ] Define a long-test mechanism, such as a custom `-long` flag or documented equivalent, for opt-in execution.
- [ ] Move long-running fixture sweeps, broad replay sweeps, or duration-heavy tests out of the default lane.
- [ ] The default functional command excludes long tests without requiring developers to name packages manually.
- [ ] Long tests still run successfully through one documented opt-in command.
- [ ] At least one existing slow test category is migrated into the long lane as part of the refactor.

### US-006: Break broad fixture sweeps into targeted coverage
**Description:** As a developer, I want oversized sweep tests decomposed into focused package-local coverage so I can understand failures and keep default runtime low.

**Acceptance Criteria:**
- [ ] Broad sweep tests such as fixture-directory loaders are split into targeted tests owned by the packages they validate.
- [ ] Any remaining broad-sweep validation moves to the long suite.
- [ ] Each new targeted test has a clear behavioral purpose tied to its package.
- [ ] Default functional runtime decreases measurably after the sweep split.
- [ ] Failures identify a narrow feature area rather than a generic mega-sweep.

### US-007: Provide one default command for all non-long functional tests
**Description:** As a developer, I want one obvious command for the default functional suite so I do not have to memorize package lists.

**Acceptance Criteria:**
- [ ] Add one documented default command that runs all non-long functional tests.
- [ ] The default command does not include stress tests or long tests.
- [ ] The default command is suitable for local development and CI gating.
- [ ] The long-test command is documented separately from the default command.
- [ ] The default command runtime is measured and reported against the under-10-second target.

### US-008: Add migration guardrails for future test placement
**Description:** As a maintainer, I want clear guardrails so new tests land in the right package and long tests do not drift back into the default lane.

**Acceptance Criteria:**
- [ ] Add contributor guidance for selecting a package and filename prefix for new functional tests.
- [ ] Add guardrails or review checks that detect long tests accidentally added to the default lane.
- [ ] Add guardrails or review checks that prevent new shared helpers from accumulating in random test files.
- [ ] The migration plan includes a compatibility strategy while old tests are being moved.
- [ ] The final structure is explained in repository docs or test-specific docs.

## High-Level Technical Design

The new suite should use a hybrid organization model with behavior/domain as the primary boundary and transport prefixes as a secondary discoverability aid. The top-level default functional root becomes `tests/functional/`, while opt-in slow coverage moves to a dedicated long lane such as `tests/long/functional` or an equivalent tagged path that is excluded from the default command.

Proposed default non-long package layout:

```text
tests/
  functional/
    smoke/
    workflow/
    guards_batch/
    runtime_api/
    providers/
    replay_contracts/
    bootstrap_portability/
    internal/
      support/
  stress/
  long/
    functional/
```

Package intent:

- `smoke`: a very small confidence lane for broad health checks and sanity coverage.
- `workflow`: end-to-end workflow behavior such as dispatcher, ideation, review loops, integration, and batch orchestration not specific to provider/replay internals.
- `guards_batch`: guard matching, dependency gating, multichannel joins, repeaters, and batch submission semantics.
- `runtime_api`: runtime state, dashboard/API projections, generated API/schema alignment, scheduling, and stateful service behavior.
- `providers`: provider CLI contracts, script executor behavior, env/arg merging, timeout/retry behavior, and runtime config execution boundaries.
- `replay_contracts`: record/replay, event-stream artifacts, timeline projections, and serialized contract invariants.
- `bootstrap_portability`: init/bootstrap, portability, current-factory activation, and customer-facing setup or migration paths.
- `internal/support`: shared harnesses, reusable executors, fixtures, and assertions used across multiple packages.

File naming remains behavior-owned by package, but uses prefixes for discoverability. For example:

- `cli_init_factory_test.go` in `bootstrap_portability`
- `api_named_factory_test.go` in `runtime_api`
- `replay_event_stream_artifact_test.go` in `replay_contracts`
- `watcher_current_factory_switch_test.go` in `bootstrap_portability`

The key rule is that prefix does not determine the package by itself. Package answers "what behavior is being validated?" Prefix answers "through which surface is that behavior being exercised?"

Long-test strategy:

- Default functional command runs every package under `tests/functional/...`.
- Long or duration-heavy functional tests live outside the default package tree or behind explicit opt-in gating.
- Stress tests remain separate and are never part of default functional runs.
- Broad sweep tests are decomposed into targeted package-local tests, with any remaining wide-coverage sweeps moved into the long lane.

Shared support strategy:

- Extract current shared helpers from generic test files into `tests/functional/internal/support`.
- Keep support API intentionally small: harness constructors, reusable custom executors, fixture setup helpers, common assertions, and test-time command runners.
- Avoid cyclical or hidden package coupling by prohibiting one behavior package from importing another behavior package's test helpers.

Control flow for a migrated test:

1. Identify the primary behavior under test.
2. Assign the test to the matching behavior package.
3. Apply a transport prefix if the surface area matters for discoverability.
4. Move any reusable helper logic into `internal/support`.
5. If the test cannot meet default runtime expectations or is a broad sweep, move it into the long lane.

Verification strategy:

- Measure current baseline and post-migration default runtime.
- Verify the default command includes all non-long functional packages.
- Verify the long command includes only explicitly opted-in long tests.
- Spot-check representative tests from each package.
- Add documentation and review guardrails so future tests follow the same structure.

## Functional Requirements

1. FR-1: The repository must replace the monolithic `tests/functional_test` package with a structured `tests/functional/` package tree for default non-long functional coverage.
2. FR-2: The primary organizing principle for default functional packages must be behavior/domain, not transport or implementation seam alone.
3. FR-3: The suite must use transport prefixes such as `cli_`, `api_`, `replay_`, and `watcher_` in filenames when they improve discoverability.
4. FR-4: The default functional command must run all non-long functional tests without requiring developers to enumerate packages manually.
5. FR-5: The default functional command must exclude stress and long tests.
6. FR-6: Long-running functional tests must be executable only through an explicit opt-in mechanism such as `-long` or an equivalent documented command.
7. FR-7: Shared harnesses, custom executors, and cross-package helpers must live under `tests/functional/internal/support`.
8. FR-8: Broad sweep tests must be decomposed into targeted tests where possible, and any remaining broad sweeps must move to the long lane.
9. FR-9: The repository must document the purpose of each functional package, the prefix convention, and the long-test policy.
10. FR-10: The default non-long functional runtime must be reduced from the current roughly 74 seconds to 10 seconds or less on the agreed baseline environment.

## Non-Goals

- Rewriting the production runtime architecture solely to make tests faster.
- Eliminating stress testing or long-duration regression coverage.
- Converting every functional test into a pure unit test.
- Guaranteeing that every individual package is equally sized or equally fast.
- Solving every flaky test problem unrelated to package decomposition and lane separation.

## Additional Technical Considerations

- Current slow hotspots include provider/script-wrap error normalization, event-stream replay artifacts, current-factory activation, runtime config alignment, timeout/process cleanup, and broad fixture sweep tests. The migration plan should target these first when deciding what belongs in the long lane.
- A current real-world baseline showed the default monolithic functional package at roughly 74 seconds, so a 10-second target likely requires both file/package decomposition and active removal of slow sweeps from the default lane.
- The current suite contains helper-heavy files such as `service_harness_test.go`, `testhelpers_test.go`, `provider_harness_helpers_test.go`, `replay_regression_harness_test.go`, and similar support-oriented files. These should be audited early because package decomposition will otherwise break hidden helper dependencies.
- The design should prefer durable package names over naming every package after the current implementation structure. The suite should still make sense if internal production modules shift.
- The default and long commands should be easy to wire into `Makefile`, CI, or task runners so developers do not need to remember custom package globs.

## Success Metrics

- Default non-long functional suite completes in 10 seconds or less.
- The number of files in any single default functional package is low enough for contributors to scan without losing context.
- Developers can identify the correct destination package and filename prefix for a new functional test without asking for ad hoc guidance.
- Long-duration coverage remains available and passes via its explicit opt-in command.
- Failure output from the default lane points to narrow behavior packages rather than one giant mixed test directory.

## Open Questions

- What exact command surface should expose long tests: a custom `go test` flag, build tags, a wrapper script, or a Makefile target that passes package filters?
- What machine and baseline conditions should be used to enforce the 10-second threshold in CI so the target remains fair and stable?
- Should the `smoke` package remain a distinct lane long-term, or should it primarily serve as a migration aid while the full non-long suite is being optimized?
- Which existing tests are allowed to stay in the default lane if they are slightly slower but provide uniquely valuable regression coverage?
