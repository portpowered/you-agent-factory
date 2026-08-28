# Validation report: C06 packaged Factory Builder

## Environment and artifact

- Commit/build identifier: `d9f14a1ff2f2b9e096769fa34198ee5259cfd8c1` (rebased implementation head validated before this report was amended).
- Base comparison: merge-base with `origin/main` is `54c3400e3bcc4cabce5bd3a376f51dedd424133d`.
- Environment and configuration: Windows 11 Home `10.0.26200`, `windows/amd64`, Go `1.25.0`; task packet files remain ignored scaffolding and outside the product diff.
- Governing plan: `docs/internal/development/plans/backlog/shared-functional-process-sessions.md`, section 12 `TASK-003`.
- Customer entry point: the package's `TestFactoryBuilder` parent builds one reusable process, installs the production packaged Factory, and starts one continuous loopback API host. Each of the six Builder behavior cases opens one unique non-default Factory Session and invokes through the public Factory Session HTTP contract because the public `run` command has no selector for an already-open session. Builder-issued customer commands still run through `Process.Execute`; installed graph/JavaScript checks run through the public CLI after the host releases the default runtime.
- Real and substituted dependencies: `root.BuildProcess`/production wiring, packaged Factory loading, Factory Sessions, Factory Runtime, Work, Recordings, Workers, filesystem, and loopback HTTP were exercised locally. Only the provider command runner and API listener start edge were controlled through `edges.Edges`; no remote provider or paid call was used.
- Cost/call budget used: `$0`; no credentials, customer data, non-loopback network calls, or paid calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| C06 inventory and assertion preservation | PASS | The parent retains all six exact scenario branches: vague first turn, no request, graph, JavaScript, invalid new destination, and invalid existing destination. The invalid row remains two subtests in `invocation_test.go`; behavior assertions remain in the owned package. | No new Factory Event-specific assertion was added; broader P011 inventory remains with its owning gate. |
| C06 focused functional proof | PASS | `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/factory_builder` exited `0` in `11.632s`. | Does not prove remote provider behavior, OS hard-kill cleanup, race safety, project-wide timing, terminal CI, or merge. |
| C06 repeat functional proof | PASS | `go test -count=3 -timeout=30m ./tests/functional/factory/packaged/factory_builder` exited `0` in `38.318s`; all three package runs completed without a flake or leakage failure. | Does not prove race-detector safety, project-wide suite behavior, or remote provider behavior. |
| C06 lifecycle and cleanup | PASS | A verbose focused run before the rebase exited `0` in `10.446s` and emitted `process_starts=1 explicit_sessions_opened=6 explicit_sessions_closed=6 unique_session_ids=6 scenario_roots_removed=6 runtime_artifacts=0 isolated_rows=0`. The six sessions were opened before their actual Builder calls, each public close was followed by a `404` probe, persisted state was queried before host shutdown, and the shared listener/root cleanup checks passed; the rebased focused and repeat runs also passed. | The package proves its own lifecycle and persisted-state contract; project-wide artifact and isolation probes remain outside this story. |
| C06 bounded topology/setup optimization | PASS | The reusable process and one host are built once. Explicit sessions are closed as a batch; the host is stopped once; installed-artifact CLI assertions then reuse the same process. The per-case current-session replacement, durable inline replay, post-hoc unused sessions, and second process/server were removed. The rebased package timing is `11.632s` focused and `38.318s` for three runs, compared with the prior corrected local `22.907s` and `69.756s`; this is a package-level improvement on the same environment class, while the requested CI rerun is tracked separately in the PR conversation. | Local wall-clock timing is host-contended and not a terminal CI result; it does not prove project-wide latency. |
| C06 clean-head scope and compatibility audit | PASS | `git diff --check` is clean. The implementation commit changes only `tests/functional/factory/packaged/factory_builder/{greeting_test.go,invocation_test.go,shared_fixture_test.go}`; no shared support, sibling, contract, generated, definition, baseline, UI, or CI file changed. `invocation_test.go` is 938 lines, below the backend-size limit. | Does not prove terminal CI or merge. |
| C06 validation-loopback report shape and fidelity | PASS | This report follows the validation-loopback template, records exact commands, artifact, observed lifecycle counters, the public CLI/API boundary, and remaining unproven edges. Browser verification is not applicable to this test-only package. | Does not claim remote provider, OS crash, project-wide timing, terminal CI, conflict resolution, or merge evidence. |

### Six-case assertion inventory

