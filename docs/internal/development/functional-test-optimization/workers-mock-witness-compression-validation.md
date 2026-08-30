# Validation report: BEH-001 — Efficient, behavior-complete workers/mock witness topology

## Environment and artifact

- Commit/build identifier: source commit `8de795530937ff610717f0c6794bff8e7fd1d155`, rebased directly onto `origin/main` `5c997439079adf8761959897ba2fb70fdad8a1f9`; the final handoff head adds only this validation artifact and the evidence-ledger update.
- Environment and configuration: Windows 11 Home `10.0.26200` (`windows/amd64`), Go `go1.25.0`, PowerShell `7.6.5`, 24 logical processors, `GOMAXPROCS`, `GOFLAGS`, `FUNCTIONAL_DEFAULT_JOBS`, and `CI` unset. The successful full-package/race runs used a fresh temporary directory per process for `HOME`, `USERPROFILE`, `APPDATA`, and `LOCALAPPDATA`; no shared global staging state was changed.
- Customer entry point: retained rows invoke the compiled `./cmd/factory` binary; migrated rows construct the reusable customer process through `root.BuildProcess` and execute public CLI inputs through `Process.Execute`.
- Real and substituted dependencies: real Go compilation and OS process/exit/pipe behavior for six retained rows; controlled mock-worker and command-runner edges for the nine migrated rows; no remote or paid provider calls.
- Cost/call budget used: `$0`; one compiler child, six target-binary scenario children, and the bounded local test commands listed below.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| `GATE-COUNT-001` | PASS | Independent source count found six actual target-binary scenario calls: 3 single-Work, 1 aggregation, 2 named. Three build callers share one `sync.Once` compiler child; inclusive total is 7. | Future topology drift |
| `GATE-MAP-001` | PASS | The evidence ledger maps every contiguous `MOCK-01..48` property to an actual retained or migrated owner. Spot-checks covered default/verbose success, ordered aggregation, mixed JSON, breaker reason, script failure, and named human output in actual test bodies. | Additional reviewer-selected spot-checks |
| `GATE-SPINE-001` | PASS | Retained and migrated focused selectors passed; the complete package passed in the isolated environment. | Migrated rows intentionally do not prove OS exit/pipe semantics |
| `GATE-PACKAGE-001` | PASS | Three separate `go test ./tests/functional/workers/mock/... -count=1` runs with fresh isolated homes exited 0. | Default shared-host setup remains contended |
| `GATE-RACE-001` | PASS | One separate `go test -race ./tests/functional/workers/mock/... -count=1` run with a fresh isolated home exited 0 and emitted no race report. | Exhaustive schedules and future hosts |
| `GATE-PERF-001` | PASS | spawn-count criterion met; wall-clock medians advisory only - measured under operator-host contention, see ledger. Six target-binary scenario executions versus the 15 baseline and zero dropped `MOCK-01..48` mappings. | Future host/topology drift |
| `GATE-LOOP-001` | PASS | Clean rebase, source/count/map reconciliation, focused selectors, ordinary package, and race evidence were completed without repairing defects. | Terminal CI and merge |

## Customer journey

1. The six retained compiled-CLI children exercised exact quiet success,
   human failure, single-failure JSON, ordered aggregate JSON, named quiet
   success, and named terminal JSON failure at the real OS boundary; all
   focused selectors exited 0.
2. The nine migrated public CLI rows executed on the shared customer process
   with isolated factory inputs, HOME, streams, runtime logs, and controlled
   command edges; all nine selectors exited 0, including the failure-then-
   recovery path.
3. The complete workers/mock package exited 0 three times under fresh isolated
   user/config roots, and the race run exited 0 without a detected race.

## Cross-task integration and usability

- Documentation discoverability: the baseline ledger and this validation
  report are under `docs/internal/development/functional-test-optimization/`.
- Permission and error behavior: retained rows preserve real nonzero process
  errors and stderr; migrated rows preserve public error/output values through
  `Process.Execute`.
- Persistence/reload behavior: not applicable to this test-topology change;
  each row uses an isolated temporary Factory and no persisted product state.
- Accessibility/keyboard/responsive behavior: not applicable to backend/CLI
  test topology.
- Operational signals: the package's existing runtime logs exposed the exact
  shared staging-owner contention; the test change adds no production logging.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| PERF-001 | Advisory | Run the three story-001 pre samples and compare them with the three successful story-003 isolated-home post samples | Spawn reduction with zero witness loss; wall-clock direction is advisory under the operator ruling | Six target-binary scenario executions versus 15 baseline and zero dropped `MOCK-01..48` mappings. The medians were measured on an operator host concurrently running a live factory with many parallel Codex worker processes, so host noise dominates the signal. | `workers-mock-witness-compression-evidence.md`, Story 001 and Story 003 sections |
| ENV-001 | Environmental | `go test ./tests/functional/workers/mock/... -count=1` in the default environment | Package reaches all assertions | Repeated `indeterminate-contention`, owner PID 6224, at `C:\Users\andre\.you-agent-factory\factories\.you--full-flow.staging-owner` | Three default post samples and one default race attempt |

## Verdict

`PASS`

The implementation preserves the 48 mapped properties, reduces the target
binary to six scenario executions (seven including the separate compiler
child), and is green for the complete package and race gate when run with
fresh isolated user/config roots. Under the operator ruling, the wall-clock
medians are advisory only because they were measured on an operator host
concurrently running a live factory with many parallel Codex worker processes;
the spawn-count criterion and zero-drop witness mapping are the acceptance
evidence.

## Follow-up candidate

The shared `~default` Factory Definitions binding prevents safe concurrent one-shot migration rows because multiple invocations contend for the same runtime authority. A future performance lane may introduce per-invocation Factory Definitions/session authority and reassess parallelization; this lane leaves the behavior-preserving serialized topology unchanged.
