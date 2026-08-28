# Validation report: C06 packaged Factory Builder

## Environment and artifact

- Commit/build identifier: `9c07763c72f727aed4811bfa07d73d890610ac57` (corrected implementation head validated before this report refresh).
- Base comparison: merge-base with `origin/main` was `54c3400e3bcc4cabce5bd3a376f51dedd424133d`.
- Environment and configuration: Windows 11 Home `10.0.26200`, `windows/amd64`, Go `1.25.0`; tracked Git status was clean before validation; task packet files remained ignored and outside the product diff.
- Governing plan: `docs/internal/development/plans/backlog/shared-functional-process-sessions.md`, section 12 `TASK-003` (the task packet's missing `docs/temp/functional-test-optimization.md` reference was corrected to this readable canonical plan).
- Customer entry point: the package's `TestFactoryBuilder` parent builds one reusable process, installs the production packaged Factory, and runs all six ordinary behavior cases through `Process.Execute` and the public `you --json run` CLI. After those finite invocations, the same process starts one continuous loopback API host and opens/closes the six private scenario sessions for lifecycle evidence; HTTP is retained only for that API-owned lifecycle contract.
- Real and substituted dependencies: `root.BuildProcess`/production wiring, packaged Factory loading, Factory Sessions, Factory Runtime, Work, Recordings, Workers, filesystem, and loopback HTTP were exercised locally. Only the provider command runner and API listener start edge were controlled through `edges.Edges`; no remote provider or paid call was used.
- Cost/call budget used: `$0`; no credentials, customer data, non-loopback network calls, or paid calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| C06 inventory and assertion preservation | PASS | The baseline at the merge-base contains the five P011 top-level rows; the invalid row expands into `new_destination` and `existing_destination`. The final parent retains all six exact scenario names as subtests in `greeting_test.go:58-74`. The detailed mapping is below. | No direct Factory Event assertion exists in either the baseline or final package source; this report does not claim a new event-specific assertion. Broader P011/project inventory remains with its owning gate. |
| C06 focused functional proof | PASS | `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/factory_builder` exited `0` in `23.962s` on corrected implementation head `9c07763c72f727aed4811bfa07d73d890610ac57`. | Does not prove remote provider behavior, OS hard-kill cleanup, race safety, project-wide timing, terminal CI, or merge. |
| C06 repeat functional proof | PASS | `go test -count=3 -timeout=30m ./tests/functional/factory/packaged/factory_builder` exited `0` in `78.759s`; all three package runs completed without a flake or leakage failure. | Does not prove race-detector safety, project-wide suite behavior, or remote provider behavior. |
| C06 lifecycle and cleanup | PASS | A verbose focused run exited `0` in `24.006s` and emitted `process_starts=1 explicit_sessions_opened=6 explicit_sessions_closed=6 unique_session_ids=6 scenario_roots_removed=6 runtime_artifacts=0 isolated_rows=0`. The source registers provider cleanup before scenario assertions, defers session closure before opening the six sessions, probes persisted sessions before host shutdown, checks public absence, closes response bodies, rejects post-shutdown `/status`, and removes roots (`greeting_test.go:53-81`, `shared_fixture_test.go:259-357`, `445-530`). | The lifecycle line records the declared zero artifact/row fields; project-wide artifact and isolation probes remain outside this package story. |
| C06 clean-head scope and compatibility audit | PASS | `git diff --check $(git merge-base origin/main HEAD) HEAD` was clean. The implementation correction changes only `greeting_test.go`, `invocation_test.go`, and `shared_fixture_test.go` under the owned package; no shared support, sibling, contract, generated, definition, baseline, UI, or CI file changed. | Does not prove terminal CI or merge. |
| C06 validation-loopback report shape and fidelity | PASS | This report follows `factory/docs/standards/validation-loopback-template.md`, records the exact commands and artifact, and classifies the proof as functional/local-real with a controlled provider edge. | Does not claim remote provider, OS crash, project-wide timing, terminal CI, conflict resolution, or merge evidence. |

### Six-case assertion inventory

| Case / final scenario | Preserved observable assertions |
| --- | --- |
| CASE-H01 / `TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding` | Completed CLI invocation; exactly one routing classification; zero build prompts; exactly one help prompt; all five authored guidance fragments remain required (`greeting_test.go:86-123`). |
| CASE-B02 / `TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing` | Bare CLI invocation completes and does not call the build workstation (`greeting_test.go:127-136`). |
| CASE-H03 / `TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory` | Builder completion text/status and no failure diagnostics; workspace-scoped staging; exact operator-root persistence command; no project-local shadow; installed definition validation/listing; graph name, Work type, worker, `init`/`complete`/`failed` routes; installed run return/signature and final text; one installed provider call (`invocation_test.go:87-116`, `270-430`, `524-558`). |
| CASE-H04 / `TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory` | Builder completion text/status; workspace staging and exact persistence; installed definition/listing; non-empty UTF-8 inline source; required briefs; `DEFAULT` permissions; `maxAgents=2` and concurrency `2`; installed-path CLI run with final JSON summary and `analysisCount=2`; two provider calls (`invocation_test.go:121-150`, `391-522`). |
| CASE-U05 / `.../new_destination` | Completed safe validation guidance containing Factory name, validation failure, diagnostic code, missing worker, correction, and not-installed result; redaction of secrets/paths/commands; workspace staging; one validation attempt; zero persistence and extra provider calls; no project shadow or installed destination (`invocation_test.go:157-213`, `220-268`). |
| CASE-U06 / `.../existing_destination` | All CASE-U05 protections plus byte-identical installed definition and successful existing-Factory run before and after rejection; no current-session replacement executes (`invocation_test.go:177-213`). |

The baseline assertion inventory was obtained from the merge-base versions of
`greeting_test.go` and `invocation_test.go` and compared with the corrected
source. The migration restores the baseline public CLI entry for every
ordinary and installed behavior run, while the public session HTTP contract is
used only for the six-session lifecycle witness. It does not remove the named
behavior branches or their product assertions. Work and Factory Event
terminology in the P011 inventory is broader than the direct assertions
present in this package; no event-only evidence is inferred from the package
run.

## Customer journey

1. The corrected implementation built one production-composed process through `support.BuildProcessWithContext` and installed the packaged `@you/factory-builder` through a normal process command. The six finite behavior cases then used private home/work/operator roots, controlled provider routing, and public `you --json run` commands (`shared_fixture_test.go:259-307`, `invocation_test.go:37-81`).
2. After the finite CLI cases completed, the same process started one continuous loopback API host. The six retained scenario directories were opened as unique non-default Factory Sessions through the public API and registered in the lifecycle ledger (`greeting_test.go:77-81`, `shared_fixture_test.go:309-357`, `445-494`).
3. The focused and repeat commands above exercised the greeting, empty-request, graph, JavaScript, invalid-new, and invalid-existing journeys through the local-real production composition. The verbose result showed all six named subtests as `PASS` and the exact one-start/six-session lifecycle line.
4. The test defers closure of all six sessions while the host is live, verifies each public session read returns `404`, removes each private root, probes the persisted session list for zero rows/artifacts, then lets framework cleanup stop the reusable process, reject post-shutdown `/status`, and remove the shared root. All cleanup assertions passed.

## Cross-task integration and usability

- Documentation discoverability: not applicable; this is a test-only optimization with no customer-facing documentation or copy change.
- Permission and error behavior: invalid candidates retain safe, redacted guidance and no-install/no-overwrite behavior; no authorization semantics were added or claimed.
- Persistence/reload behavior: graph and JavaScript definitions are staged, persisted to the operator root, validated/listed, loaded, and run; the existing-destination bytes and run remain unchanged after rejection.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: the package emits the one-process/six-session lifecycle line and fails cleanup with explicit `FACTORY-BUILDER-CLEANUP-001` diagnostics. The verbose run also emitted existing discovery/watcher diagnostic noise for probed non-root temporary paths; it did not affect the passing result or indicate a C06 leak.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | — | No blocking finding remains after the reviewed corrections: the package-size boundary is below 1000 lines, ordinary and installed behavior use `Process.Execute`/public CLI, current-session replacement and inline durable replay are removed, lifecycle cleanup queries authoritative persisted state, and the governing plan reference is readable. | All required story-001 behavior and story-002 validation checks pass on corrected implementation head `9c07763c72f727aed4811bfa07d73d890610ac57`. | Focused, repeat, verbose focused, source audit, canonical-plan check, and diff checks above. |

## Verdict

PASS

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no FAIL or BLOCKED criterion was observed.