| Case / final scenario | Preserved observable assertions |
| --- | --- |
| CASE-H01 / `TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding` | One completed Builder response; exactly one routing classification; zero build prompts; one help prompt; authored guidance retains all required command/documentation fragments. |
| CASE-B02 / `TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing` | Bare invocation completes and does not call the build workstation. |
| CASE-H03 / `TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory` | Explicit-session Builder completion; scenario-factory-scoped staging; exact operator-root `you factory create` persistence command; no project-local shadow; installed definition validation/listing; graph Work Type, worker, `init`/`complete`/`failed` routes; installed-path CLI run return/signature/final text; one installed provider call. |
| CASE-H04 / `TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory` | Explicit-session Builder completion; scenario-factory staging and exact persistence; installed definition/listing; non-empty UTF-8 inline source; required briefs; `DEFAULT` permissions; `maxAgents=2` and concurrency `2`; installed-path CLI response-stream run with `analysisCount=2`; two provider calls. |
| CASE-U05 / `.../new_destination` | Explicit-session safe validation guidance containing Factory name, validation failure, diagnostic code, missing worker, correction, and not-installed result; path/command/secret redaction; one validation attempt; zero persistence and extra provider calls; no project shadow or installed destination. |
| CASE-U06 / `.../existing_destination` | Existing Factory is installed and run once before host startup; its bytes are captured; the actual invalid Builder request runs on its unique explicit session; the definition remains byte-identical and the existing Factory runs successfully again after host shutdown. No current-session replacement is used. |

The baseline assertion inventory was obtained from the merge-base versions of
`greeting_test.go` and `invocation_test.go` and compared with the corrected
source. The actual six Builder behavior requests now use the public explicit
Factory Session route because the CLI cannot target an already-open session;
the Builder's customer commands and all installed-artifact checks retain the
public CLI/`Process.Execute` path. The migration does not remove the named
behavior branches or their product assertions. Work and Factory Event
terminology in the P011 inventory is broader than the direct assertions
present in this package; no event-only evidence is inferred from the package
run.

## Customer journey

1. The corrected implementation built one root-owned production process and installed the packaged `@you/factory-builder`. Before host startup it prepared the existing-destination baseline Factory and ran it once through the public `you --json run --factory ... --no-record` command, preserving the before/after assertion without competing with the host's default runtime.
2. The same process then started one continuous loopback API host. Each child created a private home/work/operator root, registered its provider route by normalized scenario path, opened one unique non-default Factory Session, and sent its real Builder request to `/factory-sessions/{sessionID}/invocations`.
3. Graph and JavaScript Builder flows staged and validated candidates and persisted them through customer-facing CLI commands executed by the controlled provider runner. Invalid new and existing flows retained their safe, redacted validation behavior; both invalid Builder requests used their own explicit sessions.
4. After all six child behaviors finished, the parent closed all six sessions and verified public absence plus the persisted list. It then stopped the one host, ran the installed graph/JavaScript and existing-destination CLI assertions on the same reusable process, removed all six scenario roots, and recorded the lifecycle counters. Framework cleanup closed the process, probed that `/status` was no longer served, unregistered provider routes, and removed the shared root.

## Cross-task integration and usability

- Documentation discoverability: not applicable; this is a test-only optimization with no customer-facing documentation or copy change.
- Permission and error behavior: invalid candidates retain safe, redacted guidance and no-install/no-overwrite behavior; no authorization semantics were added or claimed.
- Persistence/reload behavior: graph and JavaScript definitions are staged, persisted to the operator root, validated/listed, loaded, and run; the existing-destination bytes and run remain unchanged after rejection.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: the package emits the one-process/six-session lifecycle line and fails cleanup with explicit `FACTORY-BUILDER-CLEANUP-001` diagnostics. The verbose run also emitted existing discovery/watcher diagnostic noise for probed non-root temporary paths; it did not affect the passing result or indicate a C06 leak.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | — | No local blocking finding remains after the topology correction. | Focused, repeat, and verbose package runs pass; the implementation proves one root-built process, one host, six actual explicit-session Builder cases, and installed CLI behavior after host release. | Commands and lifecycle output above. The prior CI comparison was base row `11.926s` (run `33159812096`, job `98811362498`) versus PR row `14.660s` (run `33160990936`, job `98815263861`); a new required CI run must start on the final pushed head. |

## Verdict

PASS

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no FAIL or BLOCKED criterion was observed.
