# Validation report: C07 Factory functional-package cleanup

## Environment and artifact

- Commit/build identifier: `5bec35267777f79c79a3c12adac6653cd6393fa8` (detached clean-room worktree at the implementation head before this documentation-only report; no runtime or test behavior changed while writing the report).
- Environment and configuration: Windows `10.0.26200.0`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, repository `go.mod`, empty `GOFLAGS`, default `GOCACHE`, no modified or untracked files before the temporary forced-unwind reports were created.
- Customer entry point: production `root.BuildProcess` and `Process.Execute`; API-owned and explicit CLI/API-parity cases use their public loopback HTTP boundary; acceptance cases use the root-process harness and real local OS process boundary where that is the tested contract.
- Real and substituted dependencies: production application composition, local filesystem, loopback HTTP, local process/listener and stdio behavior, and test-owned temporary roots; provider/model effects are controlled at `edges.Edges`/command-runner boundaries. No remote provider, credential, customer-data, or paid call was used.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0`.

## Discovery and denominator

The PRD declares 21 packages and 106 top-level tests. The exact discovery command on the clean tracked head found 22 packages and 99 current top-level `Test*` functions. All declared rows and the additional live package were accounted for; no executable row was silently removed, renamed, or left unclassified.

The difference is a plan-input delta already observed in the progress log: `named_invocation` is a current packaged package, while earlier packaged test entry points were consolidated into shared-process parents (`factory_builder`, `fix`, and `review`). This report proves the live head rather than rewriting the PRD denominator.

### Top-level package census

| Package | PRD count | Live `go test -list '^Test'` count | Named `Action=run` records | Note |
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
3. `go test -json -count=1 -timeout=10m ./tests/functional/factory/packaged/... ./tests/functional/factory/current/... ./tests/functional/factory/definitions/... ./tests/functional/acceptance/...` exited 0 and produced 173 unique `Action=run` records whose `Test` contained `/`. The per-package record counts are retained in the table; acceptance produced no named records.

This proves the current executable denominator and named-scenario accounting at the clean local-real fidelity. It does not resolve the stale planning denominator or recreate the absent source plan `docs/temp/functional-test-optimization.md`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| DISC-001 current package/test/named-scenario accounting | PASS with planning delta | Clean discovery exited 0; 22 live packages, 99 top-level tests, and 173 named run records are accounted for. The PRD-declared 21/106 denominator and the extra `named_invocation` package are explicitly reconciled above. | Planning owner still needs to reconcile the PRD/source-plan denominator. |
| Cleanup matrix H01/H02, U01-U07, B01-B05, Q01-Q02 | PASS | Count-one/count-three functional groups, public cleanup censuses, and seven intentional forced-unwind children reported zero owned residue for the applicable process, listener, session, definition, route, worktree, replay/runtime, selector, root, installed-copy, log/home/work, config, and reserved-port fields. | Host-wide resources outside test ownership and unsupported platform signal behavior. |
| MATRIX-X01 authorization applicability | PASS | No authentication boundary is owned by these packages; no authorization behavior was invented or claimed. | Parent authorization gates. |
| MATRIX-X02 shared-support defect disposition | PASS | No forced-unwind or repeat run reproduced a defect rooted in `tests/functional/internal/support/**`; the diff contains no shared-support path. | A future reproduction would belong to a later single-owner task. |
| Public behavior preservation | PASS | All three declared package groups passed count one and count three on the clean tracked head; existing Work, Factory Event, dispatch, replay, CLI/API, persistence, and provider-call assertions remained active. | Remote provider behavior and parent full-suite AC3. |
| GATE-PKG-001 | PASS | `go test -count=1 -timeout=15m ./tests/functional/factory/packaged/...` exited 0 in 75.737s; 19 packaged packages passed. | Remote providers and other package families. |
| GATE-CURDEF-002 | PASS | `go test -count=1 -timeout=15m ./tests/functional/factory/current ./tests/functional/factory/definitions` exited 0 in 19.443s. | Unsupported platform watcher/process behavior. |
| GATE-ACC-003 | PASS | `go test -count=1 -timeout=15m ./tests/functional/acceptance` exited 0 in 13.823s. | Remote provider authorization/availability. |
| GATE-REPEAT-004 | PASS after one bounded transient rerun | Current/Definitions count three exited 0 in 54.826s; acceptance count three exited 0 in 40.543s; the exact packaged count-three rerun exited 0 in 201.858s. The first packaged aggregate attempt exited 1 only in untouched `packaged/goal`; its one package-local retry exited 0 in 44.311s, and the exact aggregate rerun then passed. | Future host contention and parent Project triple full-suite runs. |
| GATE-RACE-005 | PASS | Sequential `go test -race -count=1 -timeout=20m` runs passed for `plan_parallel`, `full_flow`, `tts`, `catalog`, `classify`, `fix`, `review`, `factory/current`, and `factory/definitions`; no race diagnostics were emitted. | Other schedules, platforms, and unexercised owners. |
| GATE-AUDIT-006 | PASS | `make test-lane-audit` exited 0; observed ownership summary was `contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446`. | Static policy cannot prove runtime behavior. |
| GATE-DIFF-007 | PASS | `git diff --check origin/main...HEAD` exited 0; the tracked diff contains only the owned functional-test paths and this report, with no API, generated, shared-support, baseline, Makefile, workflow, or excluded-surface change. | Full repository verification and future base changes. |
| Security, privacy, cost, and cleanup | PASS | All runs used controlled local edges, zero credentials/customer data, zero paid calls, and test-owned roots; no leaked root or reserved port was reported. | Machine-wide artifacts not owned by this lane. |
| UI/browser criteria | PASS — not applicable | No UI, copy, accessibility, localization, responsive, or browser surface changed. | None for this lane. |
| VAL-C07-009 | PASS for local clean-room validation | This report is the read-only loopback artifact for the exact clean tracked head and records the complete live census, matrix evidence, gate outputs, findings, and unproven edges. | Review-stage PR CI, remote providers, Unix-only behavior, parent Project AC3, and project-wide inventory beyond this lane. |
| GATE-PR-CI-008 and implementation delivery | REVIEW HANDOFF | Review must verify current-head Backend Functional Coverage, record package-level directional performance in a PR comment, and own terminal CI, conflict resolution, and merge. Implementation stops after the final head is pushed, the PR is open, CI has started, and blocking feedback is addressed. | CI terminal result and merge remain review-owned. |

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
3. Execute the packaged, Current/Definitions, and acceptance count-one commands through production root composition. Each exited 0 with the timings recorded above.
4. Execute each owned package group with `-count=3`. Current/Definitions and acceptance passed on the first run. The first packaged aggregate run exposed a non-reproducible failure in the untouched `goal` package; its bounded package retry and one exact aggregate rerun both passed, so no production or shared-support repair was made.
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
| C07-F001 | NON-BLOCKING planning delta | Run DISC-001 exactly on the clean tracked head. | The current plan denominator and live executable denominator agree. | PRD declares 21/106; the head has 22/99, including `named_invocation` and consolidated shared parents. All live rows are accounted for and no path was changed by this loopback. | Discovery commands and census table above; progress entries for Stories 001–003. |
| C07-F002 | NON-BLOCKING planning input | Resolve the PRD `sourcePlan` path and referenced task packet. | The parent plan and task packet are available for independent comparison. | `docs/temp/functional-test-optimization.md` and the referenced task file are absent in this checkout; the PRD, standards, current diff, and progress log were used as the available authority. | `Test-Path`/`rg --files` checks; absence already recorded in progress. |
| C07-F003 | NON-BLOCKING, resolved | `go test -count=3 -timeout=45m ./tests/functional/factory/packaged/...` on the clean head. | The repeat command exits 0. | The first aggregate attempt exited 1 in untouched `packaged/goal`; its one package retry exited 0 and the exact aggregate rerun exited 0. No diff-caused failure or cleanup residue was reproduced. | First aggregate and bounded retry/rerun outputs above. |
| C07-F004 | REVIEW-STAGE | Push the final report head and inspect the required PR check. | Current-head Backend Functional Coverage has started and review can record the package timing/result. | CI cannot start until the final head is pushed and the PR is opened; terminal CI and merge are explicitly review-owned. | GATE-PR-CI-008 handoff row above. |

## Plan deltas requested

- Affected behavior and criterion: DISC-001 denominator and the source-plan/task-packet references.
- Root-cause evidence or remaining uncertainty: the live tracked head contains one additional packaged package and consolidated top-level test parents, while the referenced `docs/temp` plan and task packet are not present. The current executable rows and named records are fully accounted for, but the intended historical denominator cannot be inferred safely.
- Smallest recommended correction/prerequisite: in a planning follow-up, reconcile the PRD/source-plan denominator to the current inventory and restore or attach the canonical plan/task packet; do not change functional tests or shared support as part of this C07 loopback.
- Dependencies and retest scope: planning-owner decision only; if the denominator is changed, rerun DISC-001 and the report reconciliation. No C07 implementation repair is requested.

## Verdict

PASS for the local clean-room validation and owned cleanup evidence. The known denominator/source-plan discrepancies are explicit planning deltas, not silent scope changes. Implementation delivery is a review-stage handoff: push the final report head, open the PR, start required CI, address any blocking feedback, and stop; review owns terminal CI, conflict resolution, and merge.
