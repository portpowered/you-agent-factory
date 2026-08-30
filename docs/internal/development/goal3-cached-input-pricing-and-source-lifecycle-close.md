# Validation report: cached-input pricing and source-session lifecycle

This is the final local validation ledger for
`goal3-cached-input-pricing-and-source-lifecycle-close`. The initial clean-room
check identified bounded implementation deltas; the implementation follow-up
applied only those behavior-preserving corrections. CI results are intentionally
not recorded here.

## Environment and artifact

- Commit/build identifier: behavior-validation code head `937491027f1dece779616b363c05a8aa9313c05b`.
- Baseline identifier: `1552d3e3ef114eeb68b1828d46db7d686ad1ef33`, before the two implementation stories.
- Environment and configuration: Windows `10.0.26100`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, repository `go.mod`, local Go/build caches, and the repository's installed UI dependencies.
- Customer entry point: local-real `root.BuildProcess` composition, Factory Session runtime/recording, the public metrics costs query, and controlled provider command output.
- Real and substituted dependencies: real application composition, filesystem, session/recording persistence, replay, configuration, and metrics query; a deterministic counted provider command is injected only for the controlled functional witness.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0.00`. The controlled witness proved the material cached-input edge, so GATE-LIVE-PRICE was not triggered.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Integrated pricing and lifecycle behavior | PASS | The declared pricing functional matrix, focused package tests, race witness, and successor-lineage functional witness passed. The final matrix records `10000` input, `9984` cached input, and `100` output tokens as `PRICED` at built-in `0.002268` USD for `openai/gpt-5-codex`; the operator row produces `0.006012` USD, omission produces `UNPRICED` with a null amount and reason `cached-input rate is not configured`, and explicit zero produces exact arithmetic `0.001020` (canonical serialized form `0.00102`). The lifecycle witness observes the final dispatch response, one persisted `SESSION_COMPLETED`, flush-before-advertisement ordering, and the exact source/successor lineage rows. | Remote vendor billing and historical backfill remain out of scope. |
| Clean-room failure handling | PASS | The initial findings were converted into the bounded correction plan: lifecycle helpers were moved into existing package surfaces, duplicate test helpers/declarations were consolidated, and the affected size/package-shape gates were rerun successfully. | Linux-hosted deadcode comparison remains to be confirmed. |
| Paid validation disposition | PASS | Controlled local-real provider output proved positive cached usage, exact default/override/absence behavior, and zero provider calls during cost re-query; no paid call was attempted. | Real vendor payload variation is intentionally not exercised. |
| Sanitized evidence ledger | PASS | The ledger below records baseline/final behavior, dependency fidelity, budget, implementation head, gate results, and remaining edges without credentials or CI results. | Terminal CI and merge are review-owned. |
| Repository compatibility/static policy | PASS* | `make verify-fast`, `make fmt-check`, `make backend-size`, `make pkg-file-count`, and `make contracts-check` passed on the final code head. `make lint` passed every changed-surface/static target; its only local nonzero result is the platform-sensitive deadcode snapshot comparison described in G03-F004. | Linux-hosted deadcode comparison and terminal CI remain unproven locally. |
| Implementation-stage delivery | READY | The final code head is ready for the required rebase, push, open PR, and CI-start handoff. The implementation stage stops after CI starts; terminal CI, conflicts, and merge are review-owned. | Hosted CI and review feedback. |

## Customer journey

1. The clean implementation worktree was clean before validation. The focused pricing command and the lifecycle command exercised the application through the real root/session/recording/metrics path with only the provider response controlled.
2. The pricing matrix observed the built-in Codex rate, complete operator replacement, omitted cached subclass, explicit zero subclass, invalid token subsets, and the retained Claude/Codex catalog rows. The functional matrix's counted provider was called once for the recording; subsequent cost queries made no provider call.
3. The lifecycle witness persisted `SESSION_RESULT_UPDATED`, then exactly one `SESSION_COMPLETED`, and only then completed the response/subscription path. It also proves append/flush failure remains unadvertised and retryable, an incomplete source cannot become a predecessor, and a reopened successor retains canonical source/successor identities and both usage rows.
4. `make verify-fast` completed successfully: typecheck passed, 216 native UI tests passed, 3,108 dashboard tests passed, 447 Go packages passed, and the 169-test repository validation suite passed with one documented platform-specific skip.
5. The bounded corrections made the implementation-size, package-count, formatting, and functional-evidence checks pass. On Windows, `make lint` still reports only the platform-sensitive deadcode snapshot mismatch; no current-only symbol is introduced by the diff, so the Linux-hosted check remains the authoritative edge. The final head is ready for push/PR/CI handoff; no terminal CI result is claimed here.

## Cross-task integration and usability

- Documentation discoverability: this report is the canonical handoff artifact at `docs/internal/development/goal3-cached-input-pricing-and-source-lifecycle-close.md`.
- Permission and error behavior: negative and impossible cached subsets are rejected; omitted cached usage is distinct from explicit zero; incomplete source closure and append/flush failures are covered by the lifecycle tests.
- Persistence/reload behavior: canonical event ordering is checked from persisted sequence numbers, and the successor witness reopens the source/successor path before querying metrics.
- Accessibility/keyboard/responsive behavior: not applicable; this lane changes Go runtime, pricing, recording, and functional evidence surfaces, not UI behavior.
- Operational signals: provider call counts, persisted event sequence, session/source/successor identities, usage-row counts, status/reason fields, and metrics selector output are asserted at their owning boundaries.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| G03-F001 | resolved | `make backend-size` and `make pkg-maint` | Changed files remain within the repository's backend file/function limits. | Lifecycle helpers were moved into existing package files and `buildBundle` was reduced below the limit; the final gates passed. | Final code head `937491027f1dece779616b363c05a8aa9313c05b`. |
| G03-F002 | resolved | `make pkg-file-count` | New package files do not exceed the deletion-only baseline. | Adjacent tests were consolidated and the final package counts passed the exact deletion-only baseline. | Final code head `937491027f1dece779616b363c05a8aa9313c05b`. |
| G03-F003 | resolved | `make contracts-check` | Every functional evidence declaration maps a stable ID to its supported evidence shape. | The duplicate `cli/you.metrics.costs` declaration was removed; the successor-lineage witness has its own supported metrics evidence mapping, and the checker passed. | `contracts/functional-scenario-evidence.json`, `contracts/functional-scenarios.json`. |
| G03-F004 | environment-limited | `make deadcode` | The current deadcode report matches the checked-in baseline. | Windows reports 3,072 current findings against the Linux-shaped 3,074 baseline. Current-only findings are the Windows repository-staging and terminal-lock helpers; baseline-only findings are their Unix counterparts plus Unix process-test helpers. No current-only symbol is from this diff, and the baseline was not changed. | `bin/deadcode-current.txt`, `docs/internal/baselines/deadcode-baseline.txt`; Linux CI remains the required comparison. |
| G03-F005 | resolved environment note | Initial `make verify-fast` | The required UI typecheck can resolve its declared Bun types. | The initial run lacked `ui/node_modules/bun-types`; one `make ui-deps` setup installed the repository dependencies, after which the final committed-head verify-fast run passed. | Local command results; no source or test changes. |

## Sanitized evidence ledger

| Evidence | Head/baseline | Dependency fidelity | Exact result | Budget | Remaining edge |
| --- | --- | --- | --- | --- | --- |
| Baseline behavior | `1552d3e3ef114eeb68b1828d46db7d686ad1ef33` | Existing committed implementation | Cached-input pricing and durable successor close were not yet present in the reviewed lane. | None | Baseline is historical context only. |
| Pricing matrix | `937491027f1dece779616b363c05a8aa9313c05b` | Local-real root/session/recording/config/query with controlled counted provider | Built-in `0.002268`; operator `0.006012`; omitted `UNPRICED`/null; explicit zero `0.001020` arithmetic and `0.00102` serialized. One recording provider call; zero cost-query provider calls. | `$0.00`, zero paid calls | Other vendors/models and live billing. |
| Lifecycle/lineage | `937491027f1dece779616b363c05a8aa9313c05b` | Local-real runtime/session/recording persistence and replay | One ordered `SESSION_COMPLETED`; append/flush failure remains retryable and unadvertised; incomplete source rejected; reopened successor retains exact source/successor identities and two usage rows. | `$0.00`, zero paid calls | Historical backfill. |
| Focused/race/functional gates | `937491027f1dece779616b363c05a8aa9313c05b` | Go package tests, race detector, local-real functional root | Pricing packages, lifecycle packages, `TestRuntimeCompletionDurablyClosesSourceForSuccessorMetrics`, pricing matrix, and `TestRuntimeMetricsSuccessorLineageAfterSourceLifecycleClose` passed. | Local compute only | Full CI terminal result. |
| GATE-VERIFY-FAST | `937491027f1dece779616b363c05a8aa9313c05b` | Repository-local UI, Go, and validation lanes | PASS after one dependency setup; 216 native UI tests, 3,108 dashboard tests, 447 Go packages, and 169 validation tests passed; one platform skip. | Local compute only | Hosted CI terminal result. |
| GATE-LINT | `937491027f1dece779616b363c05a8aa9313c05b` | Repository-local static policy checks | All changed-surface, format, boundary, catalog, ownership, package-count, and contract targets passed. Windows-only deadcode snapshot comparison remains environment-limited as G03-F004; no diff-introduced current-only symbol was found. | Local compute only | Linux-hosted deadcode comparison and terminal CI. |
| Delivery | `937491027f1dece779616b363c05a8aa9313c05b` | Final local implementation head, before external handoff | Ready for final rebase, push, PR opening, and CI start. This ledger contains no hosted CI result. | No paid/external provider calls | Review-owned terminal CI, conflicts, and merge. |

## Verdict

PASS for the local implementation stage, with the Windows-only deadcode snapshot
comparison explicitly retained as an environment edge for Linux CI. The bounded
clean-room delta plan is complete, the integrated runtime behavior and controlled
cost budget pass, and the final head is ready for the required external handoff.

## Remaining edges and handoff

- Rebase `origin/main` immediately before the final push, after the required C14 path preflight; rerun affected gates if the rebase changes the code head.
- Push the final head, open or update the named PR using `prd.json` as its description, and confirm required CI has started on that head.
- Record hosted CI status and any review feedback in the PR conversation, never in a commit. Terminal CI, conflict resolution, and merge remain review-owned.
