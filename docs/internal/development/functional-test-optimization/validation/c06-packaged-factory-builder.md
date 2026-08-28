# Validation report: C06 packaged Factory Builder

## Environment and artifact

- Commit/build identifier: `33bc2c7c92b50d384c09cc2b11802d377fc628ca` (implementation head validated before this report-only artifact).
- Base comparison: merge-base with `origin/main` was `67710223e327d02c0de93a6ad826c754fe5c1702`.
- Environment and configuration: Windows 11 Home `10.0.26200`, `windows/amd64`, Go `1.25.0`; tracked Git status was clean before validation; task packet files remained ignored and outside the product diff.
- Customer entry point: the package's `TestFactoryBuilder` parent builds one reusable process and exercises Factory Builder through the public Factory Session HTTP contract; installed JavaScript execution uses the public durable sync contract.
- Real and substituted dependencies: `root.BuildProcess`/production wiring, packaged Factory loading, Factory Sessions, Factory Runtime, Work, Recordings, Workers, filesystem, and loopback HTTP were exercised locally. Only the provider command runner and API listener start edge were controlled through `edges.Edges`; no remote provider or paid call was used.
- Cost/call budget used: `$0`; no credentials, customer data, non-loopback network calls, or paid calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| C06 inventory and assertion preservation | PASS | The baseline at the merge-base contains the five P011 top-level rows; the invalid row expands into `new_destination` and `existing_destination`. The final parent retains all six exact scenario names as subtests in `greeting_test.go:58-74`. The detailed mapping is below. | No direct Factory Event assertion exists in either the baseline or final package source; this report does not claim a new event-specific assertion. Broader P011/project inventory remains with its owning gate. |
| C06 focused functional proof | PASS | `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/factory_builder` exited `0` in `5.918s`. | Does not prove remote provider behavior, OS hard-kill cleanup, race safety, project-wide timing, terminal CI, or merge. |
| C06 repeat functional proof | PASS | `go test -count=3 -timeout=30m ./tests/functional/factory/packaged/factory_builder` exited `0` in `27.154s`; all three package runs completed without a flake or leakage failure. | Does not prove race-detector safety, project-wide suite behavior, or remote provider behavior. |
| C06 lifecycle and cleanup | PASS | A verbose focused run exited `0` in `5.817s` and emitted `process_starts=1 explicit_sessions_opened=6 explicit_sessions_closed=6 unique_session_ids=6 scenario_roots_removed=6 runtime_artifacts=0 isolated_rows=0`. The source registers cleanup before assertions and closes sessions, checks public absence, closes response bodies, unregisters provider routes, rejects post-shutdown `/status`, and removes roots (`shared_fixture_test.go:284-334`, `362-447`). | The lifecycle line records the declared zero artifact/row fields; project-wide artifact and isolation probes remain outside this package story. |
| C06 clean-head scope and compatibility audit | PASS | `git diff --check $(git merge-base origin/main HEAD) HEAD` was clean. Before this report artifact, the implementation diff contained only `greeting_test.go`, `invocation_test.go`, and `shared_fixture_test.go` under the owned package. No shared support, sibling, contract, generated, definition, baseline, UI, or CI file changed. | Does not prove terminal CI or merge. |
| C06 validation-loopback report shape and fidelity | PASS | This report follows `factory/docs/standards/validation-loopback-template.md`, records the exact commands and artifact, and classifies the proof as functional/local-real with a controlled provider edge. | Does not claim remote provider, OS crash, project-wide timing, terminal CI, conflict resolution, or merge evidence. |

### Six-case assertion inventory

| Case / final scenario | Preserved observable assertions |
| --- | --- |
| CASE-H01 / `TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding` | Completed invocation; exactly one routing classification; zero build prompts; exactly one help prompt; all five authored guidance fragments remain required (`greeting_test.go:77-116`). |
| CASE-B02 / `TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing` | Bare invocation completes and does not call the build workstation (`greeting_test.go:119-135`). |
| CASE-H03 / `TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory` | Builder completion text/status and no failure diagnostics; workspace-scoped staging; exact operator-root persistence command; no project-local shadow; installed definition validation/listing; graph name, Work type, worker, `init`/`complete`/`failed` routes; installed run return/signature and final text; one installed provider call (`invocation_test.go:40-74`, `291-443`, `510-682`). |
| CASE-H04 / `TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory` | Builder completion text/status; workspace staging and exact persistence; installed definition/listing; non-empty UTF-8 inline source; required briefs; `DEFAULT` permissions; `maxAgents=2` and concurrency `2`; durable installed run with final JSON summary and `analysisCount=2`; two provider calls (`invocation_test.go:76-110`, `344-508`). |
| CASE-U05 / `.../new_destination` | Completed safe validation guidance containing Factory name, validation failure, diagnostic code, missing worker, correction, and not-installed result; redaction of secrets/paths/commands; workspace staging; one validation attempt; zero persistence and extra provider calls; no project shadow or installed destination (`invocation_test.go:112-174`, `181-232`). |
| CASE-U06 / `.../existing_destination` | All CASE-U05 protections plus byte-identical installed definition and successful existing-Factory run before and after rejection; no replacement executes (`invocation_test.go:141-167`). |

The baseline assertion inventory was obtained from the merge-base versions of
`greeting_test.go` and `invocation_test.go` and compared with the final source.
The migration changes ownership of process/session setup and the public session
entry used for installed runs; it does not remove the named behavior branches
or their product assertions. Work and Factory Event terminology in the P011
inventory is broader than the direct assertions present in this package; no
event-only evidence is inferred from the package run.

## Customer journey

1. The clean implementation head built one production-composed process through `support.BuildProcessWithContext` and one continuous loopback API host. The injected listener counted one start, while the provider edge routed by each scenario's private path (`shared_fixture_test.go:253-313`, `362-391`).
2. The parent ran the six exact scenarios sequentially. Each scenario copied the packaged `@you/factory-builder` into a private home, used private home/work/operator roots, received a unique request ID, opened a non-default Factory Session, and registered a scenario-owned provider route.
3. The focused and repeat commands above exercised the greeting, empty-request, graph, JavaScript, invalid-new, and invalid-existing journeys through the local-real production composition. The verbose result showed all six named subtests as `PASS`.
4. Each child cleanup closed its Factory Session, verified a subsequent public session read returned `404`, removed its private root, and recorded closure. The parent cleanup then stopped the reusable process, checked that `/status` was no longer served, and removed the shared root. All cleanup assertions passed.

## Cross-task integration and usability

- Documentation discoverability: not applicable; this is a test-only optimization with no customer-facing documentation or copy change.
- Permission and error behavior: invalid candidates retain safe, redacted guidance and no-install/no-overwrite behavior; no authorization semantics were added or claimed.
- Persistence/reload behavior: graph and JavaScript definitions are staged, persisted to the operator root, validated/listed, loaded, and run; the existing-destination bytes and run remain unchanged after rejection.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: the package emits the one-process/six-session lifecycle line and fails cleanup with explicit `FACTORY-BUILDER-CLEANUP-001` diagnostics. The verbose run also emitted existing discovery/watcher diagnostic noise for probed non-root or already-removed temporary paths; it did not affect the passing result or indicate a C06 leak.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | — | No blocking finding in the clean-room proof. | All required story-002 runtime and scope checks passed. | Focused, repeat, verbose focused, source audit, and diff checks above. |

## Verdict

PASS

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no FAIL or BLOCKED criterion was observed.
