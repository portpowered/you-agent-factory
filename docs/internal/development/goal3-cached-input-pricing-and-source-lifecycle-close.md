# Validation report: cached-input pricing and source-session lifecycle

This is the implementation-stage, no-repair validation record for
`goal3-cached-input-pricing-and-source-lifecycle-close`. The validator did not
edit product code or tests after the clean-room checks identified failures.

## Environment and artifact

- Commit/build identifier: `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` under validation.
- Baseline identifier: `1552d3e3ef114eeb68b1828d46db7d686ad1ef33`, before the two implementation stories.
- Environment and configuration: Windows `10.0.26100`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, repository `go.mod`, local Go/build caches, and the repository's installed UI dependencies.
- Customer entry point: local-real `root.BuildProcess` composition, Factory Session runtime/recording, the public metrics costs query, and controlled provider command output.
- Real and substituted dependencies: real application composition, filesystem, session/recording persistence, replay, configuration, and metrics query; a deterministic counted provider command is injected only for the controlled functional witness.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0.00`. The controlled witness proved the material cached-input edge, so GATE-LIVE-PRICE was not triggered.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Integrated pricing and lifecycle behavior | PASS | The declared pricing functional matrix, focused package tests, race witness, and successor-lineage functional witness passed. The final matrix records `10000` input, `9984` cached input, and `100` output tokens as `PRICED` at built-in `0.002268` USD for `openai/gpt-5-codex`; the operator row produces `0.006012` USD, omission produces `UNPRICED` with a null amount and reason `cached-input rate is not configured`, and explicit zero produces exact arithmetic `0.001020` (canonical serialized form `0.00102`). The lifecycle witness observes the final dispatch response, one persisted `SESSION_COMPLETED`, flush-before-advertisement ordering, and the exact source/successor lineage rows. | Remote vendor billing and historical backfill remain out of scope. |
| Clean-room failure handling | PASS | `make lint` failures are recorded below as a delta-plan request. No product code or tests were repaired by this validation story. | Owner resolution of the implementation delta. |
| Paid validation disposition | PASS | Controlled local-real provider output proved positive cached usage, exact default/override/absence behavior, and zero provider calls during cost re-query; no paid call was attempted. | Real vendor payload variation is intentionally not exercised. |
| Sanitized evidence ledger | PASS | The ledger below records baseline/final behavior, dependency fidelity, budget, implementation head, gate results, and remaining edges without credentials or CI results. | Terminal CI and merge are review-owned. |
| Repository compatibility/static policy | FAIL | `make verify-fast` passed after one environment-only `make ui-deps` setup. `make lint` failed in `backend-size`, `pkg-maint`, `pkg-file-count`, `deadcode`, and `contracts-check`; exact findings are in G03-F001 through G03-F004. | The implementation owner must apply the smallest behavior-preserving correction and rerun the gates. |
| Implementation-stage delivery | BLOCKED | The lint failure prevents the final rebase/push, PR opening, and CI start in this no-repair iteration. | Review-stage terminal CI, conflict resolution, and merge. |

## Customer journey

1. The clean implementation worktree was clean before validation. The focused pricing command and the lifecycle command exercised the application through the real root/session/recording/metrics path with only the provider response controlled.
2. The pricing matrix observed the built-in Codex rate, complete operator replacement, omitted cached subclass, explicit zero subclass, invalid token subsets, and the retained Claude/Codex catalog rows. The functional matrix's counted provider was called once for the recording; subsequent cost queries made no provider call.
3. The lifecycle witness persisted `SESSION_RESULT_UPDATED`, then exactly one `SESSION_COMPLETED`, and only then completed the response/subscription path. It also proves append/flush failure remains unadvertised and retryable, an incomplete source cannot become a predecessor, and a reopened successor retains canonical source/successor identities and both usage rows.
4. `make verify-fast` completed successfully: typecheck passed, 216 native UI tests passed, 3,108 dashboard tests passed, 447 Go packages passed, and the 169-test repository validation suite passed with one documented platform-specific skip.
5. `make lint` reached all named targets but failed the implementation-size/package-shape, deadcode, and functional-evidence checks listed below. The validator stopped without repair, so no final head was pushed and no PR or CI result is claimed.

## Cross-task integration and usability

- Documentation discoverability: this report is the canonical handoff artifact at `docs/internal/development/goal3-cached-input-pricing-and-source-lifecycle-close.md`.
- Permission and error behavior: negative and impossible cached subsets are rejected; omitted cached usage is distinct from explicit zero; incomplete source closure and append/flush failures are covered by the lifecycle tests.
- Persistence/reload behavior: canonical event ordering is checked from persisted sequence numbers, and the successor witness reopens the source/successor path before querying metrics.
- Accessibility/keyboard/responsive behavior: not applicable; this lane changes Go runtime, pricing, recording, and functional evidence surfaces, not UI behavior.
- Operational signals: provider call counts, persisted event sequence, session/source/successor identities, usage-row counts, status/reason fields, and metrics selector output are asserted at their owning boundaries.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| G03-F001 | blocking | `make backend-size` and `make pkg-maint` | Changed files remain within the repository's backend file/function limits. | Four changed surfaces exceed the gate: `function buildBundle` in `pkg/services/factory_runtime/internal/runtime_build.go` is 102 lines (limit 100); `pkg/services/factory_runtime/internal/services/orchestration/runtime/factory.go` is 1,061 lines (limit 1,000); `pkg/services/factory_sessions/internal/sessionservice/assembly.go` is 1,013 lines (limit 1,000); `pkg/services/recordings/internal/events/event_history.go` is 1,037 lines (limit 1,000). | Targeted gate output on implementation head `1dded504f2`. |
| G03-F002 | blocking | `make pkg-file-count` | New package files do not exceed the deletion-only baseline. | `pkg/services/factory_runtime/internal/services/orchestration/runtime` grew from 37 to 38 files and `pkg/services/recordings/internal` grew from 20 to 22 files. | Targeted gate output on implementation head `1dded504f2`. |
| G03-F003 | blocking | `make contracts-check` | Every functional evidence declaration maps a stable ID to its supported evidence shape. | `TestRuntimeCostsCachedInputConfigurationMatrix` declares `cli/you.metrics.costs`, which is already declared by `TestRuntimeCostsEndToEndFromProviderCompletion`; the checker reports the new declaration's stable ID as unused. | `contracts/functional-scenario-evidence.json`, `contracts/functional-scenarios.json`, and `tests/functional/factory/visualization/runtime_metrics/end_to_end_costs_test.go`. |
| G03-F004 | blocking pending owner review | `make deadcode` | The current deadcode report matches the checked-in baseline. | The checker reports baseline drift with equal counts, 3,074 baseline findings and 3,074 current findings. The current report includes changed test-support symbols, but this validator did not decide whether a baseline update is intentional. | `bin/deadcode-current.txt` and `docs/internal/baselines/deadcode-baseline.txt`; no baseline file was changed. |
| G03-F005 | resolved environment note | Initial `make verify-fast` | The required UI typecheck can resolve its declared Bun types. | The initial run lacked `ui/node_modules/bun-types`; one `make ui-deps` setup installed the repository dependencies, after which the single permitted verify-fast rerun passed. | Local command results; no source or test changes. |

## Sanitized evidence ledger

| Evidence | Head/baseline | Dependency fidelity | Exact result | Budget | Remaining edge |
| --- | --- | --- | --- | --- | --- |
| Baseline behavior | `1552d3e3ef114eeb68b1828d46db7d686ad1ef33` | Existing committed implementation | Cached-input pricing and durable successor close were not yet present in the reviewed lane. | None | Baseline is historical context only. |
| Pricing matrix | `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` | Local-real root/session/recording/config/query with controlled counted provider | Built-in `0.002268`; operator `0.006012`; omitted `UNPRICED`/null; explicit zero `0.001020` arithmetic and `0.00102` serialized. One recording provider call; zero cost-query provider calls. | `$0.00`, zero paid calls | Other vendors/models and live billing. |
| Lifecycle/lineage | `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` | Local-real runtime/session/recording persistence and replay | One ordered `SESSION_COMPLETED`; append/flush failure remains retryable and unadvertised; incomplete source rejected; reopened successor retains exact source/successor identities and two usage rows. | `$0.00`, zero paid calls | Historical backfill. |
| Focused/race/functional gates | `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` | Go package tests, race detector, local-real functional root | Pricing packages, lifecycle packages, `TestRuntimeCompletionDurablyClosesSourceForSuccessorMetrics`, pricing matrix, and `TestRuntimeMetricsSuccessorLineageAfterSourceLifecycleClose` passed. | Local compute only | Full CI terminal result. |
| GATE-VERIFY-FAST | `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` | Repository-local UI, Go, and validation lanes | PASS after one dependency setup; 216 native UI tests, 3,108 dashboard tests, 447 Go packages, and 169 validation tests passed; one platform skip. | Local compute only | Hosted CI terminal result. |
| GATE-LINT | `1dded504f2e152c9feb7e60af97c3aa02e2e2d49` | Repository-local static policy checks | BLOCKED by G03-F001 through G03-F004. Format, boundary, catalog, ownership, and other completed targets passed. | Local compute only | Implementation-owner delta and rerun. |
| Delivery | Not reached | Real Git host/PR not invoked because GATE-LINT failed | No rebase, push, PR, or CI result is claimed in this sanitized ledger. | No external calls | Final rebased head, open PR, started CI, and review feedback. |

## Verdict

FAIL for implementation-stage delivery because GATE-LINT has blocking findings. The integrated runtime behavior and controlled cost budget pass, and the required no-repair disposition is recorded below.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: GATE-LINT and the implementation-stage delivery criterion; the product behavior proven by the focused and functional witnesses must remain unchanged.
- Root-cause evidence or remaining uncertainty: G03-F001 and G03-F002 identify size and package-count growth in files/packages changed by the implementation stories. G03-F003 identifies a duplicate stable-ID declaration in the new functional evidence. G03-F004 identifies deadcode baseline content drift with equal finding counts; intentionality and the correct baseline owner remain unresolved.
- Smallest recommended correction/prerequisite: implementation owners should split or otherwise reduce the four over-limit production surfaces, consolidate or relocate the added package files while preserving ownership, and correct the functional evidence declaration so one stable ID has one supported evidence mapping. The baseline owner should inspect the deadcode diff and update the baseline only if the changed findings are intentional. Do not alter the exact pricing, lifecycle ordering, retryability, or lineage assertions while making these corrections.
- Dependencies and retest scope: rerun the focused pricing/lifecycle packages, race witness, both named functional witnesses, `make verify-fast`, and `make lint`; then perform the C14 path preflight, rebase `origin/main`, rerun affected gates, and only after a clean result push/open the PR and verify CI has started. CI terminal status, conflicts, blocking review feedback, and merge remain review-owned. CI-run evidence must be recorded in a PR comment, never in a commit.

