# Validation report: C07 Factory functional-package cleanup

## Environment and artifact

- Validation source head: `909cf0bd1ebb5565372521fbb32d17b6e951b5b3` (the exact rebased implementation/test tree validated in the clean-room pass below; this report update is documentation-only and does not change executable behavior).
- Environment and configuration: Windows `10.0.26200.0`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, repository `go.mod`, empty `GOFLAGS`, default `GOCACHE`, no modified or untracked files before the temporary forced-unwind reports were created.
- Customer entry point: production `root.BuildProcess` and `Process.Execute`; API-owned and explicit CLI/API-parity cases use their public loopback HTTP boundary; acceptance cases use the root-process harness and real local OS process boundary where that is the tested contract.
- Real and substituted dependencies: production application composition, local filesystem, loopback HTTP, local process/listener and stdio behavior, and test-owned temporary roots; provider/model effects are controlled at `edges.Edges`/command-runner boundaries. No remote provider, credential, customer-data, or paid call was used.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0`.

## Discovery and denominator

The PRD declares 21 packages and 106 top-level tests. Exact discovery on the clean tracked implementation head found 22 packages and 99 current top-level `Test*` functions. All live rows were accounted for; no executable row was silently removed, renamed, or left unclassified. The declared denominator therefore remains an unresolved acceptance blocker, not a passing planning delta.

The difference is a plan-input delta already observed in the progress log: `named_invocation` is a current packaged package, while earlier packaged test entry points were consolidated into shared-process parents (`factory_builder`, `fix`, and `review`). This report proves the live head rather than rewriting the PRD denominator.

### Top-level package census

| Package | PRD count | Live `go test -list '^Test'` count | Named subtest `Action=run` records | Note |
| --- | ---: | ---: | ---: | --- |
| `packaged/catalog` | 13 | 13 | 22 | |
| `packaged/classify` | 3 | 3 | 3 | |
| `packaged/cross` | 3 | 3 | 6 | |
| `packaged/deep_research` | 1 | 1 | 5 | |
| `packaged/factory_builder` | 4 | 1 | 7 | Consolidated shared parent |
| `packaged/fix` | 8 | 5 | 16 | Consolidated shared parent |
| `packaged/full_flow` | 2 | 2 | 3 | |
| `packaged/fusion` | 1 | 1 | 3 | |
| `packaged/goal` | 5 | 5 | 9 | |
| `packaged/javascript_families` | 1 | 1 | 5 | |
| `packaged/loop` | 1 | 1 | 2 | |
| `packaged/named_invocation` | — | 4 | 17 | Current package absent from PRD census |
| `packaged/plan_execute` | 1 | 1 | 1 | |
| `packaged/plan_parallel` | 1 | 1 | 6 | |
| `packaged/quorum` | 1 | 1 | 4 | |
| `packaged/ralph` | 1 | 1 | 7 | |
| `packaged/review` | 7 | 2 | 17 | Consolidated shared parent |
| `packaged/subagent` | 1 | 1 | 9 | |
| `packaged/tts` | 6 | 6 | 8 | |
| `factory/current` | 8 | 8 | 6 | |
| `factory/definitions` | 26 | 26 | 17 | |
| `acceptance` | 12 | 12 | 0 | Acceptance has no named subtests |
| **Total** | **106** | **99** | **173** | **22 live packages; 19 packaged** |

DISC-001 procedure and artifact:

1. `go list ./tests/functional/factory/packaged/... ./tests/functional/factory/current/... ./tests/functional/factory/definitions/... ./tests/functional/acceptance/...` returned the 22 package rows above.
2. Each returned package was run with `go test <package> -list '^Test'`; the output was reconciled to the 99 live top-level rows above.
3. The mandated `go test -json -count=1 -timeout=10m ./tests/functional/factory/packaged/... ./tests/functional/factory/current ./tests/functional/factory/definitions ./tests/functional/acceptance` on the final rebased head exited 0 and produced 272 total `Action=run` records: 99 top-level records and 173 named subtest records. The per-package subtest record counts are retained in the table; acceptance produced no named subtests.

This proves the current executable denominator and named-scenario accounting at clean local-real fidelity on the delivered implementation tree. It does not resolve the stale planning denominator or recreate the absent source plan `docs/temp/functional-test-optimization.md`.

### Exact-head run record

| Procedure | Exit | Observed result |
| --- | ---: | --- |
| `go test -count=1 -timeout=15m ./tests/functional/factory/packaged/...` | 0 | 19 packaged packages passed; catalog 80.985s and TTS 60.302s package timings |
| `go test -count=1 -timeout=15m ./tests/functional/factory/current ./tests/functional/factory/definitions` | 0 | Current 17.489s and Definitions 28.337s package timings |
| `go test -count=1 -timeout=15m ./tests/functional/acceptance` | 0 | Acceptance passed in 18.573s |
| `go test -count=3 -timeout=45m ./tests/functional/factory/packaged/...` | 0 | 19 packaged packages passed; catalog 226.911s and TTS 173.309s package timings |
| `go test -count=3 -timeout=45m ./tests/functional/factory/current ./tests/functional/factory/definitions` | 0 | Current 42.501s and Definitions 56.441s package timings |
| `go test -count=3 -timeout=45m ./tests/functional/acceptance` | 0 | Acceptance passed in 41.656s |
| Required nine sequential `go test -race -count=1 -timeout=30m` owner runs | 0 | `plan_parallel`, `full_flow`, `tts`, catalog, classify, Fix, Review, Current, and Definitions passed with no race diagnostics |
| `make pkg-structure` | 0 | Functional/package structure holds; 532 deletion-only baseline entries remain |
| `make test-lane-audit` | 0 | `contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446` |
| `go run ./cmd/markdown-linter docs/internal/development/functional-test-optimization/c07-cleanup-factory-packages.md` | 0 | Canonical report has no Markdown diagnostics |
| `git diff --check origin/main...HEAD` | 0 | No whitespace errors |

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| DISC-001 current package/test/named-scenario accounting | BLOCKED | Exact clean-head discovery accounts for 22 live packages, 99 top-level tests, and 173 named run records; the live rows are reconciled in the table. | The PRD still declares 21/106, and the planning owner must reconcile that denominator with the live executable inventory. |
| Cleanup matrix H01/H02, U01-U07, B01-B05, Q01-Q02 | PASS for exercised probes | Seven exact-head intentional forced-unwind children reported zero owned residue for the applicable process, listener, session, definition, route, worktree, replay/runtime, selector, root, installed-copy, log/home/work, config, and reserved-port fields; all three current-head repeat groups also passed after the one bounded `packaged/goal` retry. | Host-wide resources outside test ownership and unsupported platform signal behavior remain unproven. |
| MATRIX-X01 authorization applicability | PASS | No authentication boundary is owned by these packages; no authorization behavior was invented or claimed. | Parent authorization gates. |
| MATRIX-X02 shared-support defect disposition | PASS | No forced-unwind or repeat run reproduced a defect rooted in `tests/functional/internal/support/**`; the diff contains no shared-support path. | A future reproduction would belong to a later single-owner task. |
| Public behavior preservation | PASS for scoped diff | All three current-head package groups passed count-one/count-three; all existing Work, Factory Event, dispatch, replay, CLI/API, persistence, and provider-call assertions remained active, and all nine race owners passed. | Remote provider behavior and parent full-suite AC3 remain unproven. |
| GATE-PKG-001 | PASS | `go test -count=1 -timeout=15m ./tests/functional/factory/packaged/...` exited 0 across all 19 packaged packages at the exact source head. | Remote providers and other package families. |
| GATE-CURDEF-002 | PASS | `go test -count=1 -timeout=15m ./tests/functional/factory/current ./tests/functional/factory/definitions` exited 0 at the exact source head. | Unsupported platform watcher/process behavior. |
| GATE-ACC-003 | PASS | `go test -count=1 -timeout=15m ./tests/functional/acceptance` exited 0 at the exact source head. | Remote provider authorization/availability. |
| GATE-REPEAT-004 | PASS | Current-head packaged, Current/Definitions, and acceptance count-three commands exited 0 on their first final-head runs. Local wall time is not used as an acceptance threshold. | Future host contention and parent Project triple full-suite runs. |
| GATE-RACE-005 | PASS | Sequential exact-head `go test -race -count=1 -timeout=30m` runs passed for `plan_parallel`, `full_flow`, `tts`, `catalog`, `classify`, `fix`, `review`, `factory/current`, and `factory/definitions`; no race diagnostics were emitted. | Other schedules, platforms, and unexercised owners. |
| GATE-AUDIT-006 | PASS | `make test-lane-audit` exited 0; observed ownership summary was `contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446`. | Static policy cannot prove runtime behavior. |
| GATE-DIFF-007 | PASS | `git diff --check origin/main...HEAD` exited 0; the tracked diff contains only the owned functional-test paths and this report, with no API, generated, shared-support, baseline, Makefile, workflow, or excluded-surface change. | Full repository verification and future base changes. |
| Security, privacy, cost, and cleanup | PASS | All runs used controlled local edges, zero credentials/customer data, zero paid calls, and test-owned roots; no leaked root or reserved port was reported. | Machine-wide artifacts not owned by this lane. |
| UI/browser criteria | PASS — not applicable | No UI, copy, accessibility, localization, responsive, or browser surface changed. | None for this lane. |
| VAL-C07-009 | BLOCKED | The report records the exact validated implementation/test tree, complete live census, matrix evidence, exact-head gate outputs, forced-unwind reports, host-contended discovery attempts, findings, and unproven edges. | The required exact denominator/source-plan authority remains unavailable; remote providers, Unix-only behavior, parent Project AC3, and project-wide inventory beyond this lane also remain outside local proof. |
| GATE-PR-CI-008 and implementation delivery | REVIEW HANDOFF BLOCKED BY PLANNING AUTHORITY | The existing PR is open; the final report-only delivery head is ready for required CI, and review owns terminal Backend Functional Coverage, package-level directional performance, conflict resolution, and merge after planning reconciliation. | CI terminal result and merge remain review-owned; this lane cannot claim full PRD completion while DISC-001/VAL-C07-009 are blocked. |

## Cleanup characterization

The forced-unwind procedure ran each applicable package-local child after its public assertions and deliberately failed at the named characterization assertion. Exit code 1 was expected; the teardown report, not the intentional failure status, is the cleanup property.

| Characterization | Exact selector | Expected exit | Post-unwind report |
| --- | --- | ---: | --- |
| Packaged catalog | `TestPackagedFactoryCatalogListsEveryEmbeddedFactory` with `YOU_FUNCTIONAL_CATALOG_FORCED_UNWIND=1` | 1 | `listener_closed=true` |
| Packaged classify | `TestPackagedClassifyRoutesSmallMediumAndLarge` with `YOU_FUNCTIONAL_CLASSIFY_FORCED_UNWIND=1` | 1 | `listener_closed=true`, `root_absent=true` |
| Packaged Fix | `TestPackagedFixSharedProcess` with `YOU_FUNCTIONAL_PACKAGED_FIX_FORCED_UNWIND=1` | 1 | `listener_closed=true`, `root_absent=true`, `selectors_zero=true`, `census_clean=true` |
| Packaged Review | `TestPackagedReviewSharedProcess` with `YOU_FUNCTIONAL_PACKAGED_REVIEW_FORCED_UNWIND=1` | 1 | `listener_closed=true`, `root_absent=true`, `selectors_zero=true`, `census_clean=true` |
| Current Factory | `TestSharedCurrentFactoryAPI` with `YOU_FUNCTIONAL_CURRENT_FACTORY_FORCED_UNWIND=1` | 1 | `process_invocation_stopped=true`, `process_closed=true`, `listener_closed=true`, `root_absent=true`, `session_deleted=true` |
| Factory Definitions | `TestFactoryDefinitionsSharedProcessKeepsHomesAndCurrentFactoriesIsolated` with `YOU_FUNCTIONAL_FACTORY_DEFINITIONS_FORCED_UNWIND=1` | 1 | Shared process/client, validation/init listeners, homes/factory roots, public session, and workspace all reported closed/deleted/absent. |
| Acceptance harness | `TestRootProcessHarness_IsolatesHomeAndLogDirectoriesAcrossSessions` with `YOU_FUNCTIONAL_ACCEPTANCE_FORCED_UNWIND=1` | 1 | Process, root/home/log/work/config/installed Factory absent and reserved port available. |

The regular count-one and count-three runs also exercised success, rejection, dependency failure, timeout, partial completion, concurrency, cancellation, recovery, boundary, persistence, replay, and package-teardown assertions where applicable. The controlled invalid-input diagnostic logs observed during the runs are expected public failure-path output, not residue findings.

## Customer journey

1. Create a detached worktree at the delivered tracked head and confirm clean status before test-owned temporary reports.
2. Discover the package list and top-level tests, then execute the mandated JSON scenario discovery. All 22 live packages and 173 named records were accounted for.
3. Execute the packaged, Current/Definitions, and acceptance count-one commands through production root composition. Each exited 0 at the exact source head.
4. Execute each owned package group with `-count=3`. Each exited 0 on the exact rebased source head on its first final-head run. No production or shared-support repair was made.
5. Execute the seven opt-in forced-unwind children. Each emitted its post-unwind JSON report and failed only at the intentional assertion. All applicable report fields were clean.
6. Execute the minimum concurrent and changed-owner race cells, `make test-lane-audit`, and `git diff --check`; all exited 0. The detached worktree and its temporary reports were removed after evidence capture.

## Cross-task integration and usability

- Documentation discoverability: this file is the canonical C07 loopback artifact under `docs/internal/development/functional-test-optimization/`; no customer-facing copy changed.
- Permission and error behavior: public invalid-input, missing Factory, provider/worker failure, cancellation, timeout, validation, session, and exit/error diagnostics remained asserted by the existing functional cases.
- Persistence/reload behavior: Current Factory save/read, Definitions import/export/snapshot/init, Goal progress, TTS replay, named invocation, and acceptance home/config/installed-copy paths passed their existing assertions.
- Accessibility/keyboard/responsive behavior: not applicable; this lane has no UI surface.
- Operational signals: package and named-scenario counts, root/process/listener state, Factory Session deletion, resource census selectors, worktree/plan/runtime/replay absence, installed-copy absence, and reserved-port availability were observed at the owned boundary.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C07-F001 | BLOCKING planning discrepancy | Run DISC-001 exactly on the clean tracked implementation head. | The current plan denominator and live executable denominator agree. | PRD declares 21/106; the head has 22/99, including `named_invocation` and consolidated shared parents. All live rows are accounted for and no path was changed by this loopback. | Discovery commands and census table above; exact declaration remains unreconciled. |
| C07-F002 | BLOCKING missing planning authority | Resolve the PRD `sourcePlan` path and referenced task packet in a clean checkout. | The parent plan and task packet are available for independent comparison. | `docs/temp/functional-test-optimization.md` and the referenced task file are absent from the tracked checkout; the local PRD, standards, current diff, and progress log were used as available authority. | `Test-Path`/`rg --files` checks; absence is recorded in progress. |
| C07-F003 | RESOLVED ENVIRONMENTAL | Run the exact-head JSON discovery command in the clean checkout. | Discovery execution exits 0 while yielding the named records. | The final rebased head yielded 272 total `Action=run` records (99 top-level and 173 named subtests) and exited 0; no process-loss fallback was required. | Final-head discovery output and package results above; no C07-caused failure was reproduced. |
| C07-F004 | REVIEW-STAGE | Push the final report head and inspect the required PR check. | Current-head Backend Functional Coverage has started and review can record the package timing/result. | PR #2397 is open; this report-only delivery head is the final local handoff. Required CI start and terminal result remain review-stage evidence, while terminal CI and merge remain review-owned. | GATE-PR-CI-008 handoff row above. |
| C07-F005 | BLOCKING planning discrepancy | Resolve the PRD denominator and source-plan/task-packet authority with the planning owner. | The PRD's exact 21/106 denominator and named source-plan references are independently available and agree with the executable head. | The source head has 22 packages, 99 top-level tests, and 173 named records; the referenced plan and task packet are absent. | Discovery census and `Test-Path`/history checks above. |

## Plan deltas requested

- Affected behavior and criterion: DISC-001 denominator and the source-plan/task-packet references.
- Root-cause evidence or remaining uncertainty: the live tracked head contains one additional packaged package and consolidated top-level test parents, while the referenced `docs/temp` plan and task packet are not present. The current executable rows and named records are fully accounted for, but the intended historical denominator cannot be inferred safely.
- Smallest recommended correction/prerequisite: planning owner must reconcile the PRD/source-plan denominator to the current inventory and restore or attach the canonical plan/task packet; do not change functional tests or shared support as part of this C07 loopback.
- Dependencies and retest scope: planning-owner decision only; if the denominator is changed, rerun DISC-001 and the report reconciliation. No C07 implementation repair is requested.

## Verdict

BLOCKED pending planning-owner reconciliation of the exact denominator and source-plan/task-packet authority. The owned cleanup evidence and implementation checks pass at the exact rebased source tree, with no shared-support change or silent scope change. Implementation delivery is a review-stage handoff; review owns required CI terminal state, conflict resolution, and merge.
