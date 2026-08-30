# Validation report: cached-input pricing and source-session lifecycle

This is the final local validation ledger for
`goal3-cached-input-pricing-and-source-lifecycle-close` after the review
correction. The earlier clean-room delta plan and the reviewer-reported
process write-amplification failure are retained in the worktree history; this
report records the corrected implementation head. Hosted CI evidence belongs in
the pull request conversation and is intentionally absent here.

## Environment and artifact

- Validated implementation head: `ac664c02ce6bc8ea5b044fb0571f8eec4a1db158` (`fix(runtime): clarify terminal persistence gate`); the behavior fix is its parent `c237166971ea68215e2f35ae9b6045bf3cf581e3` (`fix(runtime): coalesce terminal recording persistence`).
- Baseline identifier: `1552d3e3ef114eeb68b1828d46db7d686ad1ef33`, the ancestor before the two implementation stories; the final rebase target was `origin/main` at `905d5fe06e54f2cc18fa9d5ed4f08d72e5d85739`.
- Environment and configuration: Windows `10.0.26100`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, repository `go.mod`, local Go/build caches, and the repository's installed UI dependencies.
- Customer entry point: local-real `root.BuildProcess` composition, Factory Session runtime/recording, the public metrics costs query, and controlled provider command output.
- Real and substituted dependencies: real application composition, filesystem, session/recording persistence, replay, configuration, and metrics query; a deterministic counted provider command is injected only for the controlled functional witness.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0.00`. Controlled evidence proved the material cached-input edge, so `GATE-LIVE-PRICE` was not triggered.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Exact cached-input pricing | PASS | The focused pricing matrix proves `input=10000`, `cached=9984`, `output=100` at built-in `0.002268` USD for `openai/gpt-5-codex`; complete operator cached rate `0.50` yields `0.006012`; omission is `UNPRICED` with a null amount and missing-rate reason; explicit zero yields exact arithmetic `0.001020` and canonical serialized `0.00102`. | Remote vendor billing and historical backfill are out of scope. |
| Current default coverage and precedence | PASS | Providers unit inventory retains the current Codex rows and adds the verified Claude Sonnet 5 row with source/date/cache rate; complete customer rows retain precedence. | Future catalog additions and live price drift. |
| Durable source lifecycle and lineage | PASS | Runtime completion tests prove one coalesced terminal persistence gate, result-before-completion ordering, deferred completion publication, and retryable injected gate failure. The local-real successor witness proves the reopened successor retains exact source/successor identities and usage rows. | Historical backfill and remote provider behavior. |
| Process shutdown persistence cadence | PASS | `TestRecordingFlushBeforeProcessExecuteReturns` passes through `root.BuildProcess`/`Process.Execute`: durable baseline `7` events/`2` writes/`16288` bytes becomes `14` events/`3` writes/`31830` bytes, including one final failure response and `15542` final-flush bytes. The final shutdown delta is exactly one whole-file write. | Other storage implementations and host platforms. |
| Clean-room failure handling | PASS | The previously reported maintainability/package/evidence deltas were recorded as a delta plan, then resolved in bounded implementation commits. The final evidence pass was run on the rebased clean worktree without validator repairs. | Linux-hosted static comparison remains external evidence. |
| Paid validation disposition | PASS | The controlled local-real provider witness proved positive cached usage and exact default/override/absence behavior; no paid call was attempted. | Real vendor payload variation. |
| Sanitized evidence ledger | PASS | This report records baseline/final behavior, dependency fidelity, budget, commit identifiers, the review correction, and remaining edges without credentials or CI results. | Terminal CI and merge are review-owned. |
| Repository compatibility/static policy | PASS* | `make verify-fast` passed. `make lint` ran all 23 targets: 22 passed, while only the Windows deadcode snapshot comparison returned nonzero (`3072` current findings versus `3074` Linux-shaped baseline findings). The exact difference is the unchanged Unix/Windows helper swap; no current-only finding is from this diff. | Linux-hosted deadcode comparison and terminal CI. |
| Implementation-stage delivery | READY | The corrected head is ready for the final push, existing PR update, and CI-start handoff. Hosted terminal status is not claimed in this local ledger. | Push, CI start, and any new review feedback. |

## Customer journey

1. The corrected implementation was rebased onto current `origin/main`; the final local worktree was clean before this documentation update, and `git diff --check origin/main...HEAD` passed.
2. The pricing unit and local-real functional matrix observed built-in `0.002268`, operator `0.006012`, omitted cached-rate `UNPRICED`/null, explicit zero `0.00102`, invalid-rate failures, and one controlled provider call; subsequent cost queries made no provider call.
3. The runtime completion unit tests observed one coalesced terminal gate and preserved `SESSION_RESULT_UPDATED` before `SESSION_COMPLETED`. The process witness observed the persisted final response and exactly one additional whole-file write after the durable dispatch baseline. The successor witness reopened the source/successor path and retained both canonical usage rows.
4. `make verify-fast` completed successfully: 216 native UI tests, 3,108 dashboard tests, 447 Go packages, and 169 repository validation tests passed (168 passed and one documented platform-specific skip in the final validation suite).
5. `make lint` reached every target. Formatting, vet, size, package shape/count, boundaries, catalog, ownership, and contract checks passed; only the pre-existing Windows/Linux deadcode snapshot mismatch remains for hosted Linux confirmation.

## Cross-task integration and usability

- Documentation discoverability: this report is the canonical handoff artifact at `docs/internal/development/goal3-cached-input-pricing-and-source-lifecycle-close.md`.
- Permission and error behavior: negative/malformed cached rates and impossible subsets fail; omitted cached usage is distinct from explicit zero; incomplete source closure and injected persistence failure remain unadvertised.
- Persistence/reload behavior: the process artifact and successor functional witness inspect persisted sequence/order and canonical source/successor identities.
- Accessibility/keyboard/responsive behavior: not applicable; this lane changes Go runtime, pricing, recording, and functional evidence surfaces, not UI behavior.
- Operational signals: provider call counts, persisted event sequences, session/source/successor identities, usage-row counts, write counts/bytes, status/reason fields, and metrics selector output are asserted at their owning boundaries.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| G03-F001 | resolved | `go test -count=1 -timeout 15m ./tests/functional/recordings/process` | Orderly `Process.Execute` shutdown adds one final whole-file recording write after the durable dispatch baseline. | The pre-fix head added three writes; the corrected head adds one (`2` to `3`) while retaining the terminal events (`7` to `14`). | `TestRecordingFlushBeforeProcessExecuteReturns`; behavior fix `c237166971`. |
| G03-F002 | resolved | Runtime completion unit and race witnesses | Result and completion remain ordered, durable, single-shot, and retryable after an injected gate failure. | Both events are appended before one terminal gate; a failed injected gate does not publish completion and the retry succeeds; concurrent calls produce one completion. | `TestRuntimeCompletionDurablyClosesSourceForSuccessorMetrics`, `TestRuntimeCompletionFlushFailureLeavesSourceIncompleteAndRetryable`, `TestRuntimeCompletionConcurrentCallbacksRemainExactlyOnce`. |
| G03-F003 | resolved | `git merge-base --is-ancestor 1552d3e3ef114eeb68b1828d46db7d686ad1ef33 ac664c02ce6bc8ea5b044fb0571f8eec4a1db158` | The ledger's evidence head is an ancestor of the delivered implementation. | The final report identifies the rebased implementation head and its behavior-fix parent; the baseline is an ancestor. | `git merge-base --is-ancestor`; this report. |
| G03-F004 | environment-limited | `make lint` / `make deadcode` on Windows | Current normalized deadcode findings match the committed Linux-shaped baseline. | Current `3072`, baseline `3074`; current-only findings are Windows repository-staging and terminal-lock helpers, while baseline-only findings are Unix counterparts plus Unix process-test helpers. None is in this diff and the baseline was not changed. | `bin/deadcode-current.txt`, `docs/internal/baselines/deadcode-baseline.txt`; hosted Linux check remains required. |
| G03-F005 | resolved | Controlled functional pricing witness | Material cached-input valuation is proven without paid validation when the fixture supplies the recorded cached usage. | The counted provider was called once for recording and zero times for cost re-queries; no paid call was used. | Pricing functional matrix and call-count assertions. |

## Sanitized evidence ledger

| Evidence | Head/baseline | Dependency fidelity | Exact result | Budget | Remaining edge |
| --- | --- | --- | --- | --- | --- |
| Baseline behavior | `1552d3e3ef114eeb68b1828d46db7d686ad1ef33` | Existing committed implementation | Cached-input Claude coverage and the characterized durable source close were not yet present in the reviewed lane. | None | Historical context only. |
| Pricing matrix | `ac664c02ce` | Local-real root/session/recording/config/query with controlled counted provider | Built-in `0.002268`; operator `0.006012`; omitted `UNPRICED`/null; explicit zero `0.00102`; one recording provider call and zero cost-query calls. | `$0.00`, zero paid calls | Other vendors/models and live billing. |
| Lifecycle and process cadence | `ac664c02ce` | Local-real runtime/session/recording persistence plus injected write probe | One ordered terminal close; process artifact `7` events/`2` writes before stop and `14`/`3` after stop; exactly one final write delta; response publication waits for the successful terminal gate. | Local compute only | Other storage implementations and hosted terminal CI. |
| Successor lineage | `ac664c02ce` | Local-real recording/replay/session/metrics path with controlled provider | Reopened successor retains exact source/successor identities and both usage rows; incomplete source is rejected without a successor metric row. | `$0.00`, zero paid calls | Historical backfill. |
| Focused/race/functional gates | `ac664c02ce` | Go package tests, race detector, and local-real functional root | Pricing packages, lifecycle packages, coalesced completion tests, process regression, pricing matrix, successor-lineage witness, and lifecycle race witness passed. | Local compute only | Full hosted terminal result. |
| GATE-VERIFY-FAST | `ac664c02ce` | Repository-local UI, Go, and validation lanes | PASS: 216 native UI tests, 3,108 dashboard tests, 447 Go packages, and 169 validation tests with one documented platform skip. | Local compute only | Hosted CI terminal result. |
| GATE-LINT | `ac664c02ce` | Repository-local static policy checks | 22 of 23 targets passed; deadcode is the unchanged Windows/Linux snapshot mismatch (`3072` vs `3074`). No diff-introduced current-only symbol was found. | Local compute only | Hosted Linux deadcode result and terminal CI. |
| Delivery | `ac664c02ce` | Final rebased implementation head before external handoff | Ready for final push and existing PR update; CI evidence is intentionally not written into this commit. | No paid/external provider calls | PR open/update, CI start, review-owned terminal CI, conflicts, and merge. |

## Verdict

PASS for the corrected local implementation stage, with the Windows-only
deadcode snapshot comparison explicitly retained as an environment edge for
hosted Linux CI. The reviewer-reported process write amplification is fixed and
covered by the process-level regression; the integrated pricing, source close,
and successor lineage behavior passes with zero paid calls. The final rebased
head is ready for the required push, PR update, and CI-start handoff.

## Remaining edges and handoff

- Push the final head and update PR #2473 using `prd.json` as its description.
- Confirm required CI has started on the final head and respond to the blocking review comment with the corrected commit and exact local evidence.
- Keep hosted CI evidence in the PR conversation, never in a commit. Terminal CI, conflict resolution, and merge remain review-owned.
- Historical backfill, real vendor billing, and browser verification remain out of scope for this non-UI lane.
